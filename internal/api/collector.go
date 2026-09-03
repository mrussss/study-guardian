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

type CollectorTurnRequest struct {
	Platform               string                    `json:"platform"`
	ExternalConversationID string                    `json:"external_conversation_id"`
	Title                  string                    `json:"title"`
	URL                    string                    `json:"url"`
	CapturePolicy          string                    `json:"capture_policy"`
	ExternalTurnID         string                    `json:"external_turn_id"`
	TurnKey                string                    `json:"turn_key"`
	ObservedAt             string                    `json:"observed_at"`
	ModeAtStart            string                    `json:"mode_at_start"`
	TaskAtStart            string                    `json:"task_at_start"`
	EligibleForReview      bool                      `json:"eligible_for_review"`
	ActiveBranchKey        string                    `json:"active_branch_key"`
	Finalized              bool                      `json:"finalized"`
	Messages               []CollectorMessageRequest `json:"messages"`
	Message                *CollectorMessageRequest  `json:"message,omitempty"`
}

type CollectorMessageRequest struct {
	ExternalMessageID string `json:"external_message_id"`
	Role              string `json:"role"`
	BranchKey         string `json:"branch_key"`
	Content           string `json:"content"`
	ContentHash       string `json:"content_hash"`
	ObservedAt        string `json:"observed_at"`
	FinalizedAt       string `json:"finalized_at,omitempty"`
	IsFinal           bool   `json:"is_final"`
	IsActive          bool   `json:"is_active"`
	MetadataJSON      string `json:"metadata_json"`
}

type CollectorHeartbeatRequest struct {
	CollectorVersion string `json:"collector_version"`
	ParserVersion    string `json:"parser_version"`
	QueueDepth       int    `json:"queue_depth"`
	LastErrorCode    string `json:"last_error_code"`
}

func (s *Server) handleCollectorContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	status := s.stateMgr.GetStatus()
	jsonOK(w, map[string]any{
		"user_mode":   status.UserMode,
		"task":        status.Task,
		"server_time": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handleCollectorTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CollectorTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, fmt.Errorf("invalid collector turn JSON"), http.StatusBadRequest)
		return
	}
	if req.Message != nil {
		req.Messages = append(req.Messages, *req.Message)
	}
	if err := s.ingestCollectorTurn(r.Context(), req); err != nil {
		jsonError(w, err, http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"accepted": true, "turn_key": req.TurnKey})
}

func (s *Server) handleCollectorMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CollectorTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, fmt.Errorf("invalid collector message JSON"), http.StatusBadRequest)
		return
	}
	if req.Message == nil {
		jsonError(w, fmt.Errorf("collector message is required"), http.StatusBadRequest)
		return
	}
	req.Messages = []CollectorMessageRequest{*req.Message}
	if err := s.ingestCollectorTurn(r.Context(), req); err != nil {
		jsonError(w, err, http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"accepted": true, "turn_key": req.TurnKey})
}

func (s *Server) ingestCollectorTurn(ctx context.Context, req CollectorTurnRequest) error {
	if s.store == nil {
		return fmt.Errorf("collector storage is unavailable")
	}
	if req.CapturePolicy == "" {
		req.CapturePolicy = "AUTO"
	}
	if req.CapturePolicy != "AUTO" && req.CapturePolicy != "ALWAYS_INCLUDE" && req.CapturePolicy != "ALWAYS_EXCLUDE" {
		return fmt.Errorf("invalid capture_policy %q", req.CapturePolicy)
	}
	observedAt, err := parseCollectorTime(req.ObservedAt)
	if err != nil {
		return fmt.Errorf("invalid observed_at: %w", err)
	}
	mode := strings.TrimSpace(req.ModeAtStart)
	if mode == "" {
		return fmt.Errorf("mode_at_start is required")
	}
	messages := make([]storage.ChatMessageRecord, 0, len(req.Messages))
	for _, message := range req.Messages {
		messageObservedAt := observedAt
		if message.ObservedAt != "" {
			messageObservedAt, err = parseCollectorTime(message.ObservedAt)
			if err != nil {
				return fmt.Errorf("invalid message observed_at: %w", err)
			}
		}
		var finalizedAt *time.Time
		if message.FinalizedAt != "" {
			value, parseErr := parseCollectorTime(message.FinalizedAt)
			if parseErr != nil {
				return fmt.Errorf("invalid message finalized_at: %w", parseErr)
			}
			finalizedAt = &value
		}
		messages = append(messages, storage.ChatMessageRecord{
			ExternalMessageID: message.ExternalMessageID, Role: message.Role, BranchKey: message.BranchKey,
			Content: message.Content, ContentHash: message.ContentHash, ObservedAt: messageObservedAt,
			FinalizedAt: finalizedAt, IsFinal: message.IsFinal, IsActive: message.IsActive, MetadataJSON: message.MetadataJSON,
		})
	}
	turn := storage.ChatTurnRecord{
		ExternalTurnID: req.ExternalTurnID, TurnKey: req.TurnKey, ObservedAt: observedAt,
		LocalDate: observedAt.In(time.Local).Format("2006-01-02"), ModeAtStart: mode, TaskAtStart: req.TaskAtStart,
		EligibleForReview: req.EligibleForReview && strings.EqualFold(mode, "STUDY"), ActiveBranchKey: req.ActiveBranchKey, Finalized: req.Finalized,
	}
	_, err = s.store.IngestChatTurn(ctx, storage.ChatConversationRecord{
		Platform: req.Platform, ExternalConversationID: req.ExternalConversationID, Title: req.Title, URL: req.URL,
		CapturePolicy: req.CapturePolicy, ObservedAt: observedAt,
	}, turn, messages, time.Now())
	if err == nil && s.review != nil {
		if _, staleErr := s.review.MarkStaleIfChanged(ctx, turn.LocalDate); staleErr != nil {
			return staleErr
		}
	}
	return err
}

func (s *Server) handleCollectorHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CollectorHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, fmt.Errorf("invalid collector heartbeat JSON"), http.StatusBadRequest)
		return
	}
	if req.QueueDepth < 0 {
		jsonError(w, fmt.Errorf("queue_depth must not be negative"), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{
		"status": "CONNECTED", "collector_version": req.CollectorVersion, "parser_version": req.ParserVersion,
		"queue_depth": req.QueueDepth, "server_time": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func parseCollectorTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Now(), nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return value, nil
}
