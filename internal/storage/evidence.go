package storage

import (
	"context"
	"time"
)

type DistractionEvidenceRecord struct {
	ID              string
	StartedAt       time.Time
	EndedAt         *time.Time
	DurationSeconds int64
	App             string
	Title           string
	Domain          string
	Task            string
	ReminderLevel   string
}

type ChatTurnEvidenceRecord struct {
	ID                     int64
	TurnKey                string
	ObservedAt             time.Time
	LocalDate              string
	TaskAtStart            string
	EligibleForReview      bool
	ActiveBranchKey        string
	Finalized              bool
	ExternalConversationID string
	ConversationTitle      string
	CapturePolicy          string
	UserContent            string
	AssistantContent       string
}

func (s *Storage) ListSessionsForDate(ctx context.Context, date string) ([]SessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, mode, task, started_at, ended_at, duration_seconds, end_reason FROM sessions WHERE date(started_at) = ? ORDER BY started_at`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var record SessionRecord
		if err := rows.Scan(&record.ID, &record.Mode, &record.Task, &record.StartedAt, &record.EndedAt, &record.DurationSeconds, &record.EndReason); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Storage) ListDistractionsForDate(ctx context.Context, date string) ([]DistractionEvidenceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, started_at, ended_at, duration_seconds, app, title, domain, task, reminder_level FROM distraction_events WHERE date(started_at) = ? ORDER BY started_at`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DistractionEvidenceRecord
	for rows.Next() {
		var record DistractionEvidenceRecord
		if err := rows.Scan(&record.ID, &record.StartedAt, &record.EndedAt, &record.DurationSeconds, &record.App, &record.Title, &record.Domain, &record.Task, &record.ReminderLevel); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Storage) ListRemindersForDate(ctx context.Context, date string) ([]ReminderRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, mode, level, message, reason, cooldown_until FROM reminders WHERE date(created_at) = ? ORDER BY created_at`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReminderRecord
	for rows.Next() {
		var record ReminderRecord
		if err := rows.Scan(&record.ID, &record.CreatedAt, &record.Mode, &record.Level, &record.Message, &record.Reason, &record.CooldownUntil); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Storage) ListChatTurnsForDate(ctx context.Context, date string) ([]ChatTurnEvidenceRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.turn_key, t.observed_at, t.local_date, t.task_at_start, t.eligible_for_review, t.active_branch_key, t.finalized, c.external_conversation_id, c.title, c.capture_policy FROM chat_turns t JOIN chat_conversations c ON c.id = t.conversation_id WHERE t.local_date = ? ORDER BY t.observed_at, t.id`, date)
	if err != nil {
		return nil, err
	}
	var out []ChatTurnEvidenceRecord
	for rows.Next() {
		var record ChatTurnEvidenceRecord
		if err := rows.Scan(&record.ID, &record.TurnKey, &record.ObservedAt, &record.LocalDate, &record.TaskAtStart, &record.EligibleForReview, &record.ActiveBranchKey, &record.Finalized, &record.ExternalConversationID, &record.ConversationTitle, &record.CapturePolicy); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return nil, rowsErr
	}
	for index := range out {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT content FROM chat_messages WHERE turn_id = ? AND role = 'user' ORDER BY observed_at, id LIMIT 1), ''), COALESCE((SELECT content FROM chat_messages WHERE turn_id = ? AND role = 'assistant' AND is_active = 1 ORDER BY observed_at DESC, id DESC LIMIT 1), '')`, out[index].ID, out[index].ID).Scan(&out[index].UserContent, &out[index].AssistantContent); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Storage) ListSemanticSnapshotsForDate(ctx context.Context, date string) ([]SemanticSnapshotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT observed_at, local_date, task, app, title, domain, relation, confidence, activity, reason, source_kind, metadata_json FROM semantic_snapshots WHERE local_date = ? ORDER BY observed_at, id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SemanticSnapshotRecord
	for rows.Next() {
		var record SemanticSnapshotRecord
		if err := rows.Scan(&record.ObservedAt, &record.LocalDate, &record.Task, &record.App, &record.Title, &record.Domain, &record.Relation, &record.Confidence, &record.Activity, &record.Reason, &record.SourceKind, &record.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Storage) ListReviewExclusionsForDate(ctx context.Context, date string) ([]ReviewExclusionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date, source_type, source_id, created_at FROM review_exclusions WHERE date = ? ORDER BY id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewExclusionRecord
	for rows.Next() {
		var record ReviewExclusionRecord
		if err := rows.Scan(&record.Date, &record.SourceType, &record.SourceID, &record.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Storage) HasDailyState(ctx context.Context, date string) bool {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM daily_state WHERE date = ? LIMIT 1`, date).Scan(&exists)
	return err == nil && exists == 1
}
