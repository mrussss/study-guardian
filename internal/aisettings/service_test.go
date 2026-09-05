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
	input.Enabled = true
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
