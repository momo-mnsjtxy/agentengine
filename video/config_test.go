package video

import "testing"

func TestModelConfigRequiresKey(t *testing.T) {
	t.Setenv("JINGLIU_API_KEY", "")
	if _, err := ModelConfigFromEnv(); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestModelConfigDefaults(t *testing.T) {
	t.Setenv("JINGLIU_API_KEY", "test-key")
	t.Setenv("JINGLIU_BASE_URL", "")
	t.Setenv("JINGLIU_MODEL", "")
	cfg, err := ModelConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://ai.onyxaxis.org/v1" || cfg.Model != "claude-opus-5" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
