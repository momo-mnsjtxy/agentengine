package video

import (
	"fmt"
	"os"
	"strings"
)

// ModelConfig describes an OpenAI-compatible model endpoint. Secrets are read
// only from the process environment and are never included in errors or logs.
type ModelConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	Protocol  string
	Reasoning string
}

// ModelConfigFromEnv loads the defaults used by the Jingliu CLI. The model is
// intentionally configurable because provider model IDs can change.
func ModelConfigFromEnv() (ModelConfig, error) {
	cfg := ModelConfig{
		BaseURL:   envOr("JINGLIU_BASE_URL", "https://ai.onyxaxis.org/v1"),
		APIKey:    strings.TrimSpace(os.Getenv("JINGLIU_API_KEY")),
		Model:     envOr("JINGLIU_MODEL", "claude-opus-5"),
		Protocol:  envOr("JINGLIU_PROTOCOL", "responses"),
		Reasoning: envOr("JINGLIU_REASONING", "medium"),
	}
	if cfg.APIKey == "" {
		return ModelConfig{}, fmt.Errorf("JINGLIU_API_KEY is required")
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return ModelConfig{}, fmt.Errorf("JINGLIU_BASE_URL and JINGLIU_MODEL are required")
	}
	return cfg, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
