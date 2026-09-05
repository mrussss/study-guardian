package state

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

type captureReminderEvaluator struct {
	last ReminderDecisionInput
}

func (r *captureReminderEvaluator) Evaluate(input ReminderDecisionInput) *ReminderEvent {
	r.last = input
	return nil
}

type mockRuleClassifier struct{}

func (mockRuleClassifier) Classify(app, title, domain, task string) ClassificationResult {
	if app == "steam.exe" {
		return ClassificationResult{
			Relation:   RelationDistracted,
			Confidence: 0.95,
			Reason:     "Steam distraction",
			IsFromRule: true,
		}
	}
	return ClassificationResult{
		Relation:   RelationFocused,
		Confidence: 0.90,
		Reason:     "Dev work",
		IsFromRule: true,
	}
}

type mockPrivacyEvaluator struct{}

func (mockPrivacyEvaluator) Evaluate(app, title, domain string) PrivacyState {
	if app == "bitwarden.exe" {
		return PrivacySensitive
	}
	return PrivacyNormal
}

type mockReminderEvaluator struct{}

func (mockReminderEvaluator) Evaluate(input ReminderDecisionInput) *ReminderEvent {
	if input.DistractedSeconds >= 480 {
		return &ReminderEvent{
			ID:        "rem-distract",
			Level:     ReminderLevelBubble,
			Message:   "Distraction warning",
			Reason:    "DISTRACTION_WARN",
			CreatedAt: input.Now,
		}
	}
	return nil
}

func TestManagerTickAndMidnightReset(t *testing.T) {
	now := time.Date(2026, 9, 2, 23, 50, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()

	mgr := NewPersistentManager(
		clock,
		cfg,
		store,
		mockRuleClassifier{},
		mockPrivacyEvaluator{},
		mockReminderEvaluator{},
	)

	// Set to STUDY
	err := mgr.SetModeStudy("Go Concurrency Lab")
	if err != nil {
		t.Fatalf("failed to set STUDY: %v", err)
	}

	// Tick 5 seconds of active study
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	st := mgr.GetStatus()
	if st.InteractionState != InteractionActive || st.TaskRelation != RelationFocused {
		t.Fatalf("expected ACTIVE & FOCUSED, got %s & %s", st.InteractionState, st.TaskRelation)
	}

	// Advance time across midnight (to 00:05:00 next day)
	nextDay := time.Date(2026, 9, 3, 0, 5, 0, 0, time.Local)
	clock.Set(nextDay)
	mgr.Tick(nextDay, "code.exe", "main.go", "", false, true, false)

	st = mgr.GetStatus()
	// Should reset to STANDBY
	if st.UserMode != UserModeStandby {
		t.Fatalf("expected reset to STANDBY on midnight cross, got %s", st.UserMode)
	}
	// Task name should be preserved for UI suggestion
	if st.Task != "Go Concurrency Lab" {
		t.Fatalf("expected preserved task name, got %s", st.Task)
	}
	// Study seconds for the new day should be reset
	if st.StudySeconds != 0 {
		t.Fatalf("expected 0 study seconds for new day, got %d", st.StudySeconds)
	}
}

func TestManagerDistractionReminderTrigger(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()

	mgr := NewPersistentManager(
		clock,
		cfg,
		store,
		mockRuleClassifier{},
		mockPrivacyEvaluator{},
		mockReminderEvaluator{},
	)

	_ = mgr.SetModeStudy("Writing Report")

	// Tick 500 seconds with Steam
	for i := 1; i <= 100; i++ {
		now = now.Add(5 * time.Second)
		clock.Set(now)
		mgr.Tick(now, "steam.exe", "Steam Store", "", false, true, false)
	}

	st := mgr.GetStatus()
	if st.TaskRelation != RelationDistracted {
		t.Fatalf("expected DISTRACTED relation, got %s", st.TaskRelation)
	}
	if st.CurrentReminder == nil || st.CurrentReminder.Level != ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder after 500s distraction, got %+v", st.CurrentReminder)
	}
}

func TestManagerRestartRecoversOnlyInterruptedSession(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	dbPath := t.TempDir() + "/studyguardian.db"
	cfg := config.DefaultConfig()
	clock := NewFakeClock(now)
	store1, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mgr1 := NewPersistentManager(clock, cfg, store1, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if err := mgr1.SetModeOff(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(12 * time.Second)
	clock.Set(now)
	mgr1.Tick(now, "", "", "", true, false, false)
	// Simulate a process kill: close the database without a clean manager close.
	_ = store1.Close()

	store2, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	mgr2 := NewPersistentManager(clock, cfg, store2, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if got := mgr2.GetStatus().UserMode; got != UserModeOff {
		t.Fatalf("expected same-day OFF recovery, got %s", got)
	}
	openCount, err := store2.CountOpenSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if openCount != 1 {
		t.Fatalf("expected exactly one open session after recovery, got %d", openCount)
	}

	mgr2.Close()
	_ = store2.Close()
	store3, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store3.Close()
	mgr3 := NewPersistentManager(clock, cfg, store3, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if got := mgr3.GetStatus().UserMode; got != UserModeStandby {
		t.Fatalf("expected cleanly completed OFF not to be restored, got %s", got)
	}
}

func TestManagerLockAndLongGapDoNotAddUserTime(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	mgr := NewPersistentManager(clock, config.DefaultConfig(), nil, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	_ = mgr.SetModeStudy("Go")

	now = now.Add(10 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	beforeLock := mgr.GetStatus().StudySeconds

	now = now.Add(20 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, true)
	locked := mgr.GetStatus()
	if locked.StudySeconds != beforeLock || locked.InteractionState != InteractionUnknown || locked.TaskRelation != RelationUnknown {
		t.Fatalf("lock screen must pause time and clear observation, before=%d after=%+v", beforeLock, locked)
	}

	now = now.Add(2 * time.Hour)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	afterResume := mgr.GetStatus()
	if afterResume.StudySeconds != beforeLock {
		t.Fatalf("long resume gap must not add user time, got %d want %d", afterResume.StudySeconds, beforeLock)
	}

	now = now.Add(4 * time.Second)
	clock.Set(now)
	resumed := mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	if resumed.DeltaSeconds != 4 || mgr.GetStatus().StudySeconds != beforeLock+4 {
		t.Fatalf("normal ticks after resume should recover timing, outcome=%+v status=%+v", resumed, mgr.GetStatus())
	}
}

func TestBreakReminderUsesCurrentBreakSessionDuration(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	reminders := &captureReminderEvaluator{}
	mgr := NewPersistentManager(clock, config.DefaultConfig(), nil, mockRuleClassifier{}, mockPrivacyEvaluator{}, reminders)
	_ = mgr.SetModeBreak()
	now = now.Add(15 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, false)
	_ = mgr.SetModeStudy("Go")
	now = now.Add(1 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	_ = mgr.SetModeBreak()
	now = now.Add(5 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, false)
	if reminders.last.BreakSeconds != 5 {
		t.Fatalf("expected current BREAK duration 5, got %d", reminders.last.BreakSeconds)
	}
}

func TestSetTaskPersistsIntoOpenSessionAndRestartRecovery(t *testing.T) {
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.Local)
	dbPath := t.TempDir() + "/studyguardian.db"
	cfg := config.DefaultConfig()
	clock := NewFakeClock(now)
	store1, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mgr1 := NewPersistentManager(clock, cfg, store1, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	if err := mgr1.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := mgr1.SetTask("  算法   练习  "); err != nil {
		t.Fatal(err)
	}
	open, err := store1.LoadOpenSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if open.Task != "算法 练习" {
		t.Fatalf("open session task=%q", open.Task)
	}
	_ = store1.Close() // process-kill simulation: preserve the open row

	store2, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	mgr2 := NewPersistentManager(clock, cfg, store2, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	if got := mgr2.GetStatus().Task; got != "算法 练习" {
		t.Fatalf("recovered task=%q, want 算法 练习", got)
	}
}
