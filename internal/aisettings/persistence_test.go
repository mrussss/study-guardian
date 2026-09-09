package aisettings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"study-guardian/internal/classifier/providers"
	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

func TestEncodePersistedAIConfigExcludesRuntimeAndSecretValues(t *testing.T) {
	value := config.DefaultConfig().AI
	value.Provider = "fake"
	value.Model = "old-model"
	value.Endpoint = "https://old.invalid"
	value.APIKey = "test-only-secret"
	value.MigrationWarning = "provider fake is disabled outside developer_mode"
	value.Text = config.AIEndpointConfig{Provider: "aihubmix", Model: "glm-test", APIKeyFile: `C:\secrets\text.key`}
	raw, err := EncodePersistedAIConfig(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"MigrationWarning", "provider fake", "old-model", "old.invalid", "test-only-secret"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("canonical settings contain forbidden runtime data: %s", forbidden)
		}
	}
	if !strings.Contains(encoded, `"schema_version":2`) || !strings.Contains(encoded, `"api_key_file"`) {
		t.Fatal("canonical schema is incomplete")
	}
}

func TestDecodeV2WinsOverLegacyFields(t *testing.T) {
	raw := `{"schema_version":2,"enabled":true,"developer_mode":false,"use_vision_only_when_needed":true,"min_confidence":0.8,"proxy":{"mode":"manual","url":"http://127.0.0.1:8080"},"text":{"enabled":true,"provider":"aihubmix","model":"current-model","fallback_models":["fallback-model"],"base_url":"https://current.invalid/v1","api_key_env":"CURRENT_KEY","api_key_file":"C:\\secrets\\text.key","timeout_seconds":30,"json_mode":"auto","temperature":null},"vision":{"enabled":false,"provider":"aihubmix","model":"vision-model","fallback_models":[],"base_url":"https://current.invalid/v1","api_key_env":"CURRENT_KEY","api_key_file":"C:\\secrets\\vision.key","timeout_seconds":30,"json_mode":"auto","temperature":null},"Provider":"fake","Model":"old-model","Endpoint":"https://old.invalid","APIKey":"test-only-secret","MigrationWarning":"stale warning"}`
	loaded, err := DecodePersisted(raw, config.DefaultConfig().AI)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.Text.Provider != "aihubmix" || loaded.Config.Text.Model != "current-model" || loaded.Config.Text.BaseURL != "https://current.invalid/v1" {
		t.Fatalf("v2 text was not authoritative: %#v", loaded.Config.Text)
	}
	if loaded.Config.Provider != "" || loaded.Config.MigrationWarning != "" || loaded.Config.APIKey != "" {
		t.Fatal("legacy runtime fields were restored")
	}
	if loaded.LegacyAPIKey == "" || !loaded.NeedsRewrite {
		t.Fatal("legacy secret/cleanup was not detected")
	}
}

func TestDecodeLegacyFlatConfigAndRewriteWithoutSecret(t *testing.T) {
	raw := `{"Enabled":true,"DeveloperMode":false,"Provider":"aihubmix","Model":"legacy-model","Endpoint":"https://legacy.invalid/v1","APIKey":"test-only-secret"}`
	loaded, err := DecodePersisted(raw, config.DefaultConfig().AI)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.Text.Provider != "aihubmix" || loaded.Config.Text.Model != "legacy-model" || loaded.Config.Text.BaseURL != "https://legacy.invalid/v1" {
		t.Fatalf("legacy config was not migrated: %#v", loaded.Config.Text)
	}
	if loaded.Config.Provider != "" || loaded.Config.APIKey != "" || loaded.LegacyAPIKey == "" || !loaded.NeedsRewrite {
		t.Fatal("legacy fields were not isolated for migration")
	}
	encoded, err := EncodePersistedAIConfig(loaded.Config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "test-only-secret") || strings.Contains(string(encoded), "APIKey") {
		t.Fatal("legacy secret was written into canonical settings")
	}
}

func TestSavePersistsCanonicalConfigAndRestartKeepsProvider(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secretDir := filepath.Join(t.TempDir(), "secrets")
	initial := config.DefaultConfig()
	initial.AI.Provider = "fake"
	initial.AI.Model = "old-model"
	initial.AI.MigrationWarning = "provider fake is disabled outside developer_mode; rules continue"
	service := New(initial, store, secretDir, nil, nil)
	input := service.Settings()
	input.Enabled = true
	input.Text = EndpointDTO{Enabled: true, Provider: "aihubmix", Model: "current-model", FallbackModels: []string{"fallback-model"}, BaseURL: "https://aihubmix.invalid/v1", TimeoutSeconds: 30, JSONMode: "auto"}
	input.Vision = EndpointDTO{Enabled: false, Provider: "aihubmix", Model: "vision-model", BaseURL: "https://aihubmix.invalid/v1", TimeoutSeconds: 30, JSONMode: "auto"}
	if _, err := service.Save(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutSecret(context.Background(), "text", "test-only-secret"); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := store.GetSetting(context.Background(), SettingKey)
	if err != nil || !ok {
		t.Fatalf("saved AI settings unavailable: %v", err)
	}
	for _, forbidden := range []string{"MigrationWarning", "provider fake", "test-only-secret", `"APIKey"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("saved settings contain forbidden data: %s", forbidden)
		}
	}

	loaded, err := DecodePersisted(raw, config.DefaultConfig().AI)
	if err != nil {
		t.Fatal(err)
	}
	restarted := config.DefaultConfig()
	restarted.AI = loaded.Config
	config.NormalizeAIConfig(restarted, false)
	registry := providers.New(restarted)
	status := registry.Status()
	if status.TextProvider != "aihubmix" || status.Warning != "" {
		t.Fatalf("restart did not preserve clean provider state: provider=%q warning=%q", status.TextProvider, status.Warning)
	}
	if restarted.AI.Text.APIKeyFile == "" || !strings.HasSuffix(filepath.Clean(restarted.AI.Text.APIKeyFile), filepath.Join("secrets", "text.key")) {
		t.Fatal("secret reference was not preserved")
	}
	if _, err := os.Stat(filepath.Join(secretDir, "text.key")); err != nil {
		t.Fatal("secret file was not kept separate")
	}
}

func TestDecodeCanonicalJSONRejectsMalformedInput(t *testing.T) {
	if _, err := DecodePersisted("not-json", config.DefaultConfig().AI); err == nil {
		t.Fatal("malformed settings must be rejected")
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(`{"schema_version":2}`), &value); err != nil {
		t.Fatal(err)
	}
}
