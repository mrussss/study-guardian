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
	ID                 string
	Mode               string
	Task               string
	StartedAt          time.Time
	LocalDate          string
	EndedAt            *time.Time
	DurationSeconds    int64
	EndReason          string
	ModeOrigin         string
	PauseReason        string
	AutoResumeEligible bool
}

type ObservationRecord struct {
	Timestamp   time.Time
	LocalDate   string
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
	LocalDate     string
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
			local_date TEXT NOT NULL DEFAULT '',
			ended_at TIMESTAMP,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			end_reason TEXT NOT NULL DEFAULT ''
			,mode_origin TEXT NOT NULL DEFAULT 'MANUAL'
			,pause_reason TEXT NOT NULL DEFAULT 'NONE'
			,auto_resume_eligible BOOLEAN NOT NULL DEFAULT 0
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
			local_date TEXT NOT NULL DEFAULT '',
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
			local_date TEXT NOT NULL DEFAULT '',
			ended_at TIMESTAMP,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			app TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT '',
			task TEXT NOT NULL DEFAULT '',
			reminder_level TEXT NOT NULL DEFAULT 'NONE',
			source TEXT NOT NULL DEFAULT 'LOCAL_RULE',
			confidence REAL NOT NULL DEFAULT 0,
			end_reason TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS reminders (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMP NOT NULL,
			local_date TEXT NOT NULL DEFAULT '',
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
			activity TEXT NOT NULL DEFAULT '',
			topic TEXT NOT NULL DEFAULT '',
			subtopic TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			progress_signal TEXT NOT NULL DEFAULT 'UNKNOWN',
			source_kind TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS motivation_daily (
			date TEXT PRIMARY KEY,
			credited_focus_seconds INTEGER NOT NULL DEFAULT 0,
			daily_target_seconds INTEGER NOT NULL DEFAULT 7200,
			checkin_completed BOOLEAN NOT NULL DEFAULT 0,
			target_completed BOOLEAN NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS motivation_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			daily_target_seconds INTEGER NOT NULL DEFAULT 7200,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS motivation_comeback_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_distraction_at TIMESTAMP,
			focus_seconds_since_distraction INTEGER NOT NULL DEFAULT 0,
			active BOOLEAN NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS ap_ledger (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			reference_id TEXT NOT NULL,
			delta_milli_ap INTEGER NOT NULL,
			created_at TIMESTAMP NOT NULL,
			local_date TEXT NOT NULL DEFAULT '',
			UNIQUE(source, reference_id)
		);`,
		`CREATE TABLE IF NOT EXISTS missions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			reward_milli_ap INTEGER NOT NULL DEFAULT 0,
			due_date TEXT,
			status TEXT NOT NULL DEFAULT 'OPEN',
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			linked_task_preset_id TEXT,
			linked_task_name TEXT,
			link_source TEXT NOT NULL DEFAULT '',
			link_confidence REAL
		);`,
		`CREATE TABLE IF NOT EXISTS achievements (
			achievement_id TEXT PRIMARY KEY,
			unlocked_at TIMESTAMP NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS reward_catalog (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			cost_milli_ap INTEGER NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS reward_redemptions (
			id TEXT PRIMARY KEY,
			reward_id TEXT NOT NULL,
			reward_name TEXT NOT NULL,
			cost_milli_ap INTEGER NOT NULL,
			redeemed_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS ui_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			message TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS chat_conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT NOT NULL,
			external_conversation_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			capture_policy TEXT NOT NULL DEFAULT 'AUTO',
			first_seen_at TIMESTAMP NOT NULL,
			last_seen_at TIMESTAMP NOT NULL,
			UNIQUE(platform, external_conversation_id)
		);`,
		`CREATE TABLE IF NOT EXISTS chat_turns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL,
			external_turn_id TEXT,
			turn_key TEXT NOT NULL,
			observed_at TIMESTAMP NOT NULL,
			local_date TEXT NOT NULL,
			mode_at_start TEXT NOT NULL,
			task_at_start TEXT NOT NULL DEFAULT '',
			eligible_for_review BOOLEAN NOT NULL DEFAULT 0,
			active_branch_key TEXT NOT NULL DEFAULT '',
			finalized BOOLEAN NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES chat_conversations(id),
			UNIQUE(conversation_id, turn_key)
		);`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			turn_id INTEGER NOT NULL,
			external_message_id TEXT,
			role TEXT NOT NULL,
			branch_key TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			observed_at TIMESTAMP NOT NULL,
			finalized_at TIMESTAMP,
			ingested_at TIMESTAMP NOT NULL,
			is_final BOOLEAN NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT 1,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			FOREIGN KEY(turn_id) REFERENCES chat_turns(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turns_local_date ON chat_turns(local_date);`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_turn ON chat_messages(turn_id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_message_external ON chat_messages(turn_id, external_message_id) WHERE external_message_id IS NOT NULL;`,
		`CREATE TABLE IF NOT EXISTS semantic_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			observed_at TIMESTAMP NOT NULL,
			local_date TEXT NOT NULL,
			task TEXT NOT NULL DEFAULT '',
			app TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			domain TEXT NOT NULL DEFAULT '',
			relation TEXT NOT NULL,
			confidence REAL NOT NULL,
			activity TEXT NOT NULL DEFAULT '',
			topic TEXT NOT NULL DEFAULT '',
			subtopic TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			progress_signal TEXT NOT NULL DEFAULT 'UNKNOWN',
			reason TEXT NOT NULL DEFAULT '',
			source_kind TEXT NOT NULL,
			window_fingerprint TEXT NOT NULL DEFAULT '',
			screen_hash TEXT NOT NULL DEFAULT '',
			first_observed_at TIMESTAMP,
			last_observed_at TIMESTAMP,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			stable_interval_seconds INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_semantic_snapshots_local_date ON semantic_snapshots(local_date);`,
		`CREATE TABLE IF NOT EXISTS review_exclusions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			UNIQUE(date, source_type, source_id)
		);`,
		`CREATE TABLE IF NOT EXISTS daily_reviews (
			date TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			generation_mode TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL DEFAULT 0,
			input_hash TEXT NOT NULL DEFAULT '',
			schema_version INTEGER NOT NULL DEFAULT 1,
			prompt_version TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			review_json TEXT NOT NULL DEFAULT '',
			markdown TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			started_at TIMESTAMP,
			generated_at TIMESTAMP,
			updated_at TIMESTAMP NOT NULL,
			error_code TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS task_presets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			name_key TEXT NOT NULL,
			pinned INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			use_count INTEGER NOT NULL DEFAULT 0,
			last_used_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_task_presets_name_key ON task_presets(name_key);`,
		`INSERT OR IGNORE INTO reward_catalog (id, name, type, cost_milli_ap, description, enabled) VALUES
			('game-15', '游戏 15 分钟', 'TIME', 250, '给自己一个短暂的游戏奖励', 1),
			('game-30', '游戏 30 分钟', 'TIME', 500, '给自己一个半小时奖励', 1),
			('game-60', '游戏 60 分钟', 'TIME', 1000, '给自己一个小时奖励', 1),
			('milk-tea', '一杯奶茶', 'LIFE', 1000, '现实生活中的小奖励', 1),
			('movie', '电影 / 小聚会', 'LIFE', 2000, '完成目标后的休闲安排', 1);`,
	}

	for _, query := range migrations {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("migration query failed (%s): %w", query, err)
		}
	}

	if err := s.ensureMissionColumns(); err != nil {
		return err
	}
	if err := s.ensureSemanticSnapshotColumns(); err != nil {
		return err
	}
	if err := s.ensureSessionColumns(); err != nil {
		return err
	}
	if err := s.ensureDistractionEventColumns(); err != nil {
		return err
	}
	if err := s.ensureClassificationCacheColumns(); err != nil {
		return err
	}
	return s.ensureDailyEvidenceDateColumns()
}

func (s *Storage) ensureSessionColumns() error {
	columns := []struct{ name, definition string }{
		{"mode_origin", "TEXT NOT NULL DEFAULT 'MANUAL'"},
		{"pause_reason", "TEXT NOT NULL DEFAULT 'NONE'"},
		{"auto_resume_eligible", "BOOLEAN NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := s.ensureColumn("sessions", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) ensureSemanticSnapshotColumns() error {
	columns := []struct{ name, definition string }{
		{"topic", "TEXT NOT NULL DEFAULT ''"},
		{"subtopic", "TEXT NOT NULL DEFAULT ''"},
		{"action", "TEXT NOT NULL DEFAULT ''"},
		{"progress_signal", "TEXT NOT NULL DEFAULT 'UNKNOWN'"},
		{"window_fingerprint", "TEXT NOT NULL DEFAULT ''"},
		{"screen_hash", "TEXT NOT NULL DEFAULT ''"},
		{"first_observed_at", "TIMESTAMP"},
		{"last_observed_at", "TIMESTAMP"},
		{"duration_seconds", "INTEGER NOT NULL DEFAULT 0"},
		{"stable_interval_seconds", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := s.ensureColumn("semantic_snapshots", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) ensureDistractionEventColumns() error {
	columns := []struct{ name, definition string }{
		{"source", "TEXT NOT NULL DEFAULT 'LOCAL_RULE'"},
		{"confidence", "REAL NOT NULL DEFAULT 0"},
		{"end_reason", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := s.ensureColumn("distraction_events", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) ensureClassificationCacheColumns() error {
	columns := []struct{ name, definition string }{
		{"activity", "TEXT NOT NULL DEFAULT ''"},
		{"topic", "TEXT NOT NULL DEFAULT ''"},
		{"subtopic", "TEXT NOT NULL DEFAULT ''"},
		{"action", "TEXT NOT NULL DEFAULT ''"},
		{"progress_signal", "TEXT NOT NULL DEFAULT 'UNKNOWN'"},
		{"source_kind", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := s.ensureColumn("classification_cache", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

// ensureDailyEvidenceDateColumns upgrades databases created before explicit
// local_date columns existed. The ALTER statements are additive and safe to
// repeat. Backfill is deliberately best-effort: an unparseable legacy value
// is left untouched rather than deleted or assigned a guessed date.
func (s *Storage) ensureDailyEvidenceDateColumns() error {
	tables := []struct {
		table string
		key   string
		time  string
	}{
		{table: "sessions", key: "id", time: "started_at"},
		{table: "observations", key: "id", time: "timestamp"},
		{table: "distraction_events", key: "id", time: "started_at"},
		{table: "reminders", key: "id", time: "created_at"},
		{table: "ap_ledger", key: "id", time: "created_at"},
	}
	for _, item := range tables {
		if err := s.ensureColumn(item.table, "local_date", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		rows, err := s.db.Query(`SELECT ` + item.key + `, ` + item.time + ` FROM ` + item.table + ` WHERE local_date = ''`)
		if err != nil {
			return err
		}
		type legacyRow struct{ id, value any }
		var pending []legacyRow
		for rows.Next() {
			var id any
			var raw any
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, legacyRow{id: id, value: raw})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, row := range pending {
			value, ok := parseStoredDBTime(row.value)
			if !ok {
				continue
			}
			date := LocalDate(value)
			if date == "" {
				continue
			}
			if _, err := s.db.Exec(`UPDATE `+item.table+` SET local_date = ? WHERE `+item.key+` = ? AND local_date = ''`, date, row.id); err != nil {
				return err
			}
		}
		if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_` + item.table + `_local_date ON ` + item.table + `(local_date)`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if found {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func (s *Storage) ensureMissionColumns() error {
	rows, err := s.db.Query(`PRAGMA table_info(missions)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	columns := []struct{ name, definition string }{
		{"linked_task_preset_id", "TEXT"}, {"linked_task_name", "TEXT"}, {"link_source", "TEXT NOT NULL DEFAULT ''"}, {"link_confidence", "REAL"},
	}
	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE missions ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) SaveSession(ctx context.Context, session SessionRecord) error {
	if session.LocalDate == "" {
		session.LocalDate = LocalDate(session.StartedAt)
	}
	query := `INSERT INTO sessions (id, mode, task, started_at, local_date, ended_at, duration_seconds, end_reason, mode_origin, pause_reason, auto_resume_eligible)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			mode = excluded.mode,
			task = excluded.task,
			started_at = excluded.started_at,
			local_date = excluded.local_date,
			ended_at = excluded.ended_at,
			duration_seconds = excluded.duration_seconds,
			end_reason = excluded.end_reason,
			mode_origin = excluded.mode_origin,
			pause_reason = excluded.pause_reason,
			auto_resume_eligible = excluded.auto_resume_eligible;`
	var endedAt *time.Time
	if session.EndedAt != nil {
		value := canonicalDBTime(*session.EndedAt)
		endedAt = &value
	}
	if session.ModeOrigin == "" {
		session.ModeOrigin = "MANUAL"
	}
	if session.PauseReason == "" {
		session.PauseReason = "NONE"
	}
	_, err := s.db.ExecContext(ctx, query, session.ID, session.Mode, session.Task, canonicalDBTime(session.StartedAt), session.LocalDate, endedAt, session.DurationSeconds, session.EndReason, session.ModeOrigin, session.PauseReason, session.AutoResumeEligible)
	return err
}

func (s *Storage) UpdateOpenSessionTask(ctx context.Context, sessionID, task string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET task = ? WHERE id = ? AND ended_at IS NULL`, task, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("open session not found")
	}
	return nil
}

func (s *Storage) RecordObservation(ctx context.Context, obs ObservationRecord) error {
	if obs.LocalDate == "" {
		obs.LocalDate = LocalDate(obs.Timestamp)
	}
	query := `INSERT INTO observations (timestamp, local_date, interaction, relation, privacy, confidence, reason, current_mode, task)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := s.db.ExecContext(ctx, query, canonicalDBTime(obs.Timestamp), obs.LocalDate, obs.Interaction, obs.Relation, obs.Privacy, obs.Confidence, obs.Reason, obs.CurrentMode, obs.Task)
	return err
}

func (s *Storage) RecordReminder(ctx context.Context, rem ReminderRecord) error {
	if rem.LocalDate == "" {
		rem.LocalDate = LocalDate(rem.CreatedAt)
	}
	query := `INSERT INTO reminders (id, created_at, local_date, mode, level, message, reason, cooldown_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := s.db.ExecContext(ctx, query, rem.ID, canonicalDBTime(rem.CreatedAt), rem.LocalDate, rem.Mode, rem.Level, rem.Message, rem.Reason, canonicalDBTime(rem.CooldownUntil))
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

type ClassificationCacheRecord struct {
	Relation       string
	Confidence     float64
	Reason         string
	Activity       string
	Topic          string
	Subtopic       string
	Action         string
	ProgressSignal string
	SourceKind     string
}

func (s *Storage) GetClassificationCacheRecord(ctx context.Context, key string, now time.Time) (ClassificationCacheRecord, bool) {
	query := `SELECT relation, confidence, reason, activity, topic, subtopic, action, progress_signal, source_kind FROM classification_cache WHERE cache_key = ? AND expires_at > ?;`
	row := s.db.QueryRowContext(ctx, query, key, canonicalDBTime(now))
	var record ClassificationCacheRecord
	if err := row.Scan(&record.Relation, &record.Confidence, &record.Reason, &record.Activity, &record.Topic, &record.Subtopic, &record.Action, &record.ProgressSignal, &record.SourceKind); err != nil {
		return ClassificationCacheRecord{}, false
	}
	return record, true
}

func (s *Storage) GetClassificationCache(ctx context.Context, key string, now time.Time) (string, float64, string, bool) {
	record, found := s.GetClassificationCacheRecord(ctx, key, now)
	if !found {
		return "", 0, "", false
	}
	return record.Relation, record.Confidence, record.Reason, true
}

func (s *Storage) SetClassificationCacheRecord(ctx context.Context, key string, record ClassificationCacheRecord, createdAt, expiresAt time.Time) error {
	if record.ProgressSignal == "" {
		record.ProgressSignal = "UNKNOWN"
	}
	query := `INSERT INTO classification_cache
		(cache_key, relation, confidence, reason, activity, topic, subtopic, action, progress_signal, source_kind, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			relation = excluded.relation,
			confidence = excluded.confidence,
			reason = excluded.reason,
			activity = excluded.activity,
			topic = excluded.topic,
			subtopic = excluded.subtopic,
			action = excluded.action,
			progress_signal = excluded.progress_signal,
			source_kind = excluded.source_kind,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at;`
	_, err := s.db.ExecContext(ctx, query, key, record.Relation, record.Confidence, record.Reason, record.Activity, record.Topic, record.Subtopic, record.Action, record.ProgressSignal, record.SourceKind, canonicalDBTime(createdAt), canonicalDBTime(expiresAt))
	return err
}

func (s *Storage) SetClassificationCache(ctx context.Context, key, relation string, confidence float64, reason string, createdAt, expiresAt time.Time) error {
	return s.SetClassificationCacheRecord(ctx, key, ClassificationCacheRecord{Relation: relation, Confidence: confidence, Reason: reason}, createdAt, expiresAt)
}

func (s *Storage) LoadDailyState(ctx context.Context, date string) (standby, study, breakSec, off, active int64, err error) {
	query := `SELECT standby_seconds, study_seconds, break_seconds, off_seconds, active_seconds FROM daily_state WHERE date = ?;`
	row := s.db.QueryRowContext(ctx, query, date)
	err = row.Scan(&standby, &study, &breakSec, &off, &active)
	return
}

func (s *Storage) LoadLastSession(ctx context.Context) (SessionRecord, error) {
	query := `SELECT id, mode, task, started_at, local_date, ended_at, duration_seconds, end_reason, mode_origin, pause_reason, auto_resume_eligible FROM sessions ORDER BY started_at DESC LIMIT 1;`
	row := s.db.QueryRowContext(ctx, query)
	var rec SessionRecord
	err := row.Scan(&rec.ID, &rec.Mode, &rec.Task, &rec.StartedAt, &rec.LocalDate, &rec.EndedAt, &rec.DurationSeconds, &rec.EndReason, &rec.ModeOrigin, &rec.PauseReason, &rec.AutoResumeEligible)
	return rec, err
}

// LoadOpenSession returns the most recently persisted session that was not
// closed. It is intentionally separate from LoadLastSession: a normal
// restart must recover only an interrupted session, never a previously
// completed mode.
func (s *Storage) LoadOpenSession(ctx context.Context) (SessionRecord, error) {
	query := `SELECT id, mode, task, started_at, local_date, ended_at, duration_seconds, end_reason, mode_origin, pause_reason, auto_resume_eligible
		FROM sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1;`
	row := s.db.QueryRowContext(ctx, query)
	var rec SessionRecord
	err := row.Scan(&rec.ID, &rec.Mode, &rec.Task, &rec.StartedAt, &rec.LocalDate, &rec.EndedAt, &rec.DurationSeconds, &rec.EndReason, &rec.ModeOrigin, &rec.PauseReason, &rec.AutoResumeEligible)
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
