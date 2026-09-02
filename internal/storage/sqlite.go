package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

type SessionRecord struct {
	ID              string
	Mode            string
	Task            string
	StartedAt       time.Time
	EndedAt         *time.Time
	DurationSeconds int64
	EndReason       string
}

type ObservationRecord struct {
	Timestamp   time.Time
	Interaction string
	Relation    string
	Privacy     string
	Confidence  float64
	Reason      string
	CurrentMode string
	Task        string
}

type ReminderRecord struct {
	ID            string
	CreatedAt     time.Time
	Mode          string
	Level         string
	Message       string
	Reason        string
	CooldownUntil time.Time
}

func OpenSQLite(dbPath string) (*Storage, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Set connection pool settings for SQLite
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return s, nil
}

func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Storage) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS daily_state (
			date TEXT PRIMARY KEY,
			standby_seconds INTEGER NOT NULL DEFAULT 0,
			study_seconds INTEGER NOT NULL DEFAULT 0,
			break_seconds INTEGER NOT NULL DEFAULT 0,
			off_seconds INTEGER NOT NULL DEFAULT 0,
			active_seconds INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			mode TEXT NOT NULL,
			task TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMP NOT NULL,
			ended_at TIMESTAMP,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			end_reason TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL UNIQUE,
			started_at TIMESTAMP NOT NULL,
			total_study_seconds INTEGER NOT NULL DEFAULT 0,
			is_current BOOLEAN NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TIMESTAMP NOT NULL,
			interaction TEXT NOT NULL,
			relation TEXT NOT NULL,
			privacy TEXT NOT NULL,
			confidence REAL NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			current_mode TEXT NOT NULL,
			task TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS distraction_events (
			id TEXT PRIMARY KEY,
			started_at TIMESTAMP NOT NULL,
			ended_at TIMESTAMP,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			app TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT '',
			task TEXT NOT NULL DEFAULT '',
			reminder_level TEXT NOT NULL DEFAULT 'NONE'
		);`,
		`CREATE TABLE IF NOT EXISTS reminders (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMP NOT NULL,
			mode TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			reason TEXT NOT NULL,
			cooldown_until TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS feedback (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			feedback TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS classification_cache (
			cache_key TEXT PRIMARY KEY,
			relation TEXT NOT NULL,
			confidence REAL NOT NULL,
			reason TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
	}

	for _, query := range migrations {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("migration query failed (%s): %w", query, err)
		}
	}

	return nil
}

func (s *Storage) SaveSession(ctx context.Context, session SessionRecord) error {
	query := `INSERT INTO sessions (id, mode, task, started_at, ended_at, duration_seconds, end_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			mode = excluded.mode,
			task = excluded.task,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			duration_seconds = excluded.duration_seconds,
			end_reason = excluded.end_reason;`
	_, err := s.db.ExecContext(ctx, query, session.ID, session.Mode, session.Task, session.StartedAt, session.EndedAt, session.DurationSeconds, session.EndReason)
	return err
}

func (s *Storage) RecordObservation(ctx context.Context, obs ObservationRecord) error {
	query := `INSERT INTO observations (timestamp, interaction, relation, privacy, confidence, reason, current_mode, task)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := s.db.ExecContext(ctx, query, obs.Timestamp, obs.Interaction, obs.Relation, obs.Privacy, obs.Confidence, obs.Reason, obs.CurrentMode, obs.Task)
	return err
}

func (s *Storage) RecordReminder(ctx context.Context, rem ReminderRecord) error {
	query := `INSERT INTO reminders (id, created_at, mode, level, message, reason, cooldown_until)
		VALUES (?, ?, ?, ?, ?, ?, ?);`
	_, err := s.db.ExecContext(ctx, query, rem.ID, rem.CreatedAt, rem.Mode, rem.Level, rem.Message, rem.Reason, rem.CooldownUntil)
	return err
}

func (s *Storage) RecordFeedback(ctx context.Context, eventID, feedback string, t time.Time) error {
	query := `INSERT INTO feedback (event_id, feedback, created_at) VALUES (?, ?, ?);`
	_, err := s.db.ExecContext(ctx, query, eventID, feedback, t)
	return err
}

func (s *Storage) UpdateDailyState(ctx context.Context, date string, standby, study, breakSec, off, active int64, now time.Time) error {
	query := `INSERT INTO daily_state (date, standby_seconds, study_seconds, break_seconds, off_seconds, active_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			standby_seconds = excluded.standby_seconds,
			study_seconds = excluded.study_seconds,
			break_seconds = excluded.break_seconds,
			off_seconds = excluded.off_seconds,
			active_seconds = excluded.active_seconds,
			updated_at = excluded.updated_at;`
	_, err := s.db.ExecContext(ctx, query, date, standby, study, breakSec, off, active, now, now)
	return err
}

func (s *Storage) GetClassificationCache(ctx context.Context, key string, now time.Time) (string, float64, string, bool) {
	query := `SELECT relation, confidence, reason FROM classification_cache WHERE cache_key = ? AND expires_at > ?;`
	row := s.db.QueryRowContext(ctx, query, key, now)
	var relation, reason string
	var confidence float64
	if err := row.Scan(&relation, &confidence, &reason); err != nil {
		return "", 0, "", false
	}
	return relation, confidence, reason, true
}

func (s *Storage) SetClassificationCache(ctx context.Context, key, relation string, confidence float64, reason string, createdAt, expiresAt time.Time) error {
	query := `INSERT INTO classification_cache (cache_key, relation, confidence, reason, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			relation = excluded.relation,
			confidence = excluded.confidence,
			reason = excluded.reason,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at;`
	_, err := s.db.ExecContext(ctx, query, key, relation, confidence, reason, createdAt, expiresAt)
	return err
}

func (s *Storage) LoadDailyState(ctx context.Context, date string) (standby, study, breakSec, off, active int64, err error) {
	query := `SELECT standby_seconds, study_seconds, break_seconds, off_seconds, active_seconds FROM daily_state WHERE date = ?;`
	row := s.db.QueryRowContext(ctx, query, date)
	err = row.Scan(&standby, &study, &breakSec, &off, &active)
	return
}

func (s *Storage) LoadLastSession(ctx context.Context) (SessionRecord, error) {
	query := `SELECT id, mode, task, started_at, ended_at, duration_seconds, end_reason FROM sessions ORDER BY started_at DESC LIMIT 1;`
	row := s.db.QueryRowContext(ctx, query)
	var rec SessionRecord
	err := row.Scan(&rec.ID, &rec.Mode, &rec.Task, &rec.StartedAt, &rec.EndedAt, &rec.DurationSeconds, &rec.EndReason)
	return rec, err
}

// LoadOpenSession returns the most recently persisted session that was not
// closed. It is intentionally separate from LoadLastSession: a normal
// restart must recover only an interrupted session, never a previously
// completed mode.
func (s *Storage) LoadOpenSession(ctx context.Context) (SessionRecord, error) {
	query := `SELECT id, mode, task, started_at, ended_at, duration_seconds, end_reason
		FROM sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1;`
	row := s.db.QueryRowContext(ctx, query)
	var rec SessionRecord
	err := row.Scan(&rec.ID, &rec.Mode, &rec.Task, &rec.StartedAt, &rec.EndedAt, &rec.DurationSeconds, &rec.EndReason)
	return rec, err
}

// CloseOpenSessions marks every interrupted session closed. Older versions
// could leave more than one open row after repeated restarts, so recovery must
// clean up the full set rather than just the newest row.
func (s *Storage) CloseOpenSessions(ctx context.Context, endedAt time.Time, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET ended_at = ?, end_reason = ? WHERE ended_at IS NULL`, endedAt, reason)
	return err
}

func (s *Storage) CountOpenSessions(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE ended_at IS NULL`)
	var count int
	err := row.Scan(&count)
	return count, err
}
