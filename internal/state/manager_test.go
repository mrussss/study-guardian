package state

import (
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

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
	mgr.Tick(now, "code.exe", "main.go", "", false, true)
	st := mgr.GetStatus()
	if st.InteractionState != InteractionActive || st.TaskRelation != RelationFocused {
		t.Fatalf("expected ACTIVE & FOCUSED, got %s & %s", st.InteractionState, st.TaskRelation)
	}

	// Advance time across midnight (to 00:05:00 next day)
	nextDay := time.Date(2026, 9, 3, 0, 5, 0, 0, time.Local)
	clock.Set(nextDay)
	mgr.Tick(nextDay, "code.exe", "main.go", "", false, true)

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
		mgr.Tick(now, "steam.exe", "Steam Store", "", false, true)
	}

	st := mgr.GetStatus()
	if st.TaskRelation != RelationDistracted {
		t.Fatalf("expected DISTRACTED relation, got %s", st.TaskRelation)
	}
	if st.CurrentReminder == nil || st.CurrentReminder.Level != ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder after 500s distraction, got %+v", st.CurrentReminder)
	}
}
