package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"study-guardian/internal/config"
	"study-guardian/internal/reminder"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func TestReminderSettingsAPIValidatesPersistsAndApplies(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "token"
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine := reminder.NewEngine(cfg)
	server := NewServer(cfg, state.NewManager(nil))
	server.SetStorage(store)
	server.SetReminderSettings(engine)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/settings/reminder", server.withAuth(server.handleReminderSettings))

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/settings/reminder", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		return response
	}
	response := put(`{"cooldown_minutes":10,"quiet_periods":[{"start":"21:00","end":"24:00"}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("put=%d body=%s", response.Code, response.Body.String())
	}
	if got := engine.GetSettings().QuietPeriods; len(got) != 1 || got[0].End != "24:00" {
		t.Fatalf("not applied: %+v", got)
	}
	if _, ok, err := store.GetSetting(httptest.NewRequest(http.MethodGet, "/", nil).Context(), reminderSettingKey); err != nil || !ok {
		t.Fatalf("not persisted ok=%v err=%v", ok, err)
	}
	response = put(`{"cooldown_minutes":10,"quiet_periods":[{"start":"23:00","end":"24:01"}]}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid time accepted: %d", response.Code)
	}
}
