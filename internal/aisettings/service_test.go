package aisettings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

func TestAISettingsSecretIsWriteOnlyAndConnectionTestIsReal(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer secret-value" {
			http.Error(w, `{"error":{"message":"no"}}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"relation\":\"FOCUSED\",\"confidence\":0.9,\"activity\":\"fixed test\",\"task_related\":true,\"reason_short\":\"connection works\"}"}}]}`))
	}))
	defer server.Close()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.DefaultConfig()
	secretDir := filepath.Join(t.TempDir(), "secrets")
	service := New(cfg, store, secretDir, nil, nil)
	input := service.Settings()
	// Connection testing must work before the user enables AI for runtime use.
	input.Enabled = false
	input.Text = EndpointDTO{Enabled: true, Provider: "openai-compatible", Model: "test-model", BaseURL: server.URL, TimeoutSeconds: 2, JSONMode: "auto"}
	if _, err := service.Save(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	settings, err := service.PutSecret(context.Background(), "text", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Text.APIKeyConfigured {
		t.Fatal("secret should be configured")
	}
	raw, _ := json.Marshal(settings)
	if strings.Contains(string(raw), "secret-value") || strings.Contains(string(raw), "api_key_file") {
		t.Fatalf("sanitized DTO leaked secret metadata: %s", raw)
	}
	result := service.Test(context.Background(), "text")
	if !result.OK || requests != 1 || result.Provider != "openai-compatible" {
		t.Fatalf("test=%+v requests=%d", result, requests)
	}
	if service.Settings().Enabled {
		t.Fatal("connection test must not enable AI runtime settings")
	}
	secretPath := filepath.Join(secretDir, "text.key")
	if data, err := os.ReadFile(secretPath); err != nil || strings.TrimSpace(string(data)) != "secret-value" {
		t.Fatalf("secret file err=%v", err)
	}
	settings, err = service.DeleteSecret(context.Background(), "text")
	if err != nil || settings.Text.APIKeyConfigured {
		t.Fatalf("delete settings=%+v err=%v", settings, err)
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("managed secret still exists: %v", err)
	}
}

func TestAIConnectionFailureCategoryIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"raw provider detail"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()
	cfg := config.DefaultConfig()
	service := New(cfg, store, filepath.Join(t.TempDir(), "secrets"), nil, nil)
	input := service.Settings()
	input.Enabled = true
	input.Text = EndpointDTO{Enabled: true, Provider: "openai-compatible", Model: "bad", BaseURL: server.URL, TimeoutSeconds: 2, JSONMode: "auto"}
	_, _ = service.Save(context.Background(), input)
	_, _ = service.PutSecret(context.Background(), "text", "wrong")
	result := service.Test(context.Background(), "text")
	if result.OK || result.ErrorKind != "authentication_failed" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAIConnectionReportsRateLimitInsteadOfInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"account_rate_limited","message":"model rate limited"}}`, http.StatusTooManyRequests)
	}))
	defer server.Close()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()
	service := New(config.DefaultConfig(), store, filepath.Join(t.TempDir(), "secrets"), nil, nil)
	input := service.Settings()
	input.Text = EndpointDTO{Enabled: true, Provider: "openai-compatible", Model: "busy", BaseURL: server.URL, TimeoutSeconds: 2, JSONMode: "auto"}
	_, _ = service.Save(context.Background(), input)
	_, _ = service.PutSecret(context.Background(), "text", "configured")
	result := service.Test(context.Background(), "text")
	if result.OK || result.ErrorKind != "account_rate_limited" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAISettingsValidateAndPersistFallbackModels(t *testing.T) {
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()
	service := New(config.DefaultConfig(), store, filepath.Join(t.TempDir(), "secrets"), nil, nil)
	input := service.Settings()
	input.Text = EndpointDTO{Enabled: true, Provider: "aihubmix", Model: "free-model", FallbackModels: []string{"paid-model"}, BaseURL: "https://aihubmix.com/v1", TimeoutSeconds: 6, JSONMode: "auto"}
	saved, err := service.Save(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Text.FallbackModels) != 1 || saved.Text.FallbackModels[0] != "paid-model" {
		t.Fatalf("saved fallback models=%v", saved.Text.FallbackModels)
	}
	input.Text.FallbackModels = []string{"free-model"}
	if _, err := service.Save(context.Background(), input); err == nil {
		t.Fatal("duplicate primary/fallback model should be rejected")
	}
}

func TestAISettingsAlwaysExposeFallbackModelsAsArrays(t *testing.T) {
	settings := sanitized(config.DefaultConfig().AI)
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"fallback_models":null`) {
		t.Fatalf("fallback model lists must be arrays: %s", raw)
	}
}

func TestProxyTestUsesSavedProxyAndDoesNotReturnResponseBody(t *testing.T) {
	var path string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.String()
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"secret":"must not cross API"}`))
	}))
	defer proxy.Close()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := New(config.DefaultConfig(), store, filepath.Join(t.TempDir(), "secrets"), nil, nil)
	input := service.Settings()
	input.Proxy = ProxyDTO{Mode: config.AIProxyManual, URL: proxy.URL}
	input.Text = EndpointDTO{Enabled: true, Provider: "openai-compatible", Model: "probe", BaseURL: "http://example.invalid/v1", TimeoutSeconds: 2, JSONMode: "auto"}
	if _, err := service.Save(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	result := service.TestProxy(context.Background())
	if !result.OK || result.Mode != config.AIProxyManual || result.LatencyMS < 0 || path != "http://example.invalid/v1/models" {
		t.Fatalf("result=%+v path=%q", result, path)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "must not cross API") || strings.Contains(string(encoded), proxy.URL) {
		t.Fatalf("proxy test leaked response or URL: %s", encoded)
	}
}
