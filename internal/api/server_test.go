package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

func setupTestServer() (*config.Config, *state.Manager, *http.ServeMux) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "secret-token-123"

	clock := state.NewFakeClock(time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC))
	stateMgr := state.NewManager(clock)
	s := NewServer(cfg, stateMgr)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("/v1/mode/study", s.withAuth(s.handleModeStudy))
	mux.HandleFunc("/v1/mode/break", s.withAuth(s.handleModeBreak))
	mux.HandleFunc("/v1/mode/off", s.withAuth(s.handleModeOff))
	mux.HandleFunc("/v1/task", s.withAuth(s.handleTask))
	mux.HandleFunc("/v1/feedback", s.withAuth(s.handleFeedback))

	return cfg, stateMgr, mux
}

func TestHealthzEndpoint(t *testing.T) {
	_, _, mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if resp.Status != "ok" || resp.Service != "supervisor" {
		t.Fatalf("unexpected health response: %+v", resp)
	}
}

func TestAuthProtection(t *testing.T) {
	_, _, mux := setupTestServer()

	// Missing header
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing auth, got %d", w.Code)
	}

	// Invalid token
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", w.Code)
	}

	// Valid token
	req = httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d", w.Code)
	}
}

func TestModeTransitionsViaAPI(t *testing.T) {
	_, _, mux := setupTestServer()

	// Study
	body, _ := json.Marshal(StudyModeRequest{Task: "Writing Go tests"})
	req := httptest.NewRequest(http.MethodPost, "/v1/mode/study", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status state.SystemStatus
	_ = json.NewDecoder(w.Body).Decode(&status)
	if status.UserMode != state.UserModeStudy || status.Task != "Writing Go tests" {
		t.Fatalf("expected STUDY mode with task, got %+v", status)
	}

	// Break
	req = httptest.NewRequest(http.MethodPost, "/v1/mode/break", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	_ = json.NewDecoder(w.Body).Decode(&status)
	if status.UserMode != state.UserModeBreak {
		t.Fatalf("expected BREAK mode, got %+v", status)
	}

	// Off
	req = httptest.NewRequest(http.MethodPost, "/v1/mode/off", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	_ = json.NewDecoder(w.Body).Decode(&status)
	if status.UserMode != state.UserModeOff {
		t.Fatalf("expected OFF mode, got %+v", status)
	}
}
