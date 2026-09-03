package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompleteJSONUsesTransportAndDecodesOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header was not forwarded")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "review-model" {
			t.Fatalf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]string{"content": `{"ok":true}`},
		}}})
	}))
	defer server.Close()

	c := NewClient(Options{Endpoint: server.URL, APIKey: "test-key", Model: "review-model", Timeout: time.Second})
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.CompleteJSON(context.Background(), "return JSON", map[string]string{"date": "2026-09-03"}, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("decoded output is false")
	}
}

func TestCompleteJSONFallsBackWhenJSONModeUnsupported(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			if body["response_format"] == nil {
				t.Fatal("first request should use JSON mode")
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format unsupported"}}`))
			return
		}
		if body["response_format"] != nil {
			t.Fatal("fallback request should omit JSON mode")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]string{"content": `{"ok":true}`},
		}}})
	}))
	defer server.Close()
	c := NewClient(Options{Endpoint: server.URL, Model: "review-model", SupportsJSONMode: true, JSONMode: "auto", Timeout: time.Second})
	var out map[string]bool
	if err := c.CompleteJSON(context.Background(), "", "payload", &out); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !out["ok"] {
		t.Fatalf("calls=%d out=%v", calls, out)
	}
}

func TestCompleteJSONRequiresModel(t *testing.T) {
	c := NewClient(Options{Endpoint: "http://127.0.0.1:1"})
	if err := c.CompleteJSON(context.Background(), "", nil, &struct{}{}); err == nil {
		t.Fatal("expected missing model error")
	}
}
