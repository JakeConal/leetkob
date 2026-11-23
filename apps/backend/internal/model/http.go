package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxAttempts is the total provider dispatch limit: the first call plus
// at most one schema-repair retry.
const maxAttempts = 2

// maxProviderResponseBytes bounds a single provider response so a
// misbehaving provider cannot exhaust worker memory. The wire content is
// additionally bounded by Config.MaxOutputTokens.
const maxProviderResponseBytes = 2 << 20

// maxRepairEchoBytes bounds how much of a rejected response may be
// echoed into the single repair attempt.
const maxRepairEchoBytes = 4096

// Doer is the minimal HTTP dependency of the adapter. *http.Client
// implements it; tests inject a fake or an httptest client.
type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Option customizes the adapter before use.
type Option func(*HTTPChecker)

// WithHTTPClient injects the HTTP transport. Tests always inject an
// httptest-backed Doer, so no test can reach a real provider.
func WithHTTPClient(doer Doer) Option {
	return func(checker *HTTPChecker) {
		if doer != nil {
			checker.doer = doer
		}
	}
}

// WithAPIKeySource injects the credential source. The source is called
// once per dispatch-capable Check, never by the constructor.
func WithAPIKeySource(source func() (string, error)) Option {
	return func(checker *HTTPChecker) {
		if source != nil {
			checker.apiKeySource = source
		}
	}
}

// HTTPChecker is the real provider adapter. It uses only the Go standard
// library and an OpenAI-compatible chat-completions endpoint.
type HTTPChecker struct {
	cfg          Config
	endpoint     string
	doer         Doer
	apiKeySource func() (string, error)
}

// NewHTTPChecker validates non-secret configuration and creates the
// adapter. It refuses MODEL_OFF=1 before reading any credential, so the
// offline checker image never creates a provider client. The API key is
// read lazily on the first Check that actually dispatches.
func NewHTTPChecker(cfg Config, options ...Option) (*HTTPChecker, error) {
	if modelOff() {
		return nil, ErrModelOff
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	checker := &HTTPChecker{
		cfg:          cfg,
		endpoint:     strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions",
		doer:         http.DefaultClient,
		apiKeySource: envAPIKey,
	}
	for _, option := range options {
		if option != nil {
			option(checker)
		}
	}
	return checker, nil
}

// Check performs one bounded semantic check.
//
// The configured timeout covers the whole call, including the single
// schema-repair retry. Provider and validation failures return an unsure
// Result with a distinct Cause and a nil error; local refusal and caller
// errors return an error. The adapter never dispatches when MODEL_OFF=1.
func (checker *HTTPChecker) Check(ctx context.Context, request Request) (Result, error) {
	if modelOff() {
		return Result{}, ErrModelOff
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	apiKey, err := checker.apiKeySource()
	if err != nil {
		return Result{}, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return Result{}, ErrMissingAPIKey
	}

	started := time.Now()
	result := Result{Model: checker.cfg.Model}
	callCtx, cancel := context.WithTimeout(ctx, checker.cfg.Timeout)
	defer cancel()

	lines := sourceLines(request.Source)
	var repair *repairHint

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt

		outcome, dispatchErr := checker.dispatch(callCtx, apiKey, request, repair)
		if dispatchErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(ctxErr, context.Canceled) {
				return Result{}, ctxErr
			}
			cause := CauseProviderFailure
			if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
				cause = CauseTimeout
			}
			return finishUnsure(result, request, cause, started), nil
		}

		result.ProviderRequestID = outcome.requestID
		result.InputTokens += outcome.inputTokens
		result.OutputTokens += outcome.outputTokens

		constraints, kind, detail := validateModelOutput(outcome.content, request, lines)
		switch kind {
		case outputValid:
			result.Constraints = constraints
			result.Elapsed = time.Since(started)
			return result, nil
		case outputEvidenceInvalid:
			// Fabricated or unsupported evidence is a deterministic content
			// failure; retrying it would not be a schema repair.
			return finishUnsure(result, request, CauseEvidenceInvalid, started), nil
		default:
			if attempt < maxAttempts {
				repair = &repairHint{previous: truncate(outcome.content, maxRepairEchoBytes), problem: detail}
				continue
			}
			return finishUnsure(result, request, CauseSchemaInvalid, started), nil
		}
	}

	// Unreachable: maxAttempts is at least one and every branch returns.
	return finishUnsure(result, request, CauseProviderFailure, started), nil
}

// repairHint carries the one rejected response into the single retry.
type repairHint struct {
	previous string
	problem  string
}

type dispatchOutcome struct {
	content      string
	requestID    string
	inputTokens  int
	outputTokens int
}

// dispatch performs exactly one provider HTTP request. Every failure is
// reported to Check, which owns cause mapping.
func (checker *HTTPChecker) dispatch(ctx context.Context, apiKey string, request Request, repair *repairHint) (dispatchOutcome, error) {
	body, err := checker.requestBody(request, repair)
	if err != nil {
		return dispatchOutcome{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, checker.endpoint, bytes.NewReader(body))
	if err != nil {
		return dispatchOutcome{}, fmt.Errorf("model: build provider request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)

	response, err := checker.doer.Do(httpRequest)
	if err != nil {
		return dispatchOutcome{}, fmt.Errorf("model: provider request failed: %w", err)
	}
	defer response.Body.Close()

	requestID := response.Header.Get("x-request-id")
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return dispatchOutcome{}, fmt.Errorf("model: provider returned status %d", response.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil {
		return dispatchOutcome{}, fmt.Errorf("model: read provider response: %w", err)
	}
	if len(raw) > maxProviderResponseBytes {
		return dispatchOutcome{}, errors.New("model: provider response exceeds the size bound")
	}

	var envelope completionEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return dispatchOutcome{}, fmt.Errorf("model: malformed provider envelope: %w", err)
	}
	if envelope.ID != "" && requestID == "" {
		requestID = envelope.ID
	}
	if len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		return dispatchOutcome{}, errors.New("model: provider returned no message content")
	}

	return dispatchOutcome{
		content:      envelope.Choices[0].Message.Content,
		requestID:    requestID,
		inputTokens:  envelope.Usage.PromptTokens,
		outputTokens: envelope.Usage.CompletionTokens,
	}, nil
}

// requestBody sends only the statement, constraint IDs/text and sanitized
// source. Tests, labels, student metadata and TA deductions are absent by
// construction, not by filtering.
func (checker *HTTPChecker) requestBody(request Request, repair *repairHint) ([]byte, error) {
	constraints := make([]Constraint, len(request.Constraints))
	copy(constraints, request.Constraints)
	prompt, err := json.Marshal(promptPayload{
		Statement:   request.Statement,
		Constraints: constraints,
		Source:      request.Source,
	})
	if err != nil {
		return nil, fmt.Errorf("model: encode prompt: %w", err)
	}

	messages := []message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(prompt)},
	}
	if repair != nil {
		if repair.previous != "" {
			messages = append(messages, message{Role: "assistant", Content: repair.previous})
		}
		instruction := repairInstruction
		if repair.problem != "" {
			instruction += " Validation detail: " + repair.problem + "."
		}
		messages = append(messages, message{Role: "user", Content: instruction})
	}

	body, err := json.Marshal(completionRequest{
		Model:          checker.cfg.Model,
		MaxTokens:      checker.cfg.MaxOutputTokens,
		Temperature:    0,
		ResponseFormat: responseFormat{Type: "json_object"},
		Messages:       messages,
	})
	if err != nil {
		return nil, fmt.Errorf("model: encode provider request: %w", err)
	}
	return body, nil
}

type promptPayload struct {
	Statement   string       `json:"statement"`
	Constraints []Constraint `json:"constraints"`
	Source      string       `json:"source"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type completionRequest struct {
	Model          string         `json:"model"`
	MaxTokens      int            `json:"max_tokens"`
	Temperature    float64        `json:"temperature"`
	ResponseFormat responseFormat `json:"response_format"`
	Messages       []message      `json:"messages"`
}

type completionEnvelope struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// finishUnsure copies the aggregate provenance into an all-unsure Result.
func finishUnsure(result Result, request Request, cause UnsureCause, started time.Time) Result {
	result.Cause = cause
	result.Constraints = make([]ConstraintResult, len(request.Constraints))
	for i, constraint := range request.Constraints {
		result.Constraints[i] = ConstraintResult{
			ConstraintID: constraint.ID,
			Verdict:      VerdictUnsure,
			Cause:        cause,
		}
	}
	result.Elapsed = time.Since(started)
	return result
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
