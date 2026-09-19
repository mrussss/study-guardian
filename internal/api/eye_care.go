package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"study-guardian/internal/config"
	"study-guardian/internal/eyecare"
)

func (s *Server) handleEyeCareSettings(w http.ResponseWriter, r *http.Request) {
	if s.eyeCare == nil {
		serviceUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeTaskPresetJSON(w, http.StatusOK, s.eyeCare.Settings())
	case http.MethodPut:
		var value config.EyeCareConfig
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			jsonError(w, errors.New("invalid eye-care settings"), http.StatusBadRequest)
			return
		}
		canonical, err := s.eyeCare.SaveSettings(value)
		if err != nil {
			statusCode := http.StatusBadRequest
			if eyecare.ErrorKind(err) == "storage_unavailable" {
				statusCode = http.StatusServiceUnavailable
			}
			writeTaskPresetJSON(w, statusCode, map[string]string{"error": eyecare.ErrorKind(err), "error_kind": eyecare.ErrorKind(err)})
			return
		}
		writeTaskPresetJSON(w, http.StatusOK, canonical)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleEyeCareStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.eyeCare == nil {
		serviceUnavailable(w)
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, s.eyeCare.Status())
}

func (s *Server) handleEyeCareAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.eyeCare == nil {
		serviceUnavailable(w)
		return
	}
	var request eyecare.ActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		jsonError(w, errors.New("invalid eye-care action"), http.StatusBadRequest)
		return
	}
	request.RequestID = strings.TrimSpace(request.RequestID)
	status, err := s.eyeCare.Act(request)
	if err != nil {
		statusCode := http.StatusBadRequest
		kind := eyecare.ErrorKind(err)
		if kind == "stale_revision" || kind == "request_conflict" || kind == "request_pending" {
			statusCode = http.StatusConflict
		} else if kind == "storage_unavailable" || kind == "reconciliation_required" {
			statusCode = http.StatusServiceUnavailable
		}
		writeTaskPresetJSON(w, statusCode, map[string]string{"error": kind, "error_kind": kind})
		return
	}
	writeTaskPresetJSON(w, http.StatusOK, status)
}
