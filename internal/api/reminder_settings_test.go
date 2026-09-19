package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/eyecare"
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

func TestReminderSettingsAPIAffectsEyeCareNotificationWithoutRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "token"
	cfg.Reminder.QuietPeriods = nil
	now := time.Date(2026, 9, 13, 10, 30, 0, 0, time.Local)
	manager := state.NewManager(state.NewFakeClock(now))
	if err := manager.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	engine := reminder.NewEngine(cfg)
	eyeCfg := cfg.EyeCare
	eyeCfg.Enabled = true
	eyeCare, err := eyecare.NewWithReminderSettingsProviderAndClock(eyeCfg, engine, nil, manager, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	eyeCare.RecordCreditedFocus(2400, state.TickOutcome{Now: now, UserMode: state.UserModeStudy, ActivityValid: true}, manager.GetStatus())
	server := NewServer(cfg, manager)
	server.SetStorage(store)
	server.SetReminderSettings(engine)

	put := func(quietPeriods string) *httptest.ResponseRecorder {
		body := `{"cooldown_minutes":10,"quiet_periods":` + quietPeriods + `}`
		request := httptest.NewRequest(http.MethodPut, "/v1/settings/reminder", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		return response
	}
	if response := put(`[{"start":"10:00","end":"11:00"}]`); response.Code != http.StatusOK {
		t.Fatalf("adding live quiet period failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if notification, err := eyeCare.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("live quiet period did not suppress eye-care toast: notification=%+v err=%v", notification, err)
	}
	if response := put(`[]`); response.Code != http.StatusOK {
		t.Fatalf("removing live quiet period failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if notification, err := eyeCare.TakePendingNotification(now); err != nil || notification == nil {
		t.Fatalf("removing quiet period did not release pending eye-care toast: notification=%+v err=%v", notification, err)
	}
}
