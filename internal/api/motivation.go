package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleMotivationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	v, err := s.motivation.GetStatus(context.Background(), time.Now())
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleMotivationHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	v, err := s.motivation.GetHistory(context.Background(), days, time.Now())
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleMotivationSettings(w http.ResponseWriter, r *http.Request) {
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := s.motivation.GetSettings(context.Background(), time.Now())
		if err != nil {
			jsonError(w, err, http.StatusInternalServerError)
			return
		}
		jsonOK(w, v)
	case http.MethodPut:
		var req struct {
			DailyTargetMinutes int `json:"daily_target_minutes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, fmt.Errorf("invalid request json"), http.StatusBadRequest)
			return
		}
		v, err := s.motivation.SetDailyTarget(context.Background(), req.DailyTargetMinutes, time.Now())
		if err != nil {
			jsonError(w, err, http.StatusBadRequest)
			return
		}
		jsonOK(w, v)
	default:
		methodNotAllowed(w)
	}
}
func (s *Server) handleMotivationAchievements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	v, err := s.motivation.Achievements(context.Background(), time.Now())
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleMissions(w http.ResponseWriter, r *http.Request) {
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := s.motivation.Missions(context.Background())
		if err != nil {
			jsonError(w, err, 500)
			return
		}
		jsonOK(w, v)
	case http.MethodPost:
		var req struct {
			Title         string  `json:"title"`
			Description   string  `json:"description"`
			RewardMilliAP int64   `json:"reward_milli_ap"`
			DueDate       *string `json:"due_date"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, fmt.Errorf("invalid request json"), 400)
			return
		}
		v, err := s.motivation.CreateMission(context.Background(), req.Title, req.Description, req.RewardMilliAP, req.DueDate)
		if err != nil {
			jsonError(w, err, 400)
			return
		}
		jsonOK(w, v)
	default:
		methodNotAllowed(w)
	}
}
func (s *Server) handleMissionAction(w http.ResponseWriter, r *http.Request) {
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "missions" {
		http.NotFound(w, r)
		return
	}
	id := parts[2]
	action := parts[3]
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	switch action {
	case "complete":
		m, done, err := s.motivation.CompleteMission(context.Background(), id)
		if err != nil {
			jsonError(w, err, 400)
			return
		}
		jsonOK(w, map[string]interface{}{"mission": m, "completed": done})
	case "cancel":
		if err := s.motivation.CancelMission(context.Background(), id); err != nil {
			jsonError(w, err, 400)
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) handleRewards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	v, err := s.motivation.Rewards(context.Background())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleRewardAction(w http.ResponseWriter, r *http.Request) {
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "rewards" || parts[3] != "redeem" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	v, err := s.motivation.RedeemReward(context.Background(), parts[2])
	if err != nil {
		jsonError(w, err, 400)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.motivation == nil {
		serviceUnavailable(w)
		return
	}
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := s.motivation.Events(context.Background(), afterID, limit)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, v)
}
func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.aiStatus == nil {
		jsonOK(w, map[string]interface{}{"enabled": false, "text_provider": "none", "text_configured": false, "vision_enabled": false})
		return
	}
	jsonOK(w, s.aiStatus())
}
func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}
func serviceUnavailable(w http.ResponseWriter) {
	http.Error(w, `{"error":"feature unavailable"}`, http.StatusServiceUnavailable)
}
func jsonError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
