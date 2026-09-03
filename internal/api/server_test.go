package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/motivation"
	"study-guardian/internal/review"
	"study-guardian/internal/semantic"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func setupTestServer() (*config.Config, *state.Manager, *http.ServeMux) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "secret-token-123"
	cfg.IPC.CollectorToken = "collector-token-456"

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
	mux.HandleFunc("/v1/collector/context", s.withCollectorAuth(s.handleCollectorContext))

	return cfg, stateMgr, mux
}

func TestCollectorTokenIsScopedAndContextIsAvailable(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "main-token"
	cfg.IPC.CollectorToken = "collector-token"
	stateMgr := state.NewManager(state.NewFakeClock(time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)))
	server := NewServer(cfg, stateMgr)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", server.withAuth(server.handleStatus))
	mux.HandleFunc("/v1/collector/context", server.withCollectorAuth(server.handleCollectorContext))
	request := httptest.NewRequest(http.MethodGet, "/v1/collector/context", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("main token must not access collector context: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/collector/context", nil)
	request.Header.Set("Authorization", "Bearer collector-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"user_mode"`)) {
		t.Fatalf("collector token context status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCollectorTurnUsesObservedLocalDateAndScopedAuth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "main-token"
	cfg.IPC.CollectorToken = "collector-token"
	stateMgr := state.NewManager(state.NewFakeClock(time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)))
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := NewServer(cfg, stateMgr)
	server.SetStorage(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/collector/turn", server.withCollectorAuth(server.handleCollectorTurn))
	body := bytes.NewBufferString(`{"platform":"chatgpt","external_conversation_id":"c1","turn_key":"t1","observed_at":"2026-09-03T23:58:00+08:00","mode_at_start":"STUDY","task_at_start":"Go","eligible_for_review":true,"finalized":true,"messages":[{"external_message_id":"m1","role":"user","content":"Explain interfaces","is_active":true}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/collector/turn", body)
	req.Header.Set("Authorization", "Bearer collector-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("collector turn status=%d body=%s", response.Code, response.Body.String())
	}
	record, err := store.LoadChatTurn(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	expectedDate := time.Date(2026, 9, 3, 23, 58, 0, 0, time.FixedZone("payload", 8*60*60)).In(time.Local).Format("2006-01-02")
	if record.LocalDate != expectedDate || !record.EligibleForReview {
		t.Fatalf("local_date=%s eligible=%v, want %s/true", record.LocalDate, record.EligibleForReview, expectedDate)
	}
}

func TestReviewGenerateAndEvidenceAPI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "main-token"
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := store.UpdateDailyState(context.Background(), "2026-09-03", 0, 1800, 0, 0, 1200, now); err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, state.NewManager(state.NewFakeClock(now)))
	server.SetStorage(store)
	server.SetReview(review.NewService(store, time.UTC, t.TempDir()))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/review/generate", server.withAuth(server.handleReviewGenerate))
	mux.HandleFunc("/v1/review/daily", server.withAuth(server.handleReviewDaily))
	mux.HandleFunc("/v1/review/evidence", server.withAuth(server.handleReviewEvidence))
	mux.HandleFunc("/v1/review/exclude", server.withAuth(server.handleReviewExclude))
	request := httptest.NewRequest(http.MethodPost, "/v1/review/generate", bytes.NewBufferString(`{"date":"2026-09-03"}`))
	request.Header.Set("Authorization", "Bearer main-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"generation_mode":"FALLBACK"`)) {
		t.Fatalf("generate status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/review/daily?date=2026-09-03", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("学习复盘")) {
		t.Fatalf("daily status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/review/evidence?date=2026-09-03&detail=summary", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"date":"2026-09-03"`)) {
		t.Fatalf("evidence status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/review/exclude", bytes.NewBufferString(`{"date":"2026-09-03","source_type":"whatever","source_id":"123"}`))
	request.Header.Set("Authorization", "Bearer main-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid exclusion status=%d body=%s", response.Code, response.Body.String())
	}
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

func TestCurrentActivityContractUsesMainAuthAndAgeFreshness(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "main-token"
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	semanticService := semantic.NewServiceWithTiming(store, semantic.Timing{LiveMaxAge: time.Minute, TransitionStableFor: time.Second, MinPersistInterval: time.Second, HeartbeatInterval: time.Minute})
	if err := semanticService.Observe(context.Background(), semantic.Candidate{ObservedAt: now, Fresh: true, UserMode: state.UserModeStudy, Task: "Go", Interaction: state.InteractionActive, Relation: state.RelationFocused, Privacy: state.PrivacyNormal, App: "Code.exe", Title: "main.go"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, state.NewManager(state.NewFakeClock(now)))
	server.SetSemantic(semanticService)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/activity/current", server.withAuth(server.handleCurrentActivity))

	request := httptest.NewRequest(http.MethodGet, "/v1/activity/current", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/activity/current", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("current activity status=%d body=%s", response.Code, response.Body.String())
	}
	var view semantic.CurrentActivityView
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != semantic.SchemaVersion || view.Activity != semantic.ActivityCoding || !view.Fresh || view.Task != "Go" {
		t.Fatalf("current activity view=%+v", view)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/activity/current", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", response.Code)
	}
}

func TestCurrentActivitySensitiveViewIsNeutralAndNotPersisted(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "main-token"
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	semanticService := semantic.NewServiceWithTiming(store, semantic.Timing{LiveMaxAge: time.Minute, TransitionStableFor: time.Second, MinPersistInterval: time.Second, HeartbeatInterval: time.Minute})
	if err := semanticService.Observe(context.Background(), semantic.Candidate{
		ObservedAt: now, Fresh: true, UserMode: state.UserModeStudy, Task: "Secret",
		Interaction: state.InteractionActive, Relation: state.RelationFocused, Privacy: state.PrivacySensitive,
		App: "Private.exe", Title: "secret.txt",
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, state.NewManager(state.NewFakeClock(now)))
	server.SetSemantic(semanticService)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/activity/current", server.withAuth(server.handleCurrentActivity))

	request := httptest.NewRequest(http.MethodGet, "/v1/activity/current", nil)
	request.Header.Set("Authorization", "Bearer main-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("current activity status=%d body=%s", response.Code, response.Body.String())
	}
	var view semantic.CurrentActivityView
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Privacy != state.PrivacySensitive || view.Fresh || view.Activity != semantic.ActivityUnknown || view.Confidence != 0 {
		t.Fatalf("sensitive current activity view=%+v", view)
	}
	rows, err := store.ListSemanticSnapshotsForDate(context.Background(), now.In(time.Local).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("sensitive observation was persisted: %+v", rows)
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

func TestMotivationSettingsAndEventsAPI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "secret-token-123"
	clock := state.NewFakeClock(time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC))
	stateMgr := state.NewManager(clock)
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := motivation.NewService(cfg, store)
	if _, err := store.RecordUIEvent(context.Background(), "TEST", "cursor test", "{}", time.Now()); err != nil {
		t.Fatal(err)
	}

	server := NewServer(cfg, stateMgr)
	server.SetMotivation(service)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/motivation/settings", server.withAuth(server.handleMotivationSettings))
	mux.HandleFunc("/v1/events", server.withAuth(server.handleEvents))

	req := httptest.NewRequest(http.MethodGet, "/v1/motivation/settings", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("settings GET status=%d body=%s", w.Code, w.Body.String())
	}
	var settings motivation.Settings
	if err := json.NewDecoder(w.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.DailyTargetMinutes != 120 {
		t.Fatalf("initial target=%d, want 120", settings.DailyTargetMinutes)
	}

	req = httptest.NewRequest(http.MethodPut, "/v1/motivation/settings", bytes.NewBufferString(`{"daily_target_minutes":90}`))
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("settings PUT status=%d body=%s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.DailyTargetMinutes != 90 {
		t.Fatalf("updated target=%d, want 90", settings.DailyTargetMinutes)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/events?after_id=0&limit=20", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"event_type":"TEST"`)) {
		t.Fatalf("events response status=%d body=%s", w.Code, w.Body.String())
	}
}
