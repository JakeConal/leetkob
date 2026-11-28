package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

type modelSettings struct {
	model         semantic.Model
	price         semantic.Price
	timeout       time.Duration
	maxLabCalls   int64
	maxDailyCalls int64
}

// configureModel has no provider access. Only the trusted worker reads the
// operator-mounted credential; offline operation never reads its configuration.
func configureModel(getenv func(string) string, readToken func(string) (string, error), log *slog.Logger) (modelSettings, error) {
	mode := strings.TrimSpace(getenv("MODEL_OFF"))
	if mode == "" || mode == "1" {
		return modelSettings{
			model: offlineModel{}, timeout: 2 * time.Second,
			price: semantic.Price{Known: true, MaxInputTokens: 1000, MaxOutputTokens: 200,
				InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 1},
		}, nil
	}
	if mode != "0" {
		return modelSettings{}, errors.New("worker: MODEL_OFF must be 0 or 1")
	}
	if getenv("WORKER_ISOLATED_ANALYZER") != "1" {
		return modelSettings{}, errors.New("worker: live model requires WORKER_ISOLATED_ANALYZER=1")
	}
	endpoint, err := url.Parse(strings.TrimSpace(getenv("MODEL_URL")))
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return modelSettings{}, errors.New("worker: MODEL_URL must be an HTTPS endpoint without credentials or query parameters")
	}
	name := strings.TrimSpace(getenv("MODEL_NAME"))
	path := strings.TrimSpace(getenv("MODEL_TOKEN_FILE"))
	if name == "" || path == "" {
		return modelSettings{}, errors.New("worker: MODEL_NAME and MODEL_TOKEN_FILE are required in live mode")
	}
	values := map[string]int64{}
	for _, field := range []struct {
		name     string
		fallback int64
		minimum  int64
		maximum  int64
	}{
		{"MODEL_TIMEOUT_SECONDS", 90, 1, 120},
		{"MODEL_MAX_INPUT_TOKENS", 120000, 1, 1000000},
		{"MODEL_MAX_OUTPUT_TOKENS", 1500, 1, 16000},
		{"MODEL_MAX_CALLS_PER_LAB", 160, 1, 1000},
		{"MODEL_MAX_CALLS_PER_DAY", 1000, 1, 10000},
		{"MODEL_INPUT_MICRO_USD_PER_TOKEN", -1, 0, 1000000},
		{"MODEL_OUTPUT_MICRO_USD_PER_TOKEN", -1, 0, 1000000},
	} {
		value := field.fallback
		if raw := strings.TrimSpace(getenv(field.name)); raw != "" {
			value, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return modelSettings{}, fmt.Errorf("worker: invalid %s", field.name)
			}
		}
		if value < field.minimum || value > field.maximum {
			return modelSettings{}, fmt.Errorf("worker: invalid or missing %s", field.name)
		}
		values[field.name] = value
	}
	token, err := readToken(path)
	if err != nil || strings.TrimSpace(token) == "" {
		return modelSettings{}, errors.New("worker: model credential file is unavailable or empty")
	}
	price := semantic.Price{
		Known: true, MaxInputTokens: values["MODEL_MAX_INPUT_TOKENS"], MaxOutputTokens: values["MODEL_MAX_OUTPUT_TOKENS"],
		InputMicroUSDPerToken: values["MODEL_INPUT_MICRO_USD_PER_TOKEN"], OutputMicroUSDPerToken: values["MODEL_OUTPUT_MICRO_USD_PER_TOKEN"],
	}
	if _, ok := price.ConservativeMicroUSD(); !ok {
		return modelSettings{}, errors.New("worker: model price bound is invalid")
	}
	return modelSettings{
		model: &auditedModel{
			inner: &semantic.HTTPAdapter{URL: endpoint.String(), Model: name, Token: strings.TrimSpace(token)},
			name:  name, price: price, log: log,
			auditDir: strings.TrimSpace(getenv("MODEL_AUDIT_DIR")),
		},
		price: price, timeout: time.Duration(values["MODEL_TIMEOUT_SECONDS"]) * time.Second,
		maxLabCalls: values["MODEL_MAX_CALLS_PER_LAB"], maxDailyCalls: values["MODEL_MAX_CALLS_PER_DAY"],
	}, nil
}

func readModelToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("model credential unavailable")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(raw) > 8192 {
		return "", errors.New("invalid model credential file")
	}
	return string(raw), nil
}

type auditedModel struct {
	inner    semantic.Model
	name     string
	price    semantic.Price
	log      *slog.Logger
	auditDir string
}

// Complete logs provenance, never a credential, prompt, raw response or identity.
// A byte bound conservatively bounds input tokens before dispatch. The HTTP
// request separately bounds output tokens. There are no transport-level retries.
func (m *auditedModel) Complete(ctx context.Context, req semantic.CompletionRequest) (semantic.ModelResponse, error) {
	if int64(len(req.System))+int64(len(req.UserJSON)) > m.price.MaxInputTokens {
		return semantic.ModelResponse{}, errors.New("worker: model input exceeds the priced bound")
	}
	started := time.Now()
	response, err := m.inner.Complete(ctx, req)
	digest := sha256.Sum256([]byte(req.System + "\n" + req.UserJSON))
	if auditErr := m.audit(req, response, err, digest, time.Since(started).Milliseconds()); auditErr != nil {
		m.log.Error("semantic audit could not be persisted")
		return semantic.ModelResponse{}, errors.New("worker: semantic audit unavailable")
	}
	fields := []any{"model", m.name, "elapsedMs", time.Since(started).Milliseconds(), "promptSha256", hex.EncodeToString(digest[:]), "repair", req.Repair}
	if err != nil {
		// Provider/transport errors can carry URLs. Do not log raw errors.
		var status semantic.ProviderStatusError
		if errors.As(err, &status) {
			fields = append(fields, "providerHTTPStatus", status.Status)
		}
		m.log.Warn("semantic provider call failed", fields...)
		return semantic.ModelResponse{}, err
	}
	fields = append(fields, "providerRequestId", response.RequestID, "inputTokens", response.InputTokens, "outputTokens", response.OutputTokens, "usageKnown", response.TokensKnown)
	if response.TokensKnown && response.InputTokens >= 0 && response.OutputTokens >= 0 && response.InputTokens <= m.price.MaxInputTokens && response.OutputTokens <= m.price.MaxOutputTokens {
		fields = append(fields, "costMicroUSD", response.InputTokens*m.price.InputMicroUSDPerToken+response.OutputTokens*m.price.OutputMicroUSDPerToken)
	}
	m.log.Info("semantic provider call completed", fields...)
	return response, nil
}

// Audit storage is private to the operator, separate from general logs/APIs.
// It retains the exact sanitized request and provenance, never a raw response.
func (m *auditedModel) audit(req semantic.CompletionRequest, response semantic.ModelResponse, callErr error, digest [32]byte, elapsed int64) error {
	if m.auditDir == "" {
		return nil
	}
	if err := os.MkdirAll(m.auditDir, 0o700); err != nil {
		return err
	}
	record := struct {
		Time              string `json:"time"`
		Model             string `json:"model"`
		ProviderRequestID string `json:"providerRequestId"`
		System            string `json:"system"`
		UserJSON          string `json:"userJSON"`
		PromptSHA256      string `json:"promptSha256"`
		InputTokens       int64  `json:"inputTokens"`
		OutputTokens      int64  `json:"outputTokens"`
		UsageKnown        bool   `json:"usageKnown"`
		ElapsedMS         int64  `json:"elapsedMs"`
		Repair            bool   `json:"repair"`
		Failed            bool   `json:"failed"`
		CostMicroUSD      *int64 `json:"costMicroUSD,omitempty"`
	}{
		Time: time.Now().UTC().Format(time.RFC3339Nano), Model: m.name, ProviderRequestID: response.RequestID,
		System: req.System, UserJSON: req.UserJSON, PromptSHA256: hex.EncodeToString(digest[:]),
		InputTokens: response.InputTokens, OutputTokens: response.OutputTokens, UsageKnown: response.TokensKnown,
		ElapsedMS: elapsed, Repair: req.Repair, Failed: callErr != nil,
	}
	if response.TokensKnown && response.InputTokens >= 0 && response.OutputTokens >= 0 && response.InputTokens <= m.price.MaxInputTokens && response.OutputTokens <= m.price.MaxOutputTokens {
		cost := response.InputTokens*m.price.InputMicroUSDPerToken + response.OutputTokens*m.price.OutputMicroUSDPerToken
		record.CostMicroUSD = &cost
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(m.auditDir, "call-*.json")
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
