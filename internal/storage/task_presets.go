package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrTaskPresetNotFound = errors.New("task preset not found")

type TaskPreset struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Pinned     bool       `json:"pinned"`
	SortOrder  int        `json:"sort_order"`
	UseCount   int64      `json:"use_count"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func NormalizeTaskName(value string) (string, string, error) {
	name := strings.Join(strings.Fields(value), " ")
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", "", fmt.Errorf("task name must contain 1-64 characters")
	}
	return name, strings.ToLower(name), nil
}

func scanTaskPreset(scanner interface{ Scan(...any) error }) (TaskPreset, error) {
	var preset TaskPreset
	var pinned int
	err := scanner.Scan(&preset.ID, &preset.Name, &pinned, &preset.SortOrder, &preset.UseCount, &preset.LastUsedAt, &preset.CreatedAt, &preset.UpdatedAt)
	preset.Pinned = pinned != 0
	return preset, err
}

func (s *Storage) CreateTaskPreset(ctx context.Context, name string, pinned bool, sortOrder int, now time.Time) (TaskPreset, error) {
	display, key, err := NormalizeTaskName(name)
	if err != nil {
		return TaskPreset{}, err
	}
	id := fmt.Sprintf("preset-%d", now.UnixNano())
	_, err = s.db.ExecContext(ctx, `INSERT INTO task_presets
		(id, name, name_key, pinned, sort_order, use_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, id, display, key, pinned, sortOrder, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TaskPreset{}, fmt.Errorf("task preset already exists")
		}
		return TaskPreset{}, err
	}
	return s.GetTaskPreset(ctx, id)
}

func (s *Storage) GetTaskPreset(ctx context.Context, id string) (TaskPreset, error) {
	preset, err := scanTaskPreset(s.db.QueryRowContext(ctx, `SELECT id, name, pinned, sort_order, use_count, last_used_at, created_at, updated_at FROM task_presets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return TaskPreset{}, ErrTaskPresetNotFound
	}
	return preset, err
}

func (s *Storage) ListPinnedTaskPresets(ctx context.Context, limit int) ([]TaskPreset, error) {
	return s.listTaskPresets(ctx, `WHERE pinned = 1 ORDER BY sort_order ASC, COALESCE(last_used_at, created_at) DESC LIMIT ?`, limit)
}

func (s *Storage) ListRecentTaskPresets(ctx context.Context, limit int) ([]TaskPreset, error) {
	return s.listTaskPresets(ctx, `WHERE last_used_at IS NOT NULL ORDER BY last_used_at DESC LIMIT ?`, limit)
}

func (s *Storage) listTaskPresets(ctx context.Context, suffix string, limit int) ([]TaskPreset, error) {
	if limit < 1 || limit > 100 {
		limit = 8
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, pinned, sort_order, use_count, last_used_at, created_at, updated_at FROM task_presets `+suffix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TaskPreset, 0)
	for rows.Next() {
		item, scanErr := scanTaskPreset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Storage) UpdateTaskPreset(ctx context.Context, id, name string, pinned bool, sortOrder int, now time.Time) (TaskPreset, error) {
	display, key, err := NormalizeTaskName(name)
	if err != nil {
		return TaskPreset{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE task_presets SET name = ?, name_key = ?, pinned = ?, sort_order = ?, updated_at = ? WHERE id = ?`, display, key, pinned, sortOrder, now, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TaskPreset{}, fmt.Errorf("task preset already exists")
		}
		return TaskPreset{}, err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return TaskPreset{}, ErrTaskPresetNotFound
	}
	return s.GetTaskPreset(ctx, id)
}

func (s *Storage) DeleteTaskPreset(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM task_presets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ErrTaskPresetNotFound
	}
	return nil
}

func (s *Storage) RecordTaskUse(ctx context.Context, name string, now time.Time) (TaskPreset, error) {
	display, key, err := NormalizeTaskName(name)
	if err != nil {
		return TaskPreset{}, err
	}
	id := fmt.Sprintf("preset-%d", now.UnixNano())
	_, err = s.db.ExecContext(ctx, `INSERT INTO task_presets
		(id, name, name_key, pinned, sort_order, use_count, last_used_at, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, 1, ?, ?, ?)
		ON CONFLICT(name_key) DO UPDATE SET
			name = excluded.name,
			use_count = task_presets.use_count + 1,
			last_used_at = excluded.last_used_at,
			updated_at = excluded.updated_at`, id, display, key, now, now, now)
	if err != nil {
		return TaskPreset{}, err
	}
	return scanTaskPreset(s.db.QueryRowContext(ctx, `SELECT id, name, pinned, sort_order, use_count, last_used_at, created_at, updated_at FROM task_presets WHERE name_key = ?`, key))
}
