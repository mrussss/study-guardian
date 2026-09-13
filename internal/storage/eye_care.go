package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type EyeCareStateRecord struct {
	LocalDate                  string
	Phase                      string
	FocusSegmentSeconds        int64
	FocusSinceLongBreakSeconds int64
	BreakStartedAt             *time.Time
	PlannedBreakEndAt          *time.Time
	DueAt                      *time.Time
	SnoozeUntil                *time.Time
	SnoozeCount                int
	CompletedShortBreaks       int
	CompletedLongBreaks        int
	RetryFocusAfterSeconds     int64
	Revision                   int64
	UpdatedAt                  time.Time
}

type EyeCareAuditRecord struct {
	LocalDate string
	EventType string
	Phase     string
	CreatedAt time.Time
}

func (s *Storage) LoadEyeCareState(ctx context.Context) (EyeCareStateRecord, bool, error) {
	var value EyeCareStateRecord
	var breakStarted, plannedEnd, dueAt, snoozeUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT local_date, phase, focus_segment_seconds,
		focus_since_long_break_seconds, break_started_at, planned_break_end_at, due_at,
		snooze_until, snooze_count, completed_short_breaks, completed_long_breaks,
		retry_focus_after_seconds, revision, updated_at FROM eye_care_state WHERE id = 1`).Scan(
		&value.LocalDate, &value.Phase, &value.FocusSegmentSeconds, &value.FocusSinceLongBreakSeconds,
		&breakStarted, &plannedEnd, &dueAt, &snoozeUntil, &value.SnoozeCount,
		&value.CompletedShortBreaks, &value.CompletedLongBreaks, &value.RetryFocusAfterSeconds,
		&value.Revision, &value.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EyeCareStateRecord{}, false, nil
	}
	if err != nil {
		return EyeCareStateRecord{}, false, err
	}
	value.BreakStartedAt = nullableTime(breakStarted)
	value.PlannedBreakEndAt = nullableTime(plannedEnd)
	value.DueAt = nullableTime(dueAt)
	value.SnoozeUntil = nullableTime(snoozeUntil)
	return value, true, nil
}

// SaveEyeCareState persists status, audit and action idempotency atomically.
// A repeated non-empty request ID returns applied=false and cannot replay the
// state transition.
func (s *Storage) SaveEyeCareState(ctx context.Context, value EyeCareStateRecord, audit *EyeCareAuditRecord, requestID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if requestID != "" {
		result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO eye_care_requests(request_id, created_at) VALUES(?, ?)`, requestID, value.UpdatedAt)
		if err != nil {
			return false, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if rows == 0 {
			return false, tx.Commit()
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO eye_care_state(
		id, local_date, phase, focus_segment_seconds, focus_since_long_break_seconds,
		break_started_at, planned_break_end_at, due_at, snooze_until, snooze_count,
		completed_short_breaks, completed_long_breaks, retry_focus_after_seconds, revision, updated_at
	) VALUES(1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET local_date=excluded.local_date, phase=excluded.phase,
		focus_segment_seconds=excluded.focus_segment_seconds,
		focus_since_long_break_seconds=excluded.focus_since_long_break_seconds,
		break_started_at=excluded.break_started_at, planned_break_end_at=excluded.planned_break_end_at,
		due_at=excluded.due_at, snooze_until=excluded.snooze_until, snooze_count=excluded.snooze_count,
		completed_short_breaks=excluded.completed_short_breaks,
		completed_long_breaks=excluded.completed_long_breaks,
		retry_focus_after_seconds=excluded.retry_focus_after_seconds,
		revision=excluded.revision, updated_at=excluded.updated_at`,
		value.LocalDate, value.Phase, value.FocusSegmentSeconds, value.FocusSinceLongBreakSeconds,
		value.BreakStartedAt, value.PlannedBreakEndAt, value.DueAt, value.SnoozeUntil,
		value.SnoozeCount, value.CompletedShortBreaks, value.CompletedLongBreaks,
		value.RetryFocusAfterSeconds, value.Revision, value.UpdatedAt)
	if err != nil {
		return false, err
	}
	if audit != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO eye_care_audit(local_date, event_type, phase, created_at) VALUES(?, ?, ?, ?)`, audit.LocalDate, audit.EventType, audit.Phase, audit.CreatedAt); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Storage) HasEyeCareRequest(ctx context.Context, requestID string) (bool, error) {
	if requestID == "" {
		return false, nil
	}
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM eye_care_requests WHERE request_id = ?`, requestID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Storage) ListEyeCareAudit(ctx context.Context, limit int) ([]EyeCareAuditRecord, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT local_date, event_type, phase, created_at FROM eye_care_audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EyeCareAuditRecord
	for rows.Next() {
		var item EyeCareAuditRecord
		if err := rows.Scan(&item.LocalDate, &item.EventType, &item.Phase, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
