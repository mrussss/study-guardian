package review

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"study-guardian/internal/ai"
	"study-guardian/internal/config"
)

func TestHTTPReviewProviderGeneratesCanonicalDocumentAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "review-test" || len(request.Messages) != 2 || request.Messages[0].Role != "system" {
			t.Fatalf("request=%+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":1,\"date\":\"2026-09-03\",\"headline\":\"复盘\",\"topics\":[],\"accomplishments\":[],\"unfinished\":[],\"difficulties\":[],\"behavior\":{\"distraction_count\":0,\"largest_distraction_seconds\":0,\"average_recovery_seconds\":0},\"tomorrow_priority\":\"继续\"}"}}]}`))
	}))
	defer server.Close()
	provider, err := NewProvider(ProviderOptions{Name: "fake-http", Endpoint: server.URL, Model: "review-test", Timeout: time.Second, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	doc, metadata, err := provider.Generate(context.Background(), ReviewInput{Date: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Date != "2026-09-03" || doc.Headline != "复盘" {
		t.Fatalf("document=%+v", doc)
	}
	if metadata.Provider != "fake-http" || metadata.Model != "review-test" || metadata.PromptVersion != ReviewPromptVersion {
		t.Fatalf("metadata=%+v", metadata)
	}
}

func TestHTTPReviewProviderClassifiesErrorsWithoutLeakingCause(t *testing.T) {
	secret := "super-secret-api-key"
	cases := []struct {
		name     string
		status   int
		body     string
		wantKind ProviderErrorKind
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":{"message":"` + secret + `"}}`, wantKind: ProviderErrorHTTP},
		{name: "server", status: http.StatusBadGateway, body: `{"error":{"message":"upstream"}}`, wantKind: ProviderErrorUnavailable},
		{name: "invalid-json", status: http.StatusOK, body: `{"choices":[{"message":{"content":"not-json"}}]}`, wantKind: ProviderErrorInvalidJSON},
		{name: "schema", status: http.StatusOK, body: `{"choices":[{"message":{"content":"{\"date\":123}"}}]}`, wantKind: ProviderErrorSchemaInvalid},
		{name: "unsupported-version", status: http.StatusOK, body: `{"choices":[{"message":{"content":"{\"schema_version\":2}"}}]}`, wantKind: ProviderErrorUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider, err := NewProvider(ProviderOptions{Name: "test", Endpoint: server.URL, APIKey: secret, Model: "model", Timeout: time.Second, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = provider.Generate(context.Background(), ReviewInput{Date: "2026-09-03"})
			if err == nil {
				t.Fatal("expected provider error")
			}
			var providerErr ProviderError
			if !errors.As(err, &providerErr) || providerErr.Kind != tc.wantKind {
				t.Fatalf("error=%T %v, want kind %s", err, err, tc.wantKind)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("provider error leaked secret: %v", err)
			}
		})
	}
}

func TestHTTPReviewProviderRequiresModelBeforeRequest(t *testing.T) {
	if _, err := NewProvider(ProviderOptions{Name: "test"}); err == nil {
		t.Fatal("missing model should fail at construction")
	} else {
		var providerErr ProviderError
		if !errors.As(err, &providerErr) || providerErr.Kind != ProviderErrorNotConfigured {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestNewConfiguredProviderInheritsTextProfileAndChecksModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Text.Provider = "ollama"
	cfg.AI.Text.Model = "qwen2.5"
	cfg.AI.Text.BaseURL = "http://127.0.0.1:11434/v1"
	provider, status := NewConfiguredProvider(cfg)
	if provider == nil || !status.Configured || status.Provider != "ollama" || status.Model != "qwen2.5" {
		t.Fatalf("provider=%T status=%+v", provider, status)
	}
	cfg.AI.Text.Model = ""
	provider, status = NewConfiguredProvider(cfg)
	if provider != nil || status.Configured || !strings.Contains(status.Warning, "model") {
		t.Fatalf("missing model provider=%T status=%+v", provider, status)
	}
}

func TestNewConfiguredProviderKeepsReviewTimeoutWhenInheritingTextProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":1,\"date\":\"2026-09-07\",\"headline\":\"复盘\",\"topics\":[],\"accomplishments\":[],\"unfinished\":[],\"difficulties\":[],\"behavior\":{\"distraction_count\":0,\"largest_distraction_seconds\":0,\"average_recovery_seconds\":0},\"tomorrow_priority\":\"继续\"}"}}]}`))
	}))
	defer server.Close()
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Text.Provider = "openai-compatible"
	cfg.AI.Text.Model = "review-model"
	cfg.AI.Text.BaseURL = server.URL
	cfg.AI.Text.TimeoutSeconds = 1
	cfg.Review.Provider.TimeoutSeconds = 3

	provider, status := NewConfiguredProvider(cfg)
	if provider == nil || !status.Configured {
		t.Fatalf("provider=%T status=%+v", provider, status)
	}
	if _, _, err := provider.Generate(context.Background(), ReviewInput{Date: "2026-09-07"}); err != nil {
		t.Fatalf("review provider should use its own timeout: %v", err)
	}
}

func TestReviewModelFallbackUsesNextModelAfterInvalidSchema(t *testing.T) {
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		models = append(models, request.Model)
		content := `{"schema_version":0}`
		if request.Model == "paid-model" {
			content = `{"schema_version":1,"date":"2026-09-06","headline":"复盘","topics":[],"accomplishments":[],"unfinished":[],"difficulties":[],"behavior":{"distraction_count":0,"largest_distraction_seconds":0,"average_recovery_seconds":0},"tomorrow_priority":"继续"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	first, _ := NewProvider(ProviderOptions{Name: "aihubmix", Endpoint: server.URL, Model: "free-model", Timeout: time.Second})
	second, _ := NewProvider(ProviderOptions{Name: "aihubmix", Endpoint: server.URL, Model: "paid-model", Timeout: time.Second})
	document, metadata, err := NewModelFallbackProvider(first, second).Generate(context.Background(), ReviewInput{Date: "2026-09-06"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Headline != "复盘" || metadata.Model != "paid-model" || len(models) != 2 {
		t.Fatalf("document=%+v metadata=%+v models=%v", document, metadata, models)
	}
}

type deadlineAwareReviewProvider struct {
	timeout time.Duration
	calls   int
	err     error
}

func (p *deadlineAwareReviewProvider) Generate(context.Context, ReviewInput) (Document, ProviderMetadata, error) {
	p.calls++
	return Document{}, ProviderMetadata{Provider: "test", Model: "test"}, p.err
}

func (p *deadlineAwareReviewProvider) Timeout() time.Duration { return p.timeout }

func TestReviewModelFallbackDoesNotStartAfterTotalDeadline(t *testing.T) {
	first := &deadlineAwareReviewProvider{timeout: time.Second, err: ProviderError{Kind: ProviderErrorHTTP, FailureKind: ai.FailureModelNotFound, FailureScope: ai.FailureScopeModel}}
	second := &deadlineAwareReviewProvider{timeout: time.Second, err: errors.New("must not run")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, _, _ = NewModelFallbackProvider(first, second).Generate(ctx, ReviewInput{Date: "2026-09-07"})
	if first.calls != 1 || second.calls != 0 {
		t.Fatalf("calls first=%d second=%d", first.calls, second.calls)
	}
}
