package distraction

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func testInput(now time.Time) Input {
	return Input{Now: now, UserMode: state.UserModeStudy, ActivityWatchOK: true, ActivityFresh: true, Privacy: state.PrivacyNormal, Relation: state.RelationDistracted, Confidence: .95, Source: state.SourceKindLocalRule, App: "steam.exe", Title: "Game", Domain: "store.steampowered.com", Task: "Go"}
}

func TestTrackerPersistsOneContinuousEventAndClosesAfterRecovery(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := NewWithTiming(store, Timing{DistractedStableFor: 2 * time.Second, RecoveryStableFor: 3 * time.Second, UnknownGrace: time.Second})
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for _, offset := range []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second} {
		input := testInput(start.Add(offset))
		input.Title = "Game window " + input.Title
		if err := tracker.Observe(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if err := tracker.Observe(ctx, Input{Now: start.Add(5 * time.Second), UserMode: state.UserModeStudy, ActivityWatchOK: true, ActivityFresh: true, Privacy: state.PrivacyNormal, Relation: state.RelationFocused, Confidence: .9}); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, Input{Now: start.Add(8 * time.Second), UserMode: state.UserModeStudy, ActivityWatchOK: true, ActivityFresh: true, Privacy: state.PrivacyNormal, Relation: state.RelationFocused, Confidence: .9}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDistractionsForDate(ctx, "2026-09-08")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].DurationSeconds != 8 || items[0].EndReason != "FOCUS_RECOVERED" {
		t.Fatalf("event=%+v", items[0])
	}
}

func TestTrackerDoesNotCreateEventForShortSwitchNoise(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := NewWithTiming(store, Timing{DistractedStableFor: 5 * time.Second, RecoveryStableFor: 2 * time.Second, UnknownGrace: time.Second})
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if err := tracker.Observe(ctx, testInput(start)); err != nil {
		t.Fatal(err)
	}
	focused := testInput(start.Add(time.Second))
	focused.Relation = state.RelationFocused
	if err := tracker.Observe(ctx, focused); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, testInput(start.Add(2*time.Second))); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDistractionsForDate(ctx, "2026-09-08")
	if err != nil || len(items) != 0 {
		t.Fatalf("short switch created items=%+v err=%v", items, err)
	}
}

func TestTrackerClosesOnLockAndActivityWatchFailure(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := NewWithTiming(store, Timing{DistractedStableFor: time.Second, RecoveryStableFor: time.Second, UnknownGrace: time.Second})
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if err := tracker.Observe(ctx, testInput(start)); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, testInput(start.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	locked := testInput(start.Add(2 * time.Second))
	locked.Locked = true
	if err := tracker.Observe(ctx, locked); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDistractionsForDate(ctx, "2026-09-08")
	if err != nil || len(items) != 1 || items[0].EndReason != "LOCKED" {
		t.Fatalf("locked items=%+v err=%v", items, err)
	}
}

func TestTrackerHoldsOneEventAcrossTransientActivityWatchGap(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := NewWithTiming(store, Timing{DistractedStableFor: time.Second, RecoveryStableFor: 2 * time.Second, UnknownGrace: time.Second})
	start := time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for _, offset := range []time.Duration{0, time.Second} {
		if err := tracker.Observe(ctx, testInput(start.Add(offset))); err != nil {
			t.Fatal(err)
		}
	}
	gap := testInput(start.Add(2 * time.Second))
	gap.ActivityFresh = false
	if err := tracker.Observe(ctx, gap); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, testInput(start.Add(4*time.Second))); err != nil {
		t.Fatal(err)
	}
	focused := testInput(start.Add(5 * time.Second))
	focused.Relation = state.RelationFocused
	if err := tracker.Observe(ctx, focused); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, func() Input {
		next := focused
		next.Now = start.Add(7 * time.Second)
		return next
	}()); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDistractionsForDate(ctx, "2026-09-08")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].DurationSeconds != 5 || items[0].EndReason != "FOCUS_RECOVERED" {
		t.Fatalf("transient gap fragmented or inflated event=%+v", items[0])
	}
}

func TestTrackerStableActivityWatchUnavailableClosesOnlyOnce(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := NewWithTiming(store, Timing{DistractedStableFor: time.Second, RecoveryStableFor: time.Second, UnknownGrace: time.Second})
	start := time.Date(2026, 9, 8, 11, 30, 0, 0, time.UTC)
	ctx := context.Background()
	if err := tracker.Observe(ctx, testInput(start)); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, testInput(start.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	unavailable := testInput(start.Add(2 * time.Second))
	unavailable.ActivityWatchOK = false
	unavailable.ActivityFresh = false
	if err := tracker.Observe(ctx, unavailable); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Observe(ctx, unavailable); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDistractionsForDate(ctx, "2026-09-08")
	if err != nil || len(items) != 1 || items[0].EndReason != "ACTIVITYWATCH_UNAVAILABLE" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}
