package model

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names for the real provider adapter. Only
// EnvAPIKey is secret and it is read lazily at dispatch time.
const (
	EnvBaseURL         = "MODEL_BASE_URL"
	EnvAPIKey          = "MODEL_API_KEY"
	EnvModel           = "MODEL_NAME"
	EnvTimeoutSeconds  = "MODEL_TIMEOUT_SECONDS"
	EnvMaxOutputTokens = "MODEL_MAX_OUTPUT_TOKENS"
)

// Bounds and defaults for a bounded call. They mirror the approved PA#3
// contract limits: a 64 KiB statement, 20 constraints and 65,536 source
// bytes, plus a conservative output budget.
const (
	DefaultTimeout         = 30 * time.Second
	MinTimeout             = 100 * time.Millisecond
	MaxTimeout             = 5 * time.Minute
	DefaultMaxOutputTokens = 1024
	MinMaxOutputTokens     = 1
	MaxMaxOutputTokens     = 16384

	// Environment timeouts are whole seconds, so the operator-facing lower
	// bound is one second even though programmatic Config allows less.
	MinTimeoutSeconds = 1

	MaxStatementBytes     = 64 * 1024
	MaxSourceBytes        = 65536
	MaxConstraints        = 20
	MaxConstraintTextRune = 4096
	MaxEvidenceTextRune   = 4096
)

// Config is the non-secret provider configuration. The API key is not
// part of Config so it cannot be logged or serialized accidentally.
type Config struct {
	// BaseURL is the API root, for example https://api.example.test/v1.
	// The adapter appends /chat/completions.
	BaseURL string
	// Model is the provider model name, for example a pinned chat model.
	Model string
	// Timeout bounds the whole Check call, including the one permitted
	// schema-repair retry.
	Timeout time.Duration
	// MaxOutputTokens bounds the provider's response length.
	MaxOutputTokens int
}

// WithDefaults fills unset timeout and output bounds. BaseURL and Model
// have no default: an implicit provider must never be contacted.
func (c Config) WithDefaults() Config {
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.MaxOutputTokens == 0 {
		c.MaxOutputTokens = DefaultMaxOutputTokens
	}
	return c
}

// Validate reports non-secret configuration errors. It never reads a
// credential.
func (c Config) Validate() error {
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("model: %s must be an absolute http(s) URL", EnvBaseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("model: %s must use http or https", EnvBaseURL)
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model: %s is required", EnvModel)
	}
	if c.Timeout < MinTimeout || c.Timeout > MaxTimeout {
		return fmt.Errorf("model: timeout must be between %s and %s", MinTimeout, MaxTimeout)
	}
	if c.MaxOutputTokens < MinMaxOutputTokens || c.MaxOutputTokens > MaxMaxOutputTokens {
		return fmt.Errorf("model: max output tokens must be between %d and %d", MinMaxOutputTokens, MaxMaxOutputTokens)
	}
	return nil
}

// ConfigFromEnv loads the non-secret provider configuration. It does not
// read EnvAPIKey and does not refuse MODEL_OFF; the constructor refuses
// offline operation before any credential or provider client exists, so a
// MODEL_OFF=1 image can load config without provider settings.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL: strings.TrimSpace(os.Getenv(EnvBaseURL)),
		Model:   strings.TrimSpace(os.Getenv(EnvModel)),
	}
	if raw := strings.TrimSpace(os.Getenv(EnvTimeoutSeconds)); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("model: %s must be an integer number of seconds", EnvTimeoutSeconds)
		}
		if seconds < MinTimeoutSeconds || seconds > int(MaxTimeout/time.Second) {
			return Config{}, fmt.Errorf("model: %s must be between %d and %d seconds", EnvTimeoutSeconds, MinTimeoutSeconds, int(MaxTimeout/time.Second))
		}
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	if raw := strings.TrimSpace(os.Getenv(EnvMaxOutputTokens)); raw != "" {
		tokens, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("model: %s must be an integer", EnvMaxOutputTokens)
		}
		if tokens < MinMaxOutputTokens || tokens > MaxMaxOutputTokens {
			return Config{}, fmt.Errorf("model: %s must be between %d and %d", EnvMaxOutputTokens, MinMaxOutputTokens, MaxMaxOutputTokens)
		}
		cfg.MaxOutputTokens = tokens
	}
	return cfg.WithDefaults(), nil
}

// modelOff reports the offline selection. MODEL_OFF is read on every
// construction and dispatch; it is never cached across a credential read.
func modelOff() bool {
	return os.Getenv("MODEL_OFF") == "1"
}

// envAPIKey reads the provider credential at dispatch time only. The
// default key source is never invoked by NewHTTPChecker.
func envAPIKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(EnvAPIKey))
	if key == "" {
		return "", ErrMissingAPIKey
	}
	return key, nil
}
