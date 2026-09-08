package storage

import (
	"context"
	"errors"
	"time"
)

// DistractionEventRecord is a bounded, review-safe interval. The tracker is
// responsible for redacting and bounding app/title/domain before this record
// reaches storage; Storage only persists the already-sanitized DTO.
type DistractionEventRecord struct {
	ID              string
	StartedAt       time.Time
	LocalDate       string
	EndedAt         *time.Time
	DurationSeconds int64
	App             string
	Title           string
	Domain          string
	Task            string
	ReminderLevel   string
	Source          string
	Confidence      float64
	EndReason       string
}

func (s *Storage) CreateDistractionEvent(ctx context.Context, event DistractionEventRecord) error {
	if event.ID == "" || event.StartedAt.IsZero() {
		return errors.New("distraction event id and started_at are required")
	}
	if event.LocalDate == "" {
		event.LocalDate = LocalDate(event.StartedAt)
	}
	if event.ReminderLevel == "" {
		event.ReminderLevel = "NONE"
	}
	if event.Source == "" {
		event.Source = "LOCAL_RULE"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO distraction_events
		(id, started_at, local_date, ended_at, duration_seconds, app, title, domain, task, reminder_level, source, confidence, end_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at=excluded.ended_at, duration_seconds=excluded.duration_seconds,
			reminder_level=excluded.reminder_level, end_reason=excluded.end_reason`,
		event.ID, canonicalDBTime(event.StartedAt), event.LocalDate, canonicalOptionalTime(event.EndedAt), event.DurationSeconds,
		event.App, event.Title, event.Domain, event.Task, event.ReminderLevel, event.Source, event.Confidence, event.EndReason)
	return err
}

func (s *Storage) UpdateDistractionEvent(ctx context.Context, event DistractionEventRecord) error {
	if event.ID == "" {
		return errors.New("distraction event id is required")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE distraction_events SET ended_at=?, duration_seconds=?, reminder_level=?, source=?, confidence=?, end_reason=? WHERE id=?`,
		canonicalOptionalTime(event.EndedAt), event.DurationSeconds, event.ReminderLevel, event.Source, event.Confidence, event.EndReason, event.ID)
	return err
}

func (s *Storage) OpenDistractionEvent(ctx context.Context) (DistractionEventRecord, error) {
	var event DistractionEventRecord
	err := s.db.QueryRowContext(ctx, `SELECT id, started_at, local_date, ended_at, duration_seconds, app, title, domain, task, reminder_level, source, confidence, end_reason FROM distraction_events WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1`).Scan(
		&event.ID, &event.StartedAt, &event.LocalDate, &event.EndedAt, &event.DurationSeconds, &event.App, &event.Title, &event.Domain, &event.Task, &event.ReminderLevel, &event.Source, &event.Confidence, &event.EndReason)
	return event, err
}

func canonicalOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	canonical := canonicalDBTime(*value)
	return &canonical
}
