package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/eyecare"
	"study-guardian/internal/state"
)

func TestEyeCareAPISettingsStatusAndRevisionedActions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "eye-care-test-token"
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	manager := state.NewManager(state.NewFakeClock(now))
	if err := manager.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	eyeCfg := cfg.EyeCare
	eyeCfg.Enabled = true
	service, err := eyecare.NewWithClock(eyeCfg, cfg.Reminder, nil, manager, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, manager)
	server.SetEyeCare(service)

	request := httptest.NewRequest(http.MethodGet, "/v1/settings/eye-care", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated settings status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/settings/eye-care", nil)
	request.Header.Set("Authorization", "Bearer "+cfg.IPC.AuthToken)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	var returned config.EyeCareConfig
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &returned) != nil || returned != eyeCfg {
		t.Fatalf("settings status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/v1/settings/eye-care", bytes.NewBufferString(`{"enabled":true,"focus_minutes":40,"short_break_minutes":5,"long_break_after_focus_minutes":120,"long_break_minutes":20,"snooze_minutes":5,"max_snoozes":2,"unexpected":true}`))
	request.Header.Set("Authorization", "Bearer "+cfg.IPC.AuthToken)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown settings field status=%d body=%s", response.Code, response.Body.String())
	}
	service.RecordCreditedFocus(2400, state.TickOutcome{
		Now: now, DeltaSeconds: 2400, UserMode: state.UserModeStudy, ActivityValid: true,
		Interaction: state.InteractionActive, Relation: state.RelationFocused,
	}, manager.GetStatus())

	request = httptest.NewRequest(http.MethodPost, "/v1/eye-care/action", bytes.NewBufferString(`{"action":"START_SHORT_BREAK","expected_revision":1,"request_id":"api-start-1"}`))
	request.Header.Set("Authorization", "Bearer "+cfg.IPC.AuthToken)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	var started eyecare.Status
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &started) != nil || started.Phase != eyecare.ShortBreak || manager.GetStatus().ModeOrigin != state.ModeOriginEyeCare {
		t.Fatalf("action status=%d body=%s manager=%+v", response.Code, response.Body.String(), manager.GetStatus())
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/mode/study", bytes.NewBufferString(`{"task":"Go"}`))
	request.Header.Set("Authorization", "Bearer "+cfg.IPC.AuthToken)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte(`"eye_care_action_required"`)) || manager.GetStatus().UserMode != state.UserModeBreak {
		t.Fatalf("generic study route bypassed eye-care: status=%d body=%s mode=%s", response.Code, response.Body.String(), manager.GetStatus().UserMode)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/eye-care/action", bytes.NewBufferString(`{"action":"RESUME_STUDY","expected_revision":0,"request_id":"api-stale-1"}`))
	request.Header.Set("Authorization", "Bearer "+cfg.IPC.AuthToken)
	response = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale revision status=%d body=%s", response.Code, response.Body.String())
	}
}
