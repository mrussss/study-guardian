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

	// Entering STUDY establishes a fresh reminder baseline.
	studyNow := now.Add(1 * time.Hour)
	input = state.ReminderDecisionInput{
		Now:               studyNow,
		UserMode:          state.UserModeStudy,
		Task:              "Go Concurrency",
		Relation:          state.RelationDistracted,
		DistractedSeconds: 0,
	}
	ev = engine.Evaluate(input)
	if ev != nil {
		t.Fatalf("expected no reminder on study baseline, got %+v", ev)
	}

	// 3. STUDY: Distraction warn (8 min)
	input.Now = studyNow.Add(8 * time.Minute)
	input.DistractedSeconds = 480
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder for 8 min distraction, got %+v", ev)
	}

	// 4. STUDY: Distraction strong (15 min)
	input.Now = studyNow.Add(15 * time.Minute)
	input.DistractedSeconds = 900 // 15 min
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelToast {
		t.Fatalf("expected Toast reminder for 15 min distraction, got %+v", ev)
	}

	// 5. BREAK: entering BREAK establishes a fresh session baseline.
	breakNow := now.Add(2 * time.Hour)
	input = state.ReminderDecisionInput{
		Now:          breakNow,
		UserMode:     state.UserModeBreak,
		BreakSeconds: 0,
	}
	ev = engine.Evaluate(input)
	if ev != nil {
		t.Fatalf("expected no reminder on break baseline, got %+v", ev)
	}

	// 5a. BREAK: Warn at 20 min, Strong at 30 min
	input.Now = breakNow.Add(20 * time.Minute)
	input.BreakSeconds = 1200
	ev = engine.Evaluate(input)
	if ev == nil || ev.Level != state.ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder for 20 min break, got %+v", ev)
	}

	input.Now = breakNow.Add(30 * time.Minute)
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

func TestReminderEngineResetsBaselineWhenEnteringStudy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Reminder.QuietPeriods = nil
	engine := NewEngine(cfg)
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	if got := engine.Evaluate(state.ReminderDecisionInput{Now: now, UserMode: state.UserModeStandby, ActiveSeconds: 3600}); got == nil {
		t.Fatal("expected standby reminder before entering study")
	}
	study := state.ReminderDecisionInput{Now: now.Add(time.Minute), UserMode: state.UserModeStudy, Relation: state.RelationDistracted, DistractedSeconds: 9999, IdleStaticSeconds: 9999}
	if got := engine.Evaluate(study); got != nil {
		t.Fatalf("mode transition inherited standby/distraction time: %+v", got)
	}
	if got := engine.Evaluate(state.ReminderDecisionInput{Now: study.Now.Add(8 * time.Minute), UserMode: state.UserModeStudy, Relation: state.RelationDistracted, DistractedSeconds: 480}); got == nil {
		t.Fatal("expected distraction reminder after fresh study threshold")
	}
}

func TestReminderEngineStaticIdleThresholds(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Reminder.QuietPeriods = nil
	engine := NewEngine(cfg)
	base := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	input := state.ReminderDecisionInput{Now: base, UserMode: state.UserModeStudy, Interaction: state.InteractionIdleStatic}
	if got := engine.Evaluate(input); got != nil {
		t.Fatalf("expected no reminder on study baseline, got %+v", got)
	}
	input.Now = base.Add(20 * time.Minute)
	input.IdleStaticSeconds = 1200
	if got := engine.Evaluate(input); got == nil || got.Reason != "IDLE_STATIC_WARN" || got.Level != state.ReminderLevelBubble {
		t.Fatalf("expected idle-static warning at 20 minutes, got %+v", got)
	}

	strong := NewEngine(cfg)
	if got := strong.Evaluate(state.ReminderDecisionInput{Now: base, UserMode: state.UserModeStudy, Interaction: state.InteractionIdleStatic}); got != nil {
		t.Fatalf("expected no reminder on strong baseline, got %+v", got)
	}
	if got := strong.Evaluate(state.ReminderDecisionInput{Now: base.Add(30 * time.Minute), UserMode: state.UserModeStudy, Interaction: state.InteractionIdleStatic, IdleStaticSeconds: 1800}); got == nil || got.Reason != "IDLE_STATIC_STRONG" || got.Level != state.ReminderLevelToast {
		t.Fatalf("expected idle-static strong reminder at 30 minutes, got %+v", got)
	}
}
