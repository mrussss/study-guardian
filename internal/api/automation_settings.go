package api

import (
	"context"
	"encoding/json"
	"net/http"

	"study-guardian/internal/automation"
)

type AutomationSettingsManager interface {
	Settings() automation.Settings
	Save(context.Context, automation.Settings) (automation.Settings, error)
}

func (s *Server) SetAutomationSettings(manager AutomationSettingsManager) {
	s.automationSettings = manager
}

func (s *Server) handleAutomationSettings(w http.ResponseWriter, r *http.Request) {
	if s.automationSettings == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "automation settings unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeTaskPresetJSON(w, http.StatusOK, s.automationSettings.Settings())
	case http.MethodPut:
		var input automation.Settings
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
			return
		}
		settings, err := s.automationSettings.Save(r.Context(), input)
		if err != nil {
			writeTaskPresetError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeTaskPresetJSON(w, http.StatusOK, settings)
	default:
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
