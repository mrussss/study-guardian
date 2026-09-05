package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"study-guardian/internal/storage"
)

type taskPresetListResponse struct {
	Pinned []storage.TaskPreset `json:"pinned"`
	Recent []storage.TaskPreset `json:"recent"`
}

type taskPresetMutation struct {
	Name      string `json:"name"`
	Pinned    bool   `json:"pinned"`
	SortOrder int    `json:"sort_order"`
}

func (s *Server) handleTaskPresets(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		pinned, err := s.store.ListPinnedTaskPresets(r.Context(), 8)
		if err != nil {
			writeTaskPresetError(w, http.StatusInternalServerError, "storage unavailable")
			return
		}
		recent, err := s.store.ListRecentTaskPresets(r.Context(), 6)
		if err != nil {
			writeTaskPresetError(w, http.StatusInternalServerError, "storage unavailable")
			return
		}
		writeTaskPresetJSON(w, http.StatusOK, taskPresetListResponse{Pinned: pinned, Recent: recent})
	case http.MethodPost:
		var input taskPresetMutation
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
			return
		}
		preset, err := s.store.CreateTaskPreset(r.Context(), input.Name, input.Pinned, input.SortOrder, time.Now())
		if err != nil {
			writeTaskPresetStoreError(w, err)
			return
		}
		writeTaskPresetJSON(w, http.StatusCreated, preset)
	default:
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleTaskPresetAction(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeTaskPresetError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/task-presets/"), "/")
	parts := strings.Split(relative, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeTaskPresetError(w, http.StatusNotFound, "task preset not found")
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "select" {
		if r.Method != http.MethodPost {
			writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		preset, err := s.store.GetTaskPreset(r.Context(), id)
		if err != nil {
			writeTaskPresetStoreError(w, err)
			return
		}
		if err := s.stateMgr.SetTask(preset.Name); err != nil {
			writeTaskPresetError(w, http.StatusConflict, "task update failed")
			return
		}
		preset, err = s.store.RecordTaskUse(r.Context(), preset.Name, time.Now())
		if err != nil {
			writeTaskPresetError(w, http.StatusInternalServerError, "storage unavailable")
			return
		}
		writeTaskPresetJSON(w, http.StatusOK, map[string]any{"preset": preset, "status": s.stateMgr.GetStatus()})
		return
	}
	if len(parts) != 1 {
		writeTaskPresetError(w, http.StatusNotFound, "task preset not found")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input taskPresetMutation
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeTaskPresetError(w, http.StatusBadRequest, "invalid request json")
			return
		}
		preset, err := s.store.UpdateTaskPreset(r.Context(), id, input.Name, input.Pinned, input.SortOrder, time.Now())
		if err != nil {
			writeTaskPresetStoreError(w, err)
			return
		}
		writeTaskPresetJSON(w, http.StatusOK, preset)
	case http.MethodDelete:
		if err := s.store.DeleteTaskPreset(r.Context(), id); err != nil {
			writeTaskPresetStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeTaskPresetError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeTaskPresetStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrTaskPresetNotFound) {
		writeTaskPresetError(w, http.StatusNotFound, "task preset not found")
		return
	}
	if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "1-64") {
		writeTaskPresetError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTaskPresetError(w, http.StatusInternalServerError, "storage unavailable")
}

func writeTaskPresetError(w http.ResponseWriter, status int, message string) {
	writeTaskPresetJSON(w, status, map[string]string{"error": message})
}

func writeTaskPresetJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
