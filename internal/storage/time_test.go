package storage

import (
	"context"
	"testing"
	"time"
)

func TestLocalDateIsComputedFromOriginalInstant(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	defer func() { time.Local = previous }()

	instant := time.Date(2026, 9, 8, 23, 59, 59, 123456789, time.Local)
	withMonotonic := instant.Add(0)
	if got := LocalDate(withMonotonic); got != "2026-09-08" {
		t.Fatalf("LocalDate=%q", got)
	}
	if got := canonicalDBTime(withMonotonic); got.Location() != time.UTC || got.Nanosecond() != 123456789 {
		t.Fatalf("canonical time=%v", got)
	}
}

func TestEvidenceWritesPersistExplicitLocalDate(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	defer func() { time.Local = previous }()
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 8, 23, 59, 59, 987654321, time.Local)
	if err := store.SaveSession(context.Background(), SessionRecord{ID: "s1", Mode: "STUDY", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListSessionsForDate(context.Background(), "2026-09-08")
	if err != nil || len(items) != 1 || items[0].LocalDate != "2026-09-08" {
		t.Fatalf("sessions=%+v err=%v", items, err)
	}
	if _, err := store.db.Exec(`INSERT INTO sessions(id,mode,task,started_at,local_date) VALUES('legacy','STUDY','', 'not-a-time','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`SELECT id FROM sessions WHERE local_date = ''`); err != nil {
		t.Fatal(err)
	}
}

func TestParseLegacyGoTimeStringWithMonotonicSuffix(t *testing.T) {
	parsed, ok := parseStoredDBTime("2026-09-08 16:10:55.459449600 +0800 CST m=+525.143675301")
	if !ok {
		t.Fatal("legacy Go time string should be parsed")
	}
	if got := LocalDate(parsed); got != "2026-09-08" {
		t.Fatalf("local date=%q, want 2026-09-08", got)
	}
}
