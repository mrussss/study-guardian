package storage

import (
	"context"
	"testing"
	"time"
)

func TestTaskPresetsNormalizeRankAndPreserveSessionHistory(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	goPreset, err := store.CreateTaskPreset(ctx, " Go ", true, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTaskPreset(ctx, "  go  ", false, 0, now); err == nil {
		t.Fatal("case-insensitive duplicate should fail")
	}
	if err := store.SaveSession(ctx, SessionRecord{ID: "history", Mode: "STUDY", Task: "Go", StartedAt: now, EndedAt: ptrTime(now.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTaskPreset(ctx, goPreset.ID, "算法", true, 1, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordTaskUse(ctx, "数据库", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordTaskUse(ctx, "算法", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	recent, err := store.ListRecentTaskPresets(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Name != "算法" || recent[0].UseCount != 1 {
		t.Fatalf("unexpected recent ranking: %+v", recent)
	}
	last, err := store.LoadLastSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if last.Task != "Go" {
		t.Fatalf("preset rename rewrote history: %q", last.Task)
	}
	if err := store.DeleteTaskPreset(ctx, goPreset.ID); err != nil {
		t.Fatal(err)
	}
	last, _ = store.LoadLastSession(ctx)
	if last.Task != "Go" {
		t.Fatalf("preset deletion rewrote history: %q", last.Task)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestNormalizeTaskNameUsesWhitespaceAndLatinCase(t *testing.T) {
	display, key, err := NormalizeTaskName("  RepoLens\t API  ")
	if err != nil || display != "RepoLens API" || key != "repolens api" {
		t.Fatalf("display=%q key=%q err=%v", display, key, err)
	}
}
