package reminder

import (
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

func TestQuietHourBoundariesAndNoReminderDebt(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Study.DistractionWarnMinutes = 8
	cfg.Study.DistractionStrongMinutes = 15
	engine := NewEngine(cfg)
	base := time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local)
	input := state.ReminderDecisionInput{UserMode: state.UserModeStudy, Relation: state.RelationDistracted}

	input.Now, input.DistractedSeconds = base.Add(11*time.Hour+59*time.Minute), 480
	if got := engine.Evaluate(input); got == nil {
		t.Fatal("11:59 should allow a due reminder")
	}
	input.Now, input.DistractedSeconds = base.Add(12*time.Hour), 481
	if got := engine.Evaluate(input); got != nil {
		t.Fatalf("12:00 should be quiet: %+v", got)
	}
	input.Now, input.DistractedSeconds = base.Add(13*time.Hour+59*time.Minute), 7620
	if got := engine.Evaluate(input); got != nil {
		t.Fatalf("13:59 should be quiet: %+v", got)
	}
	input.Now, input.DistractedSeconds = base.Add(14*time.Hour), 7621
	if got := engine.Evaluate(input); got != nil {
		t.Fatalf("14:00 must reset eligibility without catch-up: %+v", got)
	}
	input.Now, input.DistractedSeconds = base.Add(14*time.Hour+7*time.Minute+59*time.Second), 8099
	if got := engine.Evaluate(input); got != nil {
		t.Fatalf("normal threshold has not elapsed: %+v", got)
	}
	input.Now, input.DistractedSeconds = base.Add(14*time.Hour+8*time.Minute), 8101
	if got := engine.Evaluate(input); got == nil || got.Level != state.ReminderLevelBubble {
		t.Fatalf("expected reminder after fresh threshold: %+v", got)
	}
}

func TestAllDefaultQuietBoundaries(t *testing.T) {
	engine := NewEngine(config.DefaultConfig())
	base := time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local)
	checks := []struct {
		at    time.Duration
		quiet bool
	}{
		{11*time.Hour + 59*time.Minute, false}, {12 * time.Hour, true}, {13*time.Hour + 59*time.Minute, true}, {14 * time.Hour, false},
		{17*time.Hour + 29*time.Minute, false}, {17*time.Hour + 30*time.Minute, true}, {18*time.Hour + 59*time.Minute, true}, {19 * time.Hour, false},
		{20*time.Hour + 59*time.Minute, false}, {21 * time.Hour, true}, {23*time.Hour + 59*time.Minute, true},
	}
	for _, check := range checks {
		if got := engine.isQuiet(base.Add(check.at)); got != check.quiet {
			t.Fatalf("at %s quiet=%v want %v", check.at, got, check.quiet)
		}
	}
	if engine.isQuiet(base.Add(24 * time.Hour)) {
		t.Fatal("next-day 00:00 must not remain in prior day's quiet period")
	}
}

func TestQuietSuppressesAllReminderModes(t *testing.T) {
	engine := NewEngine(config.DefaultConfig())
	now := time.Date(2026, 9, 5, 12, 30, 0, 0, time.Local)
	inputs := []state.ReminderDecisionInput{
		{Now: now, UserMode: state.UserModeStandby, ActiveSeconds: 99999},
		{Now: now, UserMode: state.UserModeStudy, Relation: state.RelationDistracted, DistractedSeconds: 99999},
		{Now: now, UserMode: state.UserModeStudy, Interaction: state.InteractionIdleStatic, IdleStaticSeconds: 99999},
		{Now: now, UserMode: state.UserModeBreak, BreakSeconds: 99999},
	}
	for _, input := range inputs {
		if got := engine.Evaluate(input); got != nil {
			t.Fatalf("quiet reminder leaked for %s: %+v", input.UserMode, got)
		}
	}
}
