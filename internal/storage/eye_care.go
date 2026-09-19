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
	DueGeneration              int64
	NotifiedGeneration         int64
	NotifiedAt                 *time.Time
	Revision                   int64
	UpdatedAt                  time.Time
}

type EyeCareRequestRecord struct {
	RequestID      string
	Action         string
	Status         string
	ResultRevision int64
	ResultJSON     string
	ErrorKind      string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

type EyeCareAuditRecord struct {
	LocalDate string
	EventType string
	Phase     string
	CreatedAt time.Time
}

func (s *Storage) LoadEyeCareState(ctx context.Context) (EyeCareStateRecord, bool, error) {
	var value EyeCareStateRecord
	var breakStarted, plannedEnd, dueAt, snoozeUntil, notifiedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT local_date, phase, focus_segment_seconds,
		focus_since_long_break_seconds, break_started_at, planned_break_end_at, due_at,
		snooze_until, snooze_count, completed_short_breaks, completed_long_breaks,
		retry_focus_after_seconds, due_generation, notified_generation, notified_at,
		revision, updated_at FROM eye_care_state WHERE id = 1`).Scan(
		&value.LocalDate, &value.Phase, &value.FocusSegmentSeconds, &value.FocusSinceLongBreakSeconds,
		&breakStarted, &plannedEnd, &dueAt, &snoozeUntil, &value.SnoozeCount,
		&value.CompletedShortBreaks, &value.CompletedLongBreaks, &value.RetryFocusAfterSeconds,
		&value.DueGeneration, &value.NotifiedGeneration, &notifiedAt, &value.Revision, &value.UpdatedAt,
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
	value.NotifiedAt = nullableTime(notifiedAt)
	return value, true, nil
}

// SaveEyeCareState persists status and audit atomically. requestID is retained
// for compatibility with the first schema; new actions use the request ledger.
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
	if err := saveEyeCareStateTx(ctx, tx, value); err != nil {
		return false, err
	}
	if audit != nil {
		if err := saveEyeCareAuditTx(ctx, tx, *audit); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// SaveEyeCareSettingsAndState commits the setting and its canonical runtime
// state together so a successful settings response can never describe a
// partially-persisted configuration.
func (s *Storage) SaveEyeCareSettingsAndState(ctx context.Context, key, raw string, now time.Time, value EyeCareStateRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, raw, canonicalDBTime(now)); err != nil {
		return err
	}
	if err := saveEyeCareStateTx(ctx, tx, value); err != nil {
		return err
	}
	return tx.Commit()
}

// ClaimEyeCareNotification atomically grants one notification for the current
// due generation. Callers must check quiet-hours/snooze before claiming.
func (s *Storage) ClaimEyeCareNotification(ctx context.Context, generation int64, now time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE eye_care_state SET notified_generation=?, notified_at=?
		WHERE id=1 AND due_generation=? AND notified_generation < due_generation
		AND phase IN ('SHORT_BREAK_DUE','LONG_BREAK_DUE') AND snooze_until IS NULL`, generation, canonicalDBTime(now), generation)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

type eyeCareSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func saveEyeCareStateTx(ctx context.Context, exec eyeCareSQLExecutor, value EyeCareStateRecord) error {
	_, err := exec.ExecContext(ctx, `INSERT INTO eye_care_state(
		id, local_date, phase, focus_segment_seconds, focus_since_long_break_seconds,
		break_started_at, planned_break_end_at, due_at, snooze_until, snooze_count,
		completed_short_breaks, completed_long_breaks, retry_focus_after_seconds,
		due_generation, notified_generation, notified_at, revision, updated_at
	) VALUES(1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET local_date=excluded.local_date, phase=excluded.phase,
		focus_segment_seconds=excluded.focus_segment_seconds,
		focus_since_long_break_seconds=excluded.focus_since_long_break_seconds,
		break_started_at=excluded.break_started_at, planned_break_end_at=excluded.planned_break_end_at,
		due_at=excluded.due_at, snooze_until=excluded.snooze_until, snooze_count=excluded.snooze_count,
		completed_short_breaks=excluded.completed_short_breaks,
		completed_long_breaks=excluded.completed_long_breaks,
		retry_focus_after_seconds=excluded.retry_focus_after_seconds,
		due_generation=excluded.due_generation, notified_generation=excluded.notified_generation,
		notified_at=excluded.notified_at,
		revision=excluded.revision, updated_at=excluded.updated_at`,
		value.LocalDate, value.Phase, value.FocusSegmentSeconds, value.FocusSinceLongBreakSeconds,
		value.BreakStartedAt, value.PlannedBreakEndAt, value.DueAt, value.SnoozeUntil,
		value.SnoozeCount, value.CompletedShortBreaks, value.CompletedLongBreaks,
		value.RetryFocusAfterSeconds, value.DueGeneration, value.NotifiedGeneration, value.NotifiedAt,
		value.Revision, canonicalDBTime(value.UpdatedAt))
	return err
}

func saveEyeCareAuditTx(ctx context.Context, exec eyeCareSQLExecutor, audit EyeCareAuditRecord) error {
	_, err := exec.ExecContext(ctx, `INSERT INTO eye_care_audit(local_date, event_type, phase, created_at) VALUES(?, ?, ?, ?)`,
		audit.LocalDate, audit.EventType, audit.Phase, canonicalDBTime(audit.CreatedAt))
	return err
}

func (s *Storage) BeginEyeCareRequest(ctx context.Context, value EyeCareRequestRecord) (EyeCareRequestRecord, bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO eye_care_requests
		(request_id, action, status, result_revision, result_json, error_kind, created_at, completed_at)
		VALUES(?, ?, 'PENDING', ?, ?, '', ?, NULL)`, value.RequestID, value.Action, value.ResultRevision,
		value.ResultJSON, canonicalDBTime(value.CreatedAt))
	if err != nil {
		return EyeCareRequestRecord{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return EyeCareRequestRecord{}, false, err
	}
	if rows == 1 {
		value.Status = "PENDING"
		return value, true, nil
	}
	existing, found, err := s.GetEyeCareRequest(ctx, value.RequestID)
	if err != nil || !found {
		return EyeCareRequestRecord{}, false, err
	}
	return existing, false, nil
}

func (s *Storage) GetEyeCareRequest(ctx context.Context, requestID string) (EyeCareRequestRecord, bool, error) {
	var value EyeCareRequestRecord
	var completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT request_id, action, status, result_revision, result_json,
		error_kind, created_at, completed_at FROM eye_care_requests WHERE request_id = ?`, requestID).Scan(
		&value.RequestID, &value.Action, &value.Status, &value.ResultRevision, &value.ResultJSON,
		&value.ErrorKind, &value.CreatedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EyeCareRequestRecord{}, false, nil
	}
	if err != nil {
		return EyeCareRequestRecord{}, false, err
	}
	value.CompletedAt = nullableTime(completedAt)
	return value, true, nil
}

func (s *Storage) CompleteEyeCareRequest(ctx context.Context, requestID, action string, value EyeCareStateRecord,
	audit *EyeCareAuditRecord, resultJSON string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveEyeCareStateTx(ctx, tx, value); err != nil {
		return err
	}
	if audit != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO eye_care_audit(local_date, event_type, phase, created_at) VALUES(?, ?, ?, ?)`, audit.LocalDate, audit.EventType, audit.Phase, canonicalDBTime(audit.CreatedAt)); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE eye_care_requests SET status='APPLIED', result_revision=?, result_json=?, error_kind='', completed_at=?
		WHERE request_id=? AND action=? AND status='PENDING'`, value.Revision, resultJSON, canonicalDBTime(now), requestID, action)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("eye-care action request is not pending")
	}
	return tx.Commit()
}

func (s *Storage) FailEyeCareRequest(ctx context.Context, requestID, action, errorKind, resultJSON string, revision int64, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE eye_care_requests SET status='FAILED', result_revision=?, result_json=?, error_kind=?, completed_at=?
		WHERE request_id=? AND action=? AND status='PENDING'`, revision, resultJSON, errorKind, canonicalDBTime(now), requestID, action)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("eye-care action request is not pending")
	}
	return nil
}

func (s *Storage) ListPendingEyeCareRequests(ctx context.Context) ([]EyeCareRequestRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id, action, status, result_revision, result_json, error_kind, created_at, completed_at
		FROM eye_care_requests WHERE status='PENDING' ORDER BY created_at, request_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EyeCareRequestRecord
	for rows.Next() {
		var value EyeCareRequestRecord
		var completedAt sql.NullTime
		if err := rows.Scan(&value.RequestID, &value.Action, &value.Status, &value.ResultRevision, &value.ResultJSON,
			&value.ErrorKind, &value.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		value.CompletedAt = nullableTime(completedAt)
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Storage) PruneEyeCareRequests(ctx context.Context, now time.Time) error {
	cutoff := canonicalDBTime(now.Add(-90 * 24 * time.Hour))
	if _, err := s.db.ExecContext(ctx, `DELETE FROM eye_care_requests WHERE status!='PENDING' AND created_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM eye_care_requests WHERE status!='PENDING' AND request_id IN (
		SELECT request_id FROM eye_care_requests WHERE status!='PENDING' ORDER BY created_at DESC, request_id DESC LIMIT -1 OFFSET 10000)`)
	return err
}

// TransitionSession atomically closes the current session, optionally bumps
// its study evidence revision, and creates the canonical next session.
func (s *Storage) TransitionSession(ctx context.Context, closed, opened SessionRecord, bumpStudyEvidence bool, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if closed.ID != "" {
		if err := saveSessionRecord(ctx, tx, closed); err != nil {
			return err
		}
		if bumpStudyEvidence && closed.Mode == "STUDY" {
			date := closed.LocalDate
			if date == "" {
				date = LocalDate(closed.StartedAt)
			}
			if date == "" {
				return errors.New("study session local date is required")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO evidence_revisions(date, revision, updated_at)
				VALUES (?, 1, ?) ON CONFLICT(date) DO UPDATE SET revision=evidence_revisions.revision+1, updated_at=excluded.updated_at`,
				date, canonicalDBTime(now)); err != nil {
				return err
			}
		}
	}
	if err := saveSessionRecord(ctx, tx, opened); err != nil {
		return err
	}
	return tx.Commit()
}

func saveSessionRecord(ctx context.Context, exec eyeCareSQLExecutor, session SessionRecord) error {
	if session.LocalDate == "" {
		session.LocalDate = LocalDate(session.StartedAt)
	}
	if session.ModeOrigin == "" {
		session.ModeOrigin = "MANUAL"
	}
	if session.PauseReason == "" {
		session.PauseReason = "NONE"
	}
	var endedAt *time.Time
	if session.EndedAt != nil {
		value := canonicalDBTime(*session.EndedAt)
		endedAt = &value
	}
	_, err := exec.ExecContext(ctx, `INSERT INTO sessions
		(id, mode, task, started_at, local_date, ended_at, duration_seconds, end_reason, mode_origin, pause_reason, auto_resume_eligible)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET mode=excluded.mode, task=excluded.task, started_at=excluded.started_at,
		local_date=excluded.local_date, ended_at=excluded.ended_at, duration_seconds=excluded.duration_seconds,
		end_reason=excluded.end_reason, mode_origin=excluded.mode_origin, pause_reason=excluded.pause_reason,
		auto_resume_eligible=excluded.auto_resume_eligible`, session.ID, session.Mode, session.Task,
		canonicalDBTime(session.StartedAt), session.LocalDate, endedAt, session.DurationSeconds,
		session.EndReason, session.ModeOrigin, session.PauseReason, session.AutoResumeEligible)
	return err
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
