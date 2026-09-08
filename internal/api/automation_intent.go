package api

import (
	"encoding/json"
	"net/http"

	"study-guardian/internal/state"
)

// AutomationIntentManager exposes the short-lived confirmation queue owned by
// the state manager. The controller still decides when an intent is created;
// the API only lets the UI accept or reject it.
type AutomationIntentManager interface {
	PendingAutomationIntent() *state.AutomationIntent
	AcceptAutomationIntent(string) error
	RejectAutomationIntent(string) error
}

type automationIntentRequest struct {
	IntentID string `json:"intent_id"`
}

func (s *Server) handleAutomationPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.automationIntent == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "automation intent unavailable")
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, map[string]interface{}{"pending": s.automationIntent.PendingAutomationIntent()})
}

func (s *Server) handleAutomationAccept(w http.ResponseWriter, r *http.Request) {
	s.handleAutomationDecision(w, r, true)
}

func (s *Server) handleAutomationReject(w http.ResponseWriter, r *http.Request) {
	s.handleAutomationDecision(w, r, false)
}

func (s *Server) handleAutomationDecision(w http.ResponseWriter, r *http.Request, accept bool) {
	if r.Method != http.MethodPost {
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.automationIntent == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "automation intent unavailable")
		return
	}
	var req automationIntentRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
			return
		}
	}
	var err error
	if accept {
		err = s.automationIntent.AcceptAutomationIntent(req.IntentID)
	} else {
		err = s.automationIntent.RejectAutomationIntent(req.IntentID)
	}
	if err != nil {
		writeTaskPresetError(w, http.StatusConflict, err.Error())
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, s.stateMgr.GetStatus())
}
