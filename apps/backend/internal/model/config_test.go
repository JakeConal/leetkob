package model_test

import (
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/model"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(model.EnvBaseURL, "https://provider.example.test/v1")
	t.Setenv(model.EnvModel, "pinned-model")
	t.Setenv(model.EnvTimeoutSeconds, "45")
	t.Setenv(model.EnvMaxOutputTokens, "2048")

	cfg, err := model.ConfigFromEnv()
	if err != nil {
		t.Fatalf("config from env: %v", err)
	}
	if cfg.BaseURL != "https://provider.example.test/v1" || cfg.Model != "pinned-model" {
		t.Fatalf("config = %+v, want the configured base URL and model", cfg)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", cfg.Timeout)
	}
	if cfg.MaxOutputTokens != 2048 {
		t.Fatalf("max output tokens = %d, want 2048", cfg.MaxOutputTokens)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv(model.EnvBaseURL, "https://provider.example.test/v1")
	t.Setenv(model.EnvModel, "pinned-model")
	t.Setenv(model.EnvTimeoutSeconds, "")
	t.Setenv(model.EnvMaxOutputTokens, "")

	cfg, err := model.ConfigFromEnv()
	if err != nil {
		t.Fatalf("config from env: %v", err)
	}
	if cfg.Timeout != model.DefaultTimeout {
		t.Fatalf("timeout = %s, want %s", cfg.Timeout, model.DefaultTimeout)
	}
	if cfg.MaxOutputTokens != model.DefaultMaxOutputTokens {
		t.Fatalf("max output tokens = %d, want %d", cfg.MaxOutputTokens, model.DefaultMaxOutputTokens)
	}
}

func TestConfigFromEnvRejectsInvalidNumbers(t *testing.T) {
	t.Setenv(model.EnvBaseURL, "https://provider.example.test/v1")
	t.Setenv(model.EnvModel, "pinned-model")
	t.Setenv(model.EnvTimeoutSeconds, "30")
	t.Setenv(model.EnvMaxOutputTokens, "1024")

	for name, value := range map[string]string{"not-a-number": "abc", "zero": "0", "negative": "-5"} {
		t.Run("timeout "+name, func(t *testing.T) {
			t.Setenv(model.EnvTimeoutSeconds, value)
			if _, err := model.ConfigFromEnv(); err == nil {
				t.Fatal("want an error for an invalid timeout")
			}
		})
		t.Run("tokens "+name, func(t *testing.T) {
			t.Setenv(model.EnvMaxOutputTokens, value)
			if _, err := model.ConfigFromEnv(); err == nil {
				t.Fatal("want an error for invalid output tokens")
			}
		})
	}
}

func TestConfigValidateRejectsUnsafeValues(t *testing.T) {
	cases := []struct {
		name string
		cfg  model.Config
	}{
		{name: "missing base url", cfg: model.Config{Model: "m"}},
		{name: "relative base url", cfg: model.Config{BaseURL: "/v1", Model: "m"}},
		{name: "non-http scheme", cfg: model.Config{BaseURL: "ftp://provider.example.test/v1", Model: "m"}},
		{name: "missing model", cfg: model.Config{BaseURL: "https://provider.example.test/v1"}},
		{name: "timeout too small", cfg: model.Config{BaseURL: "https://provider.example.test/v1", Model: "m", Timeout: time.Millisecond}},
		{name: "timeout too large", cfg: model.Config{BaseURL: "https://provider.example.test/v1", Model: "m", Timeout: model.MaxTimeout + time.Second}},
		{name: "negative output tokens", cfg: model.Config{BaseURL: "https://provider.example.test/v1", Model: "m", MaxOutputTokens: -1}},
		{name: "output tokens too large", cfg: model.Config{BaseURL: "https://provider.example.test/v1", Model: "m", MaxOutputTokens: model.MaxMaxOutputTokens + 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.WithDefaults().Validate(); err == nil {
				t.Fatalf("config %+v must be rejected", tc.cfg)
			}
			if _, err := model.NewHTTPChecker(tc.cfg); err == nil {
				t.Fatalf("new checker must reject config %+v", tc.cfg)
			}
		})
	}
}

func TestConfigWithDefaultsIsValid(t *testing.T) {
	cfg := model.Config{BaseURL: "https://provider.example.test/v1", Model: "pinned-model"}.WithDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaulted config must be valid: %v", err)
	}
	if cfg.Timeout != model.DefaultTimeout || cfg.MaxOutputTokens != model.DefaultMaxOutputTokens {
		t.Fatalf("defaults = %s/%d, want %s/%d", cfg.Timeout, cfg.MaxOutputTokens, model.DefaultTimeout, model.DefaultMaxOutputTokens)
	}
}
