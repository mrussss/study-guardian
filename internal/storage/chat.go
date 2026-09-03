package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ChatConversationRecord struct {
	Platform               string
	ExternalConversationID string
	Title                  string
	URL                    string
	CapturePolicy          string
	ObservedAt             time.Time
}

type ChatTurnRecord struct {
	ExternalTurnID    string
	TurnKey           string
	ObservedAt        time.Time
	LocalDate         string
	ModeAtStart       string
	TaskAtStart       string
	EligibleForReview bool
	ActiveBranchKey   string
	Finalized         bool
}

type ChatMessageRecord struct {
	ExternalMessageID string
	Role              string
	BranchKey         string
	Content           string
	ContentHash       string
	ObservedAt        time.Time
	FinalizedAt       *time.Time
	IsFinal           bool
	IsActive          bool
	MetadataJSON      string
}

// IngestChatTurn atomically upserts a conversation, its turn, and all supplied
// messages. observed_at/local_date come from the collector payload; ingested_at
// is deliberately generated at the Supervisor boundary.
func (s *Storage) IngestChatTurn(ctx context.Context, conversation ChatConversationRecord, turn ChatTurnRecord, messages []ChatMessageRecord, ingestedAt time.Time) (int64, error) {
	if strings.TrimSpace(conversation.Platform) == "" || strings.TrimSpace(conversation.ExternalConversationID) == "" {
		return 0, errors.New("chat conversation platform and external id are required")
	}
	if strings.TrimSpace(turn.TurnKey) == "" || strings.TrimSpace(turn.ModeAtStart) == "" {
		return 0, errors.New("chat turn key and mode_at_start are required")
	}
	if conversation.CapturePolicy == "" {
		conversation.CapturePolicy = "AUTO"
	}
	if turn.LocalDate == "" {
		return 0, errors.New("chat turn local_date is required")
	}
	if conversation.ObservedAt.IsZero() {
		conversation.ObservedAt = ingestedAt
	}
	if turn.ObservedAt.IsZero() {
		turn.ObservedAt = conversation.ObservedAt
	}
	if ingestedAt.IsZero() {
		ingestedAt = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO chat_conversations
		(platform, external_conversation_id, title, url, capture_policy, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, external_conversation_id) DO UPDATE SET
			title = CASE WHEN excluded.title <> '' THEN excluded.title ELSE chat_conversations.title END,
			url = CASE WHEN excluded.url <> '' THEN excluded.url ELSE chat_conversations.url END,
			capture_policy = CASE WHEN excluded.capture_policy <> '' THEN excluded.capture_policy ELSE chat_conversations.capture_policy END,
			last_seen_at = CASE WHEN excluded.last_seen_at > chat_conversations.last_seen_at THEN excluded.last_seen_at ELSE chat_conversations.last_seen_at END;`,
		conversation.Platform, conversation.ExternalConversationID, conversation.Title, conversation.URL,
		conversation.CapturePolicy, conversation.ObservedAt, conversation.ObservedAt)
	if err != nil {
		return 0, fmt.Errorf("upsert chat conversation: %w", err)
	}
	var conversationID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE platform = ? AND external_conversation_id = ?`, conversation.Platform, conversation.ExternalConversationID).Scan(&conversationID); err != nil {
		return 0, fmt.Errorf("resolve chat conversation: %w", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO chat_turns
		(conversation_id, external_turn_id, turn_key, observed_at, local_date, mode_at_start, task_at_start, eligible_for_review, active_branch_key, finalized, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id, turn_key) DO UPDATE SET
			external_turn_id = CASE WHEN excluded.external_turn_id IS NOT NULL AND excluded.external_turn_id <> '' THEN excluded.external_turn_id ELSE chat_turns.external_turn_id END,
			active_branch_key = CASE WHEN excluded.active_branch_key <> '' THEN excluded.active_branch_key ELSE chat_turns.active_branch_key END,
			finalized = CASE WHEN excluded.finalized THEN 1 ELSE chat_turns.finalized END,
			updated_at = excluded.updated_at;`,
		conversationID, nullableString(turn.ExternalTurnID), turn.TurnKey, turn.ObservedAt, turn.LocalDate,
		turn.ModeAtStart, turn.TaskAtStart, turn.EligibleForReview, turn.ActiveBranchKey, turn.Finalized, ingestedAt, ingestedAt)
	if err != nil {
		return 0, fmt.Errorf("upsert chat turn: %w", err)
	}
	var turnID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM chat_turns WHERE conversation_id = ? AND turn_key = ?`, conversationID, turn.TurnKey).Scan(&turnID); err != nil {
		return 0, fmt.Errorf("resolve chat turn: %w", err)
	}

	for _, message := range messages {
		if strings.TrimSpace(message.Role) == "" {
			return 0, errors.New("chat message role is required")
		}
		if message.ContentHash == "" {
			hash := sha256.Sum256([]byte(message.Content))
			message.ContentHash = hex.EncodeToString(hash[:])
		}
		if message.ObservedAt.IsZero() {
			message.ObservedAt = turn.ObservedAt
		}
		if message.MetadataJSON == "" {
			message.MetadataJSON = "{}"
		}
		if message.ExternalMessageID != "" {
			var messageID int64
			err := tx.QueryRowContext(ctx, `SELECT id FROM chat_messages WHERE turn_id = ? AND external_message_id = ?`, turnID, message.ExternalMessageID).Scan(&messageID)
			if errors.Is(err, sql.ErrNoRows) {
				_, err = tx.ExecContext(ctx, `INSERT INTO chat_messages
					(turn_id, external_message_id, role, branch_key, content, content_hash, observed_at, finalized_at, ingested_at, is_final, is_active, metadata_json)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, turnID, message.ExternalMessageID, message.Role, message.BranchKey, message.Content, message.ContentHash, message.ObservedAt, message.FinalizedAt, ingestedAt, message.IsFinal, message.IsActive, message.MetadataJSON)
			} else if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE chat_messages SET role = ?, branch_key = ?, content = ?, content_hash = ?, finalized_at = ?, is_final = ?, is_active = ?, metadata_json = ?, ingested_at = ? WHERE id = ?`, message.Role, message.BranchKey, message.Content, message.ContentHash, message.FinalizedAt, message.IsFinal, message.IsActive, message.MetadataJSON, ingestedAt, messageID)
			}
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO chat_messages
				(turn_id, role, branch_key, content, content_hash, observed_at, finalized_at, ingested_at, is_final, is_active, metadata_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, turnID, message.Role, message.BranchKey, message.Content, message.ContentHash, message.ObservedAt, message.FinalizedAt, ingestedAt, message.IsFinal, message.IsActive, message.MetadataJSON)
		}
		if err != nil {
			return 0, fmt.Errorf("upsert chat message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return turnID, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Storage) LoadChatTurn(ctx context.Context, turnKey string) (ChatTurnRecord, error) {
	var record ChatTurnRecord
	var externalTurnID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT external_turn_id, turn_key, observed_at, local_date, mode_at_start, task_at_start, eligible_for_review, active_branch_key, finalized FROM chat_turns WHERE turn_key = ? ORDER BY id DESC LIMIT 1`, turnKey).Scan(
		&externalTurnID, &record.TurnKey, &record.ObservedAt, &record.LocalDate, &record.ModeAtStart, &record.TaskAtStart, &record.EligibleForReview, &record.ActiveBranchKey, &record.Finalized)
	if err != nil {
		return ChatTurnRecord{}, fmt.Errorf("load chat turn: %w", err)
	}
	record.ExternalTurnID = externalTurnID.String
	return record, nil
}
