package api

import (
	"context"
	"encoding/json"
	"net/http"

	"study-guardian/internal/aisettings"
)

type AISettingsManager interface {
	Settings() aisettings.SettingsDTO
	Save(context.Context, aisettings.SettingsDTO) (aisettings.SettingsDTO, error)
	PutSecret(context.Context, string, string) (aisettings.SettingsDTO, error)
	DeleteSecret(context.Context, string) (aisettings.SettingsDTO, error)
	Test(context.Context, string) aisettings.TestResult
}

func (s *Server) SetAISettings(manager AISettingsManager) { s.aiSettings = manager }

func (s *Server) handleAISettings(w http.ResponseWriter, r *http.Request) {
	if s.aiSettings == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "AI settings unavailable")
		return
	}
	if r.Method == http.MethodGet {
		writeTaskPresetJSON(w, http.StatusOK, s.aiSettings.Settings())
		return
	}
	if r.Method != http.MethodPut {
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input aisettings.SettingsDTO
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	settings, err := s.aiSettings.Save(r.Context(), input)
	if err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, settings)
}

type aiSecretRequest struct {
	Target string `json:"target"`
	APIKey string `json:"api_key"`
}

func (s *Server) handleAISecret(w http.ResponseWriter, r *http.Request) {
	if s.aiSettings == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "AI settings unavailable")
		return
	}
	var input aiSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	var settings aisettings.SettingsDTO
	var err error
	switch r.Method {
	case http.MethodPut:
		settings, err = s.aiSettings.PutSecret(r.Context(), input.Target, input.APIKey)
	case http.MethodDelete:
		settings, err = s.aiSettings.DeleteSecret(r.Context(), input.Target)
	default:
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, settings)
}

func (s *Server) handleAITest(w http.ResponseWriter, r *http.Request) {
	if s.aiSettings == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "AI settings unavailable")
		return
	}
	if r.Method != http.MethodPost {
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	result := s.aiSettings.Test(r.Context(), input.Target)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadGateway
	}
	writeTaskPresetJSON(w, status, result)
}
