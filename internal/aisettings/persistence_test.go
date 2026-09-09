package aisettings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	cases := []struct {
		name string
		raw  string
	}{
		{name: "not json", raw: "not-json"},
		{name: "empty object", raw: `{}`},
		{name: "schema only", raw: `{"schema_version":2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodePersisted(tc.raw, config.DefaultConfig().AI); err == nil {
				t.Fatal("malformed or incomplete settings must be rejected")
			}
		})
	}
}

func TestDecodeV2RejectsIncompleteOrInvalidFields(t *testing.T) {
	base := canonicalTestObject(t)
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing text", mutate: func(value map[string]any) { delete(value, "text") }},
		{name: "missing vision", mutate: func(value map[string]any) { delete(value, "vision") }},
		{name: "missing proxy", mutate: func(value map[string]any) { delete(value, "proxy") }},
		{name: "wrong top-level type", mutate: func(value map[string]any) { value["enabled"] = "true" }},
		{name: "wrong endpoint type", mutate: func(value map[string]any) { value["text"].(map[string]any)["timeout_seconds"] = "6" }},
		{name: "timeout out of range", mutate: func(value map[string]any) { value["text"].(map[string]any)["timeout_seconds"] = 0 }},
		{name: "too many fallbacks", mutate: func(value map[string]any) {
			value["text"].(map[string]any)["fallback_models"] = []any{"a", "b", "c", "d"}
		}},
		{name: "unknown proxy mode", mutate: func(value map[string]any) { value["proxy"].(map[string]any)["mode"] = "unknown" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := cloneObject(base)
			tc.mutate(value)
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodePersisted(string(raw), config.DefaultConfig().AI); err == nil {
				t.Fatal("invalid schema v2 was accepted")
			}
		})
	}
	if _, err := DecodePersisted(`{"text":{},"vision":{}}`, config.DefaultConfig().AI); err == nil {
		t.Fatal("schema-less object without legacy fields was accepted")
	}
}

func TestIncompleteV2DoesNotRewriteOrTouchSecret(t *testing.T) {
	value := canonicalTestObject(t)
	delete(value, "text")
	rawBytes, _ := json.Marshal(value)
	secretDir := filepath.Join(t.TempDir(), "secrets")
	store := &memoryAIStore{raw: string(rawBytes)}
	result, err := LoadAndMigratePersistedAI(context.Background(), store, secretDir, config.DefaultConfig().AI)
	if err == nil || store.setCalls != 0 || result.Rewritten {
		t.Fatal("incomplete schema v2 was rewritten")
	}
	if _, statErr := os.Stat(secretDir); !os.IsNotExist(statErr) {
		t.Fatal("incomplete schema v2 touched the secret directory")
	}
	if store.raw != string(rawBytes) {
		t.Fatal("original incomplete settings were changed")
	}
}

func TestLegacySecretMigrationFailureRetainsOriginalDatabaseRecord(t *testing.T) {
	raw := `{"Provider":"aihubmix","Model":"legacy-model","Endpoint":"https://legacy.invalid/v1","APIKey":"test-only-secret"}`
	secretPath := filepath.Join(t.TempDir(), "secret-file")
	if err := os.WriteFile(secretPath, []byte("not-a-directory"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &memoryAIStore{raw: raw}
	result, err := LoadAndMigratePersistedAI(context.Background(), store, secretPath, config.DefaultConfig().AI)
	if err == nil || store.setCalls != 0 || result.Rewritten {
		t.Fatal("secret migration failure did not fail closed")
	}
	if store.raw != raw {
		t.Fatal("legacy database record was changed after secret migration failure")
	}
}

func TestExistingSecretWinsOverLegacyKeyAndMigrationIsIdempotent(t *testing.T) {
	secretDir := filepath.Join(t.TempDir(), "secrets")
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(secretDir, "text.key")
	if err := os.WriteFile(secretPath, []byte("newer-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	value := canonicalTestObject(t)
	value["text"].(map[string]any)["api_key_file"] = secretPath
	value["APIKey"] = "older-secret"
	rawBytes, _ := json.Marshal(value)
	store := &memoryAIStore{raw: string(rawBytes)}
	first, err := LoadAndMigratePersistedAI(context.Background(), store, secretDir, config.DefaultConfig().AI)
	if err != nil || !first.Rewritten || !first.SecretReused || first.SecretCreated || store.setCalls != 1 {
		t.Fatalf("existing secret was not reused: rewritten=%v reused=%v created=%v writes=%d err=%v", first.Rewritten, first.SecretReused, first.SecretCreated, store.setCalls, err)
	}
	data, _ := os.ReadFile(secretPath)
	if strings.TrimSpace(string(data)) != "newer-secret" || strings.Contains(store.raw, "older-secret") {
		t.Fatal("legacy key replaced an existing secret or remained persisted")
	}
	second, err := LoadAndMigratePersistedAI(context.Background(), store, secretDir, config.DefaultConfig().AI)
	if err != nil || second.Rewritten || store.setCalls != 1 {
		t.Fatal("canonical migration was not idempotent")
	}
}

func TestCanonicalWriteFailureKeepsOldRecordAndLeavesPreparedSecret(t *testing.T) {
	raw := `{"Provider":"aihubmix","Model":"legacy-model","Endpoint":"https://legacy.invalid/v1","APIKey":"test-only-secret"}`
	secretDir := filepath.Join(t.TempDir(), "secrets")
	store := &memoryAIStore{raw: raw, setErr: errors.New("store unavailable")}
	result, err := LoadAndMigratePersistedAI(context.Background(), store, secretDir, config.DefaultConfig().AI)
	if err == nil || result.Rewritten || store.raw != raw {
		t.Fatal("database write failure did not retain the old record")
	}
	if !secretFileValid(filepath.Join(secretDir, "text.key")) {
		t.Fatal("prepared secret was not retained for the next retry")
	}
}

func canonicalTestObject(t *testing.T) map[string]any {
	t.Helper()
	value := config.DefaultConfig().AI
	value.Enabled = true
	value.Text = config.AIEndpointConfig{Enabled: true, Provider: "aihubmix", Model: "text-model", FallbackModels: []string{}, BaseURL: "https://aihubmix.invalid/v1", APIKeyEnv: "TEST_KEY", APIKeyFile: "", TimeoutSeconds: 6, JSONMode: "auto"}
	value.Vision = config.AIEndpointConfig{Enabled: false, Provider: "aihubmix", Model: "vision-model", FallbackModels: []string{}, BaseURL: "https://aihubmix.invalid/v1", APIKeyEnv: "TEST_KEY", APIKeyFile: "", TimeoutSeconds: 8, JSONMode: "auto"}
	raw, err := EncodePersistedAIConfig(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func cloneObject(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var clone map[string]any
	_ = json.Unmarshal(raw, &clone)
	return clone
}

type memoryAIStore struct {
	raw      string
	setCalls int
	setErr   error
}

func (s *memoryAIStore) GetSetting(context.Context, string) (string, bool, error) {
	return s.raw, s.raw != "", nil
}

func (s *memoryAIStore) SetSetting(_ context.Context, _ string, value string, _ time.Time) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.raw = value
	return nil
}
