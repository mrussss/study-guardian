package sensor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestSensorClientHealth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status:       "ok",
			Service:      "screen-sensor",
			MSSAvailable: true,
		})
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	client := NewHTTPClient(u.Hostname(), port, "token-123")

	resp, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != "ok" || !resp.MSSAvailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSensorClientCapture(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/capture" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer token-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CaptureResponse{
			Timestamp: "2026-09-02T10:00:00Z",
			Monitor:   1,
			Changed:   true,
			Hash:      "abc12345",
			IsStub:    false,
		})
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	client := NewHTTPClient(u.Hostname(), port, "token-123")

	resp, err := client.Capture(context.Background(), CaptureRequest{Monitor: 1})
	if err != nil {
		t.Fatalf("capture failed: %v", err)
	}
	if !resp.Changed || resp.Hash != "abc12345" {
		t.Fatalf("unexpected capture response: %+v", resp)
	}

	// Test with invalid token
	badClient := NewHTTPClient(u.Hostname(), port, "wrong-token")
	_, err = badClient.Capture(context.Background(), CaptureRequest{Monitor: 1})
	if err == nil {
		t.Fatalf("expected error for bad token, got nil")
	}
}
