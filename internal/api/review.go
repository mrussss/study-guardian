package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"study-guardian/internal/storage"
)

func (s *Server) handleReviewDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if s.review == nil {
		serviceUnavailable(w)
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if r.Method == http.MethodDelete {
		if err := s.review.Delete(r.Context(), date); err != nil {
			jsonError(w, err, http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"deleted": true, "date": date})
		return
	}
	record, err := s.review.Get(r.Context(), date)
	if err != nil {
		if isNotFoundReview(err) {
			jsonError(w, fmt.Errorf("review not found"), http.StatusNotFound)
			return
		}
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, record)
}

func (s *Server) handleReviewGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.review == nil {
		serviceUnavailable(w)
		return
	}
	var request struct {
		Date string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err.Error() != "EOF" {
		jsonError(w, fmt.Errorf("invalid review generate JSON"), http.StatusBadRequest)
		return
	}
	date := strings.TrimSpace(request.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	record, err := s.review.Generate(context.Background(), date)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, record)
}

func (s *Server) handleReviewExclude(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.review == nil {
		serviceUnavailable(w)
		return
	}
	var request struct {
		Date       string `json:"date"`
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		jsonError(w, fmt.Errorf("invalid review exclusion JSON"), http.StatusBadRequest)
		return
	}
	if request.Date == "" {
		request.Date = time.Now().Format("2006-01-02")
	}
	if err := s.review.Exclude(r.Context(), request.Date, request.SourceType, request.SourceID); err != nil {
		jsonError(w, err, http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"accepted": true})
}

func (s *Server) handleReviewEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.review == nil {
		serviceUnavailable(w)
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	bundle, err := s.review.Evidence(r.Context(), date)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, bundle)
}

func isNotFoundReview(err error) bool { return storage.IsNotFound(err) }
