package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMigratesLegacyAIInMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	legacy := []byte("ai:\n  enabled: true\n  provider: deepseek\n  model: deepseek-chat\n  endpoint: https://api.deepseek.com\nstandby:\n  first_study_active_minutes: 5\n")
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Text.Provider != "deepseek" || cfg.AI.Text.Model != "deepseek-chat" || cfg.AI.Text.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("legacy AI was not mapped: %#v", cfg.AI.Text)
	}
	if cfg.AI.MigrationWarning == "" {
		t.Fatal("expected migration warning")
	}
	if string(mustRead(t, path)) != string(legacy) {
		t.Fatal("LoadConfig must not rewrite legacy config")
	}
}

func TestFakeProviderRequiresDeveloperMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Text.Provider = "fake"
	NormalizeAIConfig(cfg, false)
	if cfg.AI.Enabled {
		t.Fatal("fake provider must be disabled outside developer mode")
	}
	cfg = DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.DeveloperMode = true
	cfg.AI.Text.Provider = "fake"
	NormalizeAIConfig(cfg, false)
	if !cfg.AI.Enabled {
		t.Fatal("fake provider should be allowed in developer mode")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
