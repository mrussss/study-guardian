package storage

import (
	"context"
	"testing"
	"time"
)

func TestEyeCareStateAuditAndRequestAreAtomicAndIdempotent(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	value := EyeCareStateRecord{LocalDate: "2026-09-13", Phase: "SHORT_BREAK", FocusSegmentSeconds: 2400, Revision: 2, UpdatedAt: now}
	audit := &EyeCareAuditRecord{LocalDate: value.LocalDate, EventType: "BREAK_STARTED", Phase: value.Phase, CreatedAt: now}
	applied, err := store.SaveEyeCareState(context.Background(), value, audit, "action-once")
	if err != nil || !applied {
		t.Fatalf("first save applied=%v err=%v", applied, err)
	}
	value.Phase = "WAITING_RETURN"
	value.Revision = 3
	applied, err = store.SaveEyeCareState(context.Background(), value, nil, "action-once")
	if err != nil || applied {
		t.Fatalf("replay applied=%v err=%v", applied, err)
	}
	loaded, found, err := store.LoadEyeCareState(context.Background())
	if err != nil || !found || loaded.Phase != "SHORT_BREAK" || loaded.Revision != 2 || loaded.FocusSegmentSeconds != 2400 {
		t.Fatalf("replayed request mutated canonical state: %+v found=%v err=%v", loaded, found, err)
	}
	events, err := store.ListEyeCareAudit(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].EventType != "BREAK_STARTED" {
		t.Fatalf("audit=%+v err=%v", events, err)
	}
}
