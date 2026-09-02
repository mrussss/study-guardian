package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/motivation"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type StateManager interface {
	GetStatus() state.SystemStatus
	SetModeStudy(task string) error
	SetModeBreak() error
	SetModeOff() error
	SetTask(task string) error
	RecordFeedback(eventID, feedback string) error
}

type Server struct {
	cfg        *config.Config
	stateMgr   StateManager
	httpServer *http.Server
	mu         sync.RWMutex
	motivation MotivationManager
	aiStatus   func() interface{}
}

type MotivationManager interface {
	GetStatus(context.Context, time.Time) (motivation.Status, error)
	GetHistory(context.Context, int, time.Time) ([]motivation.HistoryDay, error)
	Achievements(context.Context, time.Time) ([]motivation.AchievementDefinition, error)
	Missions(context.Context) ([]storage.Mission, error)
	CreateMission(context.Context, string, string, int64, *string) (storage.Mission, error)
	CompleteMission(context.Context, string) (storage.Mission, bool, error)
	CancelMission(context.Context, string) error
	Rewards(context.Context) ([]storage.Reward, error)
	RedeemReward(context.Context, string) (storage.Redemption, error)
}

func NewServer(cfg *config.Config, stateMgr StateManager) *Server {
	s := &Server{
		cfg:      cfg,
		stateMgr: stateMgr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("/v1/mode/study", s.withAuth(s.handleModeStudy))
	mux.HandleFunc("/v1/mode/break", s.withAuth(s.handleModeBreak))
	mux.HandleFunc("/v1/mode/off", s.withAuth(s.handleModeOff))
	mux.HandleFunc("/v1/task", s.withAuth(s.handleTask))
	mux.HandleFunc("/v1/feedback", s.withAuth(s.handleFeedback))
	mux.HandleFunc("/v1/motivation/status", s.withAuth(s.handleMotivationStatus))
	mux.HandleFunc("/v1/motivation/history", s.withAuth(s.handleMotivationHistory))
	mux.HandleFunc("/v1/motivation/achievements", s.withAuth(s.handleMotivationAchievements))
	mux.HandleFunc("/v1/missions", s.withAuth(s.handleMissions))
	mux.HandleFunc("/v1/missions/", s.withAuth(s.handleMissionAction))
	mux.HandleFunc("/v1/rewards", s.withAuth(s.handleRewards))
	mux.HandleFunc("/v1/rewards/", s.withAuth(s.handleRewardAction))
	mux.HandleFunc("/v1/ai/status", s.withAuth(s.handleAIStatus))

	addr := fmt.Sprintf("%s:%d", cfg.IPC.SupervisorHost, cfg.IPC.SupervisorPort)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) SetMotivation(m MotivationManager) { s.motivation = m }
func (s *Server) SetAIStatus(fn func() interface{}) { s.aiStatus = fn }

func (s *Server) Start() error {
	addr := s.httpServer.Addr
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %w", addr, err)
	}
	return s.httpServer.Serve(listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedToken := s.cfg.IPC.AuthToken
		if expectedToken != "" {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"unauthorized","reason":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != expectedToken {
				http.Error(w, `{"error":"unauthorized","reason":"invalid token"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

type HealthResponse struct {
	Status          string `json:"status"`
	Service         string `json:"service"`
	ActivityWatchOK bool   `json:"activitywatch_ok"`
	ScreenSensorOK  bool   `json:"screen_sensor_ok"`
	Timestamp       string `json:"timestamp"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	st := s.stateMgr.GetStatus()
	resp := HealthResponse{
		Status:          "ok",
		Service:         "supervisor",
		ActivityWatchOK: st.ActivityWatchOK,
		ScreenSensorOK:  st.ScreenSensorOK,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	st := s.stateMgr.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

type StudyModeRequest struct {
	Task string `json:"task"`
}

func (s *Server) handleModeStudy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req StudyModeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := s.stateMgr.SetModeStudy(req.Task); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	st := s.stateMgr.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) handleModeBreak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if err := s.stateMgr.SetModeBreak(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	st := s.stateMgr.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) handleModeOff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if err := s.stateMgr.SetModeOff(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	st := s.stateMgr.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

type TaskRequest struct {
	Task string `json:"task"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	if err := s.stateMgr.SetTask(req.Task); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	st := s.stateMgr.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

type FeedbackRequest struct {
	EventID  string `json:"event_id"`
	Feedback string `json:"feedback"`
}

func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request json"}`, http.StatusBadRequest)
		return
	}

	if err := s.stateMgr.RecordFeedback(req.EventID, req.Feedback); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
