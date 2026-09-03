package semantic

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func testCandidate(at time.Time) Candidate {
	return Candidate{ObservedAt: at, Fresh: true, UserMode: state.UserModeStudy, Task: "Go Lab", Interaction: state.InteractionActive, Relation: state.RelationFocused, Privacy: state.PrivacyNormal, App: "Code.exe", Title: "main.go", Domain: ""}
}

func TestServiceThrottleStableTransitionHeartbeatAndTitleKey(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := NewServiceWithTiming(store, Timing{TransitionStableFor: time.Second, MinPersistInterval: 3 * time.Second, HeartbeatInterval: 10 * time.Second, LiveMaxAge: 2 * time.Second})
	ctx := context.Background()
	start := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{start, start.Add(500 * time.Millisecond), start.Add(time.Second)} {
		if err := service.Observe(ctx, testCandidate(at)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID <= 0 || !rows[0].ObservedAt.Equal(start.Add(time.Second)) {
		t.Fatalf("initial stable rows=%+v", rows)
	}

	// A title change is deliberately excluded from the semantic key and must
	// not restart the stable transition or create a 15-second style log.
	changedTitle := testCandidate(start.Add(2 * time.Second))
	changedTitle.Title = "other.go"
	if err := service.Observe(ctx, changedTitle); err != nil {
		t.Fatal(err)
	}
	if err := service.Observe(ctx, testCandidate(start.Add(5*time.Second))); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 1 {
		t.Fatalf("title change unexpectedly created rows=%d", len(rows))
	}

	// A semantic transition is persisted only after stability and the minimum
	// interval; a long unchanged block is then represented by a heartbeat.
	reading := testCandidate(start.Add(2 * time.Second))
	reading.App, reading.Title = "Acrobat.exe", "chapter.pdf"
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	reading.ObservedAt = start.Add(3 * time.Second)
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 1 {
		t.Fatalf("transition before min interval rows=%d", len(rows))
	}
	reading.ObservedAt = start.Add(4 * time.Second)
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 2 || rows[1].Activity != string(ActivityReading) || rows[1].ID <= rows[0].ID {
		t.Fatalf("transition rows=%+v", rows)
	}
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	reading.ObservedAt = start.Add(10 * time.Second)
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 2 {
		t.Fatalf("heartbeat fired too early rows=%d", len(rows))
	}
	reading.ObservedAt = start.Add(13 * time.Second)
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 2 {
		t.Fatalf("heartbeat should still be pending rows=%d", len(rows))
	}
	reading.ObservedAt = start.Add(14 * time.Second)
	if err := service.Observe(ctx, reading); err != nil {
		t.Fatal(err)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, "2026-09-03")
	if len(rows) != 3 {
		t.Fatalf("heartbeat rows=%d, want 3", len(rows))
	}
}

func TestServicePrivacyFreshnessModeAndCrossMidnight(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := NewServiceWithTiming(store, Timing{TransitionStableFor: time.Second, MinPersistInterval: time.Second, HeartbeatInterval: 10 * time.Second, LiveMaxAge: 2 * time.Second})
	ctx := context.Background()
	loc := time.FixedZone("test-local", 8*60*60)
	start := time.Date(2026, 9, 3, 23, 59, 59, 0, loc)
	candidate := testCandidate(start)
	if err := service.Observe(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	candidate.ObservedAt = start.Add(time.Second)
	if err := service.Observe(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	rows, listErr := store.ListSemanticSnapshotsForDate(ctx, candidate.ObservedAt.In(time.Local).Format("2006-01-02"))
	if len(rows) != 1 || rows[0].LocalDate != candidate.ObservedAt.In(time.Local).Format("2006-01-02") {
		t.Fatalf("cross-midnight rows=%+v err=%v date=%s", rows, listErr, candidate.ObservedAt.In(time.Local).Format("2006-01-02"))
	}

	sensitive := testCandidate(start.Add(2 * time.Second))
	sensitive.Privacy = state.PrivacySensitive
	if err := service.Observe(ctx, sensitive); err != nil {
		t.Fatal(err)
	}
	view := service.Current(sensitive.ObservedAt)
	if view.Privacy != state.PrivacySensitive || view.Activity != ActivityUnknown || view.Fresh {
		t.Fatalf("sensitive current view=%+v", view)
	}
	if err := service.Observe(ctx, Candidate{ObservedAt: start.Add(3 * time.Second), Fresh: false, UserMode: state.UserModeStudy, Interaction: state.InteractionUnknown, Relation: state.RelationUnknown, Privacy: state.PrivacyNormal}); err != nil {
		t.Fatal(err)
	}
	view = service.Current(start.Add(3 * time.Second))
	if view.Fresh || view.Activity != ActivityUnknown || view.Confidence != 0 {
		t.Fatalf("stale current view=%+v", view)
	}
	rows, _ = store.ListSemanticSnapshotsForDate(ctx, candidate.ObservedAt.In(time.Local).Format("2006-01-02"))
	if len(rows) != 1 {
		t.Fatalf("privacy/stale observation persisted rows=%d", len(rows))
	}
	if service.Current(start.Add(6 * time.Second)).Fresh {
		t.Fatal("live view remained fresh past LiveMaxAge")
	}
}
