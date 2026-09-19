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

func TestEyeCareRequestCompletionAndSettingsTransactionsRollbackAtomically(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	initial := EyeCareStateRecord{LocalDate: "2026-09-13", Phase: "SHORT_BREAK_DUE", DueGeneration: 1, Revision: 1, UpdatedAt: now}
	if _, err := store.SaveEyeCareState(context.Background(), initial, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.BeginEyeCareRequest(context.Background(), EyeCareRequestRecord{
		RequestID: "atomic-start", Action: "START_SHORT_BREAK", Status: "PENDING", ResultJSON: `{"phase":"SHORT_BREAK"}`, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_eye_care_audit BEFORE INSERT ON eye_care_audit BEGIN SELECT RAISE(FAIL, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	changed := initial
	changed.Phase = "SHORT_BREAK"
	changed.Revision++
	audit := &EyeCareAuditRecord{LocalDate: initial.LocalDate, EventType: "BREAK_STARTED", Phase: changed.Phase, CreatedAt: now}
	if err := store.CompleteEyeCareRequest(context.Background(), "atomic-start", "START_SHORT_BREAK", changed, audit, `{"phase":"SHORT_BREAK"}`, now); err == nil {
		t.Fatal("injected audit failure was hidden")
	}
	loaded, found, err := store.LoadEyeCareState(context.Background())
	if err != nil || !found || loaded.Phase != "SHORT_BREAK_DUE" || loaded.Revision != 1 {
		t.Fatalf("failed request partially changed state: %+v found=%v err=%v", loaded, found, err)
	}
	request, found, err := store.GetEyeCareRequest(context.Background(), "atomic-start")
	if err != nil || !found || request.Status != "PENDING" {
		t.Fatalf("failed request ledger update was not rolled back: %+v found=%v err=%v", request, found, err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER fail_eye_care_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_eye_care_state BEFORE UPDATE ON eye_care_state BEGIN SELECT RAISE(FAIL, 'injected state failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEyeCareSettingsAndState(context.Background(), "eye_care.config.v1", `{"enabled":true}`, now, changed); err == nil {
		t.Fatal("injected settings/state transaction failure was hidden")
	}
	if _, found, err := store.GetSetting(context.Background(), "eye_care.config.v1"); err != nil || found {
		t.Fatalf("settings write escaped failed transaction: found=%v err=%v", found, err)
	}
	loaded, _, err = store.LoadEyeCareState(context.Background())
	if err != nil || loaded.Phase != "SHORT_BREAK_DUE" {
		t.Fatalf("settings transaction partially changed state: %+v err=%v", loaded, err)
	}
}

func TestEyeCareRequestRetentionPreservesPendingReconciliation(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, _, err := store.BeginEyeCareRequest(context.Background(), EyeCareRequestRecord{RequestID: "old-pending", Action: "SNOOZE", Status: "PENDING", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneEyeCareRequests(context.Background(), time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	request, found, err := store.GetEyeCareRequest(context.Background(), "old-pending")
	if err != nil || !found || request.Status != "PENDING" {
		t.Fatalf("pending reconciliation was pruned: %+v found=%v err=%v", request, found, err)
	}
}
