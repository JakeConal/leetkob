package checker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

// SemanticEngine decides semantic constraints for one solution. The CLI
// uses a guarded semantic.Checker over the provider transport; tests inject
// a fake. A nil engine means semantic pairs stay unsure.
type SemanticEngine interface {
	Check(ctx context.Context, in semantic.Input) (semantic.Result, error)
}

// semanticEngine applies one configured price schedule to every call so the
// lab cap is enforced before dispatch.
type semanticEngine struct {
	checker *semantic.Checker
	price   semantic.Price
}

// Check implements SemanticEngine.
func (e *semanticEngine) Check(ctx context.Context, in semantic.Input) (semantic.Result, error) {
	in.Price = e.price
	return e.checker.Check(ctx, in)
}

// callRecord is one provider dispatch in the restricted call log. The
// prompt itself is represented by a hash, never committed.
type callRecord struct {
	Time          string `json:"time"`
	Model         string `json:"model"`
	ProviderID    string `json:"providerRequestId"`
	InputTokens   int64  `json:"inputTokens"`
	OutputTokens  int64  `json:"outputTokens"`
	ElapsedMillis int64  `json:"elapsedMs"`
	CostMicroUSD  *int64 `json:"costMicroUSD,omitempty"`
	UsageKnown    bool   `json:"usageKnown"`
	PromptSHA256  string `json:"promptSha256"`
	Attempts      int    `json:"attempts"`
	Error         string `json:"error,omitempty"`
}

// loggingModel records bounded provenance for every provider response. The
// token and prompt content are never written.
type loggingModel struct {
	inner    semantic.Model
	model    string
	logPath  string
	inPrice  int64
	outPrice int64
	mu       sync.Mutex
	maxInput int64
}

// Complete implements semantic.Model.
func (m *loggingModel) Complete(ctx context.Context, req semantic.CompletionRequest) (semantic.ModelResponse, error) {
	if m.maxInput > 0 && int64(len(req.System))+int64(len(req.UserJSON)) > m.maxInput {
		return semantic.ModelResponse{}, fmt.Errorf("checker: input exceeds the priced token bound")
	}
	started := time.Now()
	// One dispatch per reservation. Only the guarded checker's schema repair
	// may retry, and it reserves again before doing so.
	response, err := m.inner.Complete(ctx, req)
	attempts := 1
	if err != nil {
		m.record(callRecord{
			Time: time.Now().UTC().Format(time.RFC3339Nano), Model: m.model,
			ElapsedMillis: time.Since(started).Milliseconds(), Error: err.Error(),
			PromptSHA256: promptDigest(req), Attempts: attempts,
		})
		return semantic.ModelResponse{}, err
	}
	if m.logPath != "" {
		digest := sha256.Sum256([]byte(req.System + "\n" + req.UserJSON))
		var cost *int64
		actual, known := (semantic.Price{Known: response.TokensKnown, MaxInputTokens: response.InputTokens, MaxOutputTokens: response.OutputTokens,
			InputMicroUSDPerToken: m.inPrice, OutputMicroUSDPerToken: m.outPrice}).ConservativeMicroUSD()
		if known && response.InputTokens <= m.maxInput && response.OutputTokens <= req.MaxOutputTokens {
			cost = &actual
		}
		record := callRecord{
			Time:          time.Now().UTC().Format(time.RFC3339Nano),
			Model:         m.model,
			Attempts:      attempts,
			ProviderID:    response.RequestID,
			InputTokens:   response.InputTokens,
			OutputTokens:  response.OutputTokens,
			ElapsedMillis: time.Since(started).Milliseconds(),
			CostMicroUSD:  cost,
			UsageKnown:    response.TokensKnown,
			PromptSHA256:  hex.EncodeToString(digest[:]),
		}
		m.record(record)
	}
	return response, nil
}

// record appends one provenance line; failures never carry the token.
func (m *loggingModel) record(record callRecord) {
	if m.logPath == "" {
		return
	}
	line, marshalErr := json.Marshal(record)
	if marshalErr != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	file, openErr := os.OpenFile(m.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if openErr != nil {
		return
	}
	_, _ = file.Write(append(line, '\n'))
	_ = file.Close()
}

func promptDigest(req semantic.CompletionRequest) string {
	digest := sha256.Sum256([]byte(req.System + "\n" + req.UserJSON))
	return hex.EncodeToString(digest[:])
}

// NewSemanticEngineFromEnv builds the guarded semantic engine. MODEL_OFF=1
// returns (nil, nil) before any credential is read. The provider URL, model
// and token are required for the real path; the token is read from the
// environment only and never logged.
func NewSemanticEngineFromEnv() (SemanticEngine, error) {
	if os.Getenv("MODEL_OFF") == "1" {
		return nil, nil
	}
	if os.Getenv("CHECKER_ISOLATED_ANALYZER") != "1" {
		return nil, fmt.Errorf("checker: live evaluation requires isolated analysis; run the qualified analyzer separately from provider credentials")
	}
	providerURL := strings.TrimSpace(os.Getenv("MODEL_URL"))
	parsed, err := url.Parse(providerURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("checker: MODEL_URL must be an HTTPS endpoint without credentials or query parameters")
	}
	modelName := strings.TrimSpace(os.Getenv("MODEL_NAME"))
	token := strings.TrimSpace(os.Getenv("MODEL_TOKEN"))
	if providerURL == "" || modelName == "" || token == "" {
		return nil, fmt.Errorf("checker: MODEL_URL, MODEL_NAME and MODEL_TOKEN are required unless MODEL_OFF=1")
	}
	timeout := envInt64("MODEL_TIMEOUT_SECONDS", 90)
	inPrice := envInt64("MODEL_INPUT_MICRO_USD_PER_TOKEN", -1)
	outPrice := envInt64("MODEL_OUTPUT_MICRO_USD_PER_TOKEN", -1)
	maxInput := envInt64("MODEL_MAX_INPUT_TOKENS", 16000)
	maxOutput := envInt64("MODEL_MAX_OUTPUT_TOKENS", 1500)
	if timeout <= 0 || inPrice < 0 || outPrice < 0 || maxInput <= 0 || maxOutput <= 0 {
		return nil, fmt.Errorf("checker: invalid model configuration")
	}

	adapter := &semantic.HTTPAdapter{URL: providerURL, Model: modelName, Token: token}
	logging := &loggingModel{
		inner: adapter, model: modelName,
		logPath: strings.TrimSpace(os.Getenv("MODEL_CALL_LOG")),
		inPrice: inPrice, outPrice: outPrice,
		maxInput: maxInput,
	}
	checker := &semantic.Checker{
		Model:   logging,
		Store:   semantic.NewMemoryStore(),
		Timeout: time.Duration(timeout) * time.Second,
	}
	return &semanticEngine{
		checker: checker,
		price: semantic.Price{
			Known: true, MaxInputTokens: maxInput, MaxOutputTokens: maxOutput,
			InputMicroUSDPerToken: inPrice, OutputMicroUSDPerToken: outPrice,
		},
	}, nil
}

func envInt64(name string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
