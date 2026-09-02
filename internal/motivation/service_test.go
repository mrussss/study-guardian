package motivation

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func testService(t *testing.T) (*Service, *storage.Storage) {
	t.Helper()
	cfg := config.DefaultConfig()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewService(cfg, store), store
}

func TestRecordTickCreditPolicy(t *testing.T) {
	svc, store := testService(t)
	base := state.TickOutcome{DeltaSeconds: 60, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationUnknown}
	svc.RecordTick(base)
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, IdleStaticSeconds: 301, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionIdleStatic, Relation: state.RelationFocused})
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationDistracted})
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, UserMode: state.UserModeStudy, ActivityValid: false, Interaction: state.InteractionActive, Relation: state.RelationFocused})
	d, err := store.GetMotivationDaily(context.Background(), time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if d.CreditedFocusSeconds != 60 {
		t.Fatalf("credited=%d, want 60", d.CreditedFocusSeconds)
	}
}

func TestStaticReadingCreditGrace(t *testing.T) {
	svc, store := testService(t)
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, IdleStaticSeconds: 60, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionIdleStatic, Relation: state.RelationFocused})
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, IdleStaticSeconds: 301, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionIdleStatic, Relation: state.RelationFocused})
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, IdleStaticSeconds: 1, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionIdleStatic, Relation: state.RelationUnknown})
	d, err := store.GetMotivationDaily(context.Background(), time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if d.CreditedFocusSeconds != 60 {
		t.Fatalf("static credit=%d, want 60", d.CreditedFocusSeconds)
	}
}

func TestMissionAndRewardAreIdempotent(t *testing.T) {
	svc, _ := testService(t)
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 3600, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationFocused})
	m, err := svc.CreateMission(context.Background(), "Read Go", "", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, done, err := svc.CompleteMission(context.Background(), m.ID); err != nil || !done {
		t.Fatalf("first complete: done=%v err=%v", done, err)
	}
	if _, done, err := svc.CompleteMission(context.Background(), m.ID); err != nil || done {
		t.Fatalf("second complete: done=%v err=%v", done, err)
	}
	if _, err := svc.RedeemReward(context.Background(), "game-60"); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if _, err := svc.RedeemReward(context.Background(), "game-60"); err == nil {
		t.Fatal("second redeem should fail when balance is exhausted")
	}
}

func TestConfiguredAPRateIsUsed(t *testing.T) {
	_, store := testService(t)
	cfg := config.DefaultConfig()
	cfg.Motivation.APPerFocusHourMilli = 2000
	svc := NewService(cfg, store)
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 3600, UserMode: state.UserModeStudy, Interaction: state.InteractionActive, Relation: state.RelationUnknown, ActivityValid: true})
	status, err := svc.GetStatus(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if status.BalanceAPMilli != 2000 || status.TodayEarnedAPMilli != 2000 {
		t.Fatalf("configured AP rate not applied: %#v", status)
	}
}

func TestComebackUnlocksAfterDistractionAndFocus(t *testing.T) {
	svc, store := testService(t)
	now := time.Now().Add(-time.Minute)
	if err := store.RecordObservation(context.Background(), storage.ObservationRecord{Timestamp: now, Interaction: "ACTIVE", Relation: "DISTRACTED", Privacy: "SAFE", CurrentMode: "STUDY"}); err != nil {
		t.Fatal(err)
	}
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 1800, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationFocused})
	items, err := svc.Achievements(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == "COMEBACK" {
			if !item.Unlocked {
				t.Fatal("comeback should unlock after prior distraction and 30 minutes of focus")
			}
			return
		}
	}
	t.Fatal("COMEBACK definition not found")
}

func TestDailyTargetSettingIsPersistentAndUpdatesToday(t *testing.T) {
	svc, store := testService(t)
	now := time.Now()
	settings, err := svc.SetDailyTarget(context.Background(), 90, now)
	if err != nil || settings.DailyTargetMinutes != 90 {
		t.Fatalf("set target: %#v %v", settings, err)
	}
	svc.RecordTick(state.TickOutcome{DeltaSeconds: 60, UserMode: state.UserModeStudy, ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationFocused})
	d, err := store.GetMotivationDaily(context.Background(), now.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if d.DailyTargetSeconds != 5400 {
		t.Fatalf("today target=%d, want 5400", d.DailyTargetSeconds)
	}
}

func TestEventsArePersistedAndCursorable(t *testing.T) {
	svc, store := testService(t)
	svc.emit("CHECKIN_COMPLETED", "done", time.Now())
	events, err := svc.Events(context.Background(), 0, 20)
	if err != nil || len(events) != 1 || events[0].EventType != "CHECKIN_COMPLETED" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if next, err := store.ListUIEvents(context.Background(), events[0].ID, 20); err != nil || len(next) != 0 {
		t.Fatalf("cursor did not advance: %#v %v", next, err)
	}
}
