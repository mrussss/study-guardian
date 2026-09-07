package classifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAICompatibleProviderRejectsInvalidSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"message": map[string]string{
					"content": `{"relation":"FOCUSED","confidence":7.8,"activity":"work","task_related":true,"reason_short":"bad confidence"}`,
				},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(server.URL, "", "test-model")
	_, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"})
	if err == nil {
		t.Fatal("expected invalid confidence to be rejected")
	}
}

func TestOpenAICompatibleProviderRejectsMissingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"message": map[string]string{
					"content": `{"relation":"UNKNOWN","confidence":0.5,"activity":"work","task_related":false}`,
				},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(server.URL, "", "test-model")
	_, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"})
	if err == nil {
		t.Fatal("expected missing reason_short to be rejected")
	}
}

func TestOpenAICompatibleProviderFallsBackWhenJSONModeUnsupported(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			if _, ok := body["response_format"]; !ok {
				t.Fatal("first request should use response_format")
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format is unsupported"}}`))
			return
		}
		if _, ok := body["response_format"]; ok {
			t.Error("fallback request must omit response_format")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]string{"content": `{"relation":"FOCUSED","confidence":0.9,"activity":"go","task_related":true,"reason_short":"task"}`}}}})
	}))
	defer server.Close()
	p := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "test", JSONMode: "auto", SupportsJSONMode: true, Timeout: time.Second})
	if _, err := p.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}

func TestOpenAICompatibleProviderDoesNotRetryUnauthorized(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()
	p := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "test", JSONMode: "auto", SupportsJSONMode: true, Timeout: time.Second})
	if _, err := p.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err == nil {
		t.Fatal("expected unauthorized error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
	if p.CooldownUntil().Before(time.Now().Add(4 * time.Minute)) {
		t.Fatal("unauthorized response should create a long cooldown")
	}
}

func TestOpenAICompatibleProviderOmitsTemperatureUnlessConfigured(t *testing.T) {
	seen := make(chan map[string]interface{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen <- body
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]string{"content": `{"relation":"FOCUSED","confidence":0.9,"activity":"go","task_related":true,"reason_short":"task"}`}}}})
	}))
	defer server.Close()

	defaultProvider := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "test", Timeout: time.Second})
	if _, err := defaultProvider.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err != nil {
		t.Fatal(err)
	}
	if body := <-seen; body["temperature"] != nil {
		t.Fatal("temperature must be omitted by default")
	}

	temperature := 0.2
	explicitProvider := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "test", Timeout: time.Second, Temperature: &temperature})
	if _, err := explicitProvider.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err != nil {
		t.Fatal(err)
	}
	if body := <-seen; body["temperature"] != temperature {
		t.Fatalf("temperature=%v, want %v", body["temperature"], temperature)
	}
}

func TestModelFallbackProviderUsesNextModelForRetryableFailure(t *testing.T) {
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		models = append(models, body.Model)
		if body.Model == "free-model" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"model_rate_limited","message":"quota reached"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"relation":"FOCUSED","confidence":0.9,"activity":"go","task_related":true,"reason_short":"task"}`}}}})
	}))
	defer server.Close()
	primary := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "free-model", Timeout: time.Second})
	fallback := NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "paid-model", Timeout: time.Second})
	provider := NewModelFallbackProvider(primary, fallback)
	if _, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "free-model" || models[1] != "paid-model" {
		t.Fatalf("models=%v", models)
	}
	if selected := provider.(interface{ LastModel() string }).LastModel(); selected != "paid-model" {
		t.Fatalf("selected model=%q", selected)
	}
}

func TestModelFallbackProviderStopsOnAuthenticationFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()
	provider := NewModelFallbackProvider(
		NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "free-model", Timeout: time.Second}),
		NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "paid-model", Timeout: time.Second}),
	)
	if _, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"}); err == nil {
		t.Fatal("expected authentication failure")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
}

func TestModelFallbackProviderOnlyAdvancesForExplicitModelFailure(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		body       string
		wantCalls  int
		wantResult bool
	}{
		{name: "no available channel", body: `{"error":{"code":"no_available_channel"}}`, wantCalls: 2, wantResult: true},
		{name: "bare account rate limit", body: `{"error":{"message":"try later"}}`, wantCalls: 1, wantResult: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Model string `json:"model"`
				}
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request.Model == "free-model" {
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(testCase.body))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"relation":"FOCUSED","confidence":0.9,"activity":"go","task_related":true,"reason_short":"task"}`}}}})
			}))
			defer server.Close()
			provider := NewModelFallbackProvider(
				NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "free-model", Timeout: time.Second}),
				NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: server.URL, Model: "paid-model", Timeout: time.Second}),
			)
			_, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"})
			if (err == nil) != testCase.wantResult || calls != testCase.wantCalls {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
