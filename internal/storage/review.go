package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SemanticSnapshotRecord struct {
	ID           int64     `json:"id"`
	ObservedAt   time.Time `json:"observed_at"`
	LocalDate    string    `json:"local_date"`
	Task         string    `json:"task"`
	App          string    `json:"app"`
	Title        string    `json:"title"`
	Domain       string    `json:"domain"`
	Relation     string    `json:"relation"`
	Confidence   float64   `json:"confidence"`
	Activity     string    `json:"activity"`
	Reason       string    `json:"reason"`
	SourceKind   string    `json:"source_kind"`
	MetadataJSON string    `json:"metadata_json"`
}

type ReviewExclusionRecord struct {
	Date       string    `json:"date"`
	SourceType string    `json:"source_type"`
	SourceID   string    `json:"source_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type DailyReviewRecord struct {
	Date           string     `json:"date"`
	Status         string     `json:"status"`
	GenerationMode string     `json:"generation_mode"`
	Revision       int        `json:"revision"`
	InputHash      string     `json:"input_hash"`
	SchemaVersion  int        `json:"schema_version"`
	PromptVersion  string     `json:"prompt_version"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	ReviewJSON     string     `json:"review_json"`
	Markdown       string     `json:"markdown"`
	AttemptCount   int        `json:"attempt_count"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	GeneratedAt    *time.Time `json:"generated_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ErrorCode      string     `json:"error_code"`
}

func (s *Storage) RecordSemanticSnapshot(ctx context.Context, record SemanticSnapshotRecord) (int64, error) {
	if record.ObservedAt.IsZero() || record.LocalDate == "" || record.Relation == "" || record.SourceKind == "" {
		return 0, errors.New("semantic snapshot observed_at, local_date, relation and source_kind are required")
	}
	if record.MetadataJSON == "" {
		record.MetadataJSON = "{}"
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO semantic_snapshots
		(observed_at, local_date, task, app, title, domain, relation, confidence, activity, reason, source_kind, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.ObservedAt, record.LocalDate, record.Task, record.App, record.Title, record.Domain, record.Relation, record.Confidence, record.Activity, record.Reason, record.SourceKind, record.MetadataJSON)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Storage) AddReviewExclusion(ctx context.Context, record ReviewExclusionRecord) error {
	if record.Date == "" || record.SourceType == "" || record.SourceID == "" {
		return errors.New("review exclusion date, source_type and source_id are required")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO review_exclusions (date, source_type, source_id, created_at) VALUES (?, ?, ?, ?)`, record.Date, record.SourceType, record.SourceID, record.CreatedAt)
	return err
}

func (s *Storage) SaveDailyReview(ctx context.Context, record DailyReviewRecord) error {
	if record.Date == "" || record.Status == "" {
		return errors.New("daily review date and status are required")
	}
	if record.SchemaVersion <= 0 {
		record.SchemaVersion = 1
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO daily_reviews
		(date, status, generation_mode, revision, input_hash, schema_version, prompt_version, provider, model, review_json, markdown, attempt_count, started_at, generated_at, updated_at, error_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			status = excluded.status, generation_mode = excluded.generation_mode, revision = excluded.revision,
			input_hash = excluded.input_hash, schema_version = excluded.schema_version, prompt_version = excluded.prompt_version,
			provider = excluded.provider, model = excluded.model, review_json = excluded.review_json, markdown = excluded.markdown,
			attempt_count = excluded.attempt_count, started_at = excluded.started_at, generated_at = excluded.generated_at,
			updated_at = excluded.updated_at, error_code = excluded.error_code`,
		record.Date, record.Status, record.GenerationMode, record.Revision, record.InputHash, record.SchemaVersion, record.PromptVersion,
		record.Provider, record.Model, record.ReviewJSON, record.Markdown, record.AttemptCount, record.StartedAt, record.GeneratedAt, record.UpdatedAt, record.ErrorCode)
	return err
}

func (s *Storage) LoadDailyReview(ctx context.Context, date string) (DailyReviewRecord, error) {
	var record DailyReviewRecord
	err := s.db.QueryRowContext(ctx, `SELECT date, status, generation_mode, revision, input_hash, schema_version, prompt_version, provider, model, review_json, markdown, attempt_count, started_at, generated_at, updated_at, error_code FROM daily_reviews WHERE date = ?`, date).Scan(
		&record.Date, &record.Status, &record.GenerationMode, &record.Revision, &record.InputHash, &record.SchemaVersion, &record.PromptVersion,
		&record.Provider, &record.Model, &record.ReviewJSON, &record.Markdown, &record.AttemptCount, &record.StartedAt, &record.GeneratedAt, &record.UpdatedAt, &record.ErrorCode)
	if err != nil {
		return DailyReviewRecord{}, fmt.Errorf("load daily review: %w", err)
	}
	return record, nil
}

func (s *Storage) DeleteDailyReview(ctx context.Context, date string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM daily_reviews WHERE date = ?`, date)
	return err
}

func (s *Storage) MarkDailyReviewStale(ctx context.Context, date string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE daily_reviews SET status = 'STALE', updated_at = ? WHERE date = ? AND status = 'READY'`, now, date)
	return err
}

// MarkDailyReviewReady publishes a review only after its Markdown artifact has
// been written successfully. The revision/status predicate prevents a stale
// or superseded generation from publishing over a newer pending attempt.
func (s *Storage) MarkDailyReviewReady(ctx context.Context, date string, revision int, generatedAt, updatedAt time.Time, errorCode string) error {
	if date == "" || revision <= 0 {
		return errors.New("daily review date and positive revision are required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE daily_reviews
		SET status = 'READY', generated_at = ?, updated_at = ?, error_code = ?
		WHERE date = ? AND revision = ? AND status = 'PENDING'`,
		generatedAt, updatedAt, errorCode, date, revision)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("daily review pending revision not found: %s/%d", date, revision)
	}
	return nil
}

func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
