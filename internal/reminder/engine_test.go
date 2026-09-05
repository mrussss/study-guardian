package reminder

import (
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

func TestReminderEngineDecisions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Reminder.QuietPeriods = nil
	engine := NewEngine(cfg)
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// 1. STANDBY below threshold -> No reminder
	input := state.ReminderDecisionInput{
		Now:           now,
		UserMode:      state.UserModeStandby,
		ActiveSeconds: 1800, // 30 min
		StudySeconds:  0,
	}
	ev := engine.Evaluate(input)
	if ev != nil {
		t.Fatalf("expected nil reminder for 30 min standby, got %+v", ev)
	}

	// 2. STANDBY above threshold (60 min) -> Bubble reminder
	input.ActiveSeconds = 3600
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder for 60 min standby, got %+v", ev)
	}

	// Cooldown within 30 min -> No duplicate reminder
	input.Now = now.Add(10 * time.Minute)
	ev = engine.Evaluate(input)
	if ev != nil {
		t.Fatalf("expected cooldown suppression, got %+v", ev)
	}

	// 3. STUDY: Distraction warn (8 min)
	studyNow := now.Add(1 * time.Hour)
	input = state.ReminderDecisionInput{
		Now:               studyNow,
		UserMode:          state.UserModeStudy,
		Task:              "Go Concurrency",
		Relation:          state.RelationDistracted,
		DistractedSeconds: 480, // 8 min
	}
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder for 8 min distraction, got %+v", ev)
	}

	// 4. STUDY: Distraction strong (15 min)
	input.Now = studyNow.Add(7 * time.Minute)
	input.DistractedSeconds = 900 // 15 min
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelToast {
		t.Fatalf("expected Toast reminder for 15 min distraction, got %+v", ev)
	}

	// 5. BREAK: Warn at 20 min, Strong at 30 min
	breakNow := now.Add(2 * time.Hour)
	input = state.ReminderDecisionInput{
		Now:          breakNow,
		UserMode:     state.UserModeBreak,
		BreakSeconds: 1200, // 20 min
	}
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder for 20 min break, got %+v", ev)
	}

	input.Now = breakNow.Add(10 * time.Minute)
	input.BreakSeconds = 1800 // 30 min
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelToast {
		t.Fatalf("expected Toast reminder for 30 min break, got %+v", ev)
	}

	// 6. OFF: No reminders ever
	input.UserMode = state.UserModeOff
	input.BreakSeconds = 9999
	ev = engine.Evaluate(input)
	if ev != nil {
		t.Fatalf("expected nil reminder when OFF, got %+v", ev)
	}
}
