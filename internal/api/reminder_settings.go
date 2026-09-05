package api

import (
	"encoding/json"
	"net/http"
	"time"

	"study-guardian/internal/config"
)

const reminderSettingKey = "reminder.config.v1"

type ReminderSettingsManager interface {
	GetSettings() config.ReminderConfig
	SetSettings(config.ReminderConfig) error
}

func (s *Server) SetReminderSettings(manager ReminderSettingsManager) { s.reminderSettings = manager }

func (s *Server) handleReminderSettings(w http.ResponseWriter, r *http.Request) {
	if s.reminderSettings == nil || s.store == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "reminder settings unavailable")
		return
	}
	if r.Method == http.MethodGet {
		writeTaskPresetJSON(w, http.StatusOK, s.reminderSettings.GetSettings())
		return
	}
	if r.Method != http.MethodPut {
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input config.ReminderConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	if input.CooldownMinutes < 1 || input.CooldownMinutes > 1440 {
		writeTaskPresetError(w, http.StatusBadRequest, "cooldown_minutes must be 1-1440")
		return
	}
	if _, err := config.ParseQuietPeriods(input.QuietPeriods); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(input)
	if err := s.store.SetSetting(r.Context(), reminderSettingKey, string(raw), time.Now()); err != nil {
		writeTaskPresetError(w, http.StatusInternalServerError, "settings persistence failed")
		return
	}
	if err := s.reminderSettings.SetSettings(input); err != nil {
		writeTaskPresetError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, s.reminderSettings.GetSettings())
}
