package automation

import (
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

func focusedOutcome() state.TickOutcome {
	return state.TickOutcome{UserMode: state.UserModeStandby, Interaction: state.InteractionActive, Relation: state.RelationFocused, ActivityValid: true, Classification: state.ClassificationResult{Relation: state.RelationFocused, Confidence: .9}}
}

func TestControllerRequiresStableFocusForAutoStart(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 3
	cfg.TransitionCooldownSeconds = 1
	c := New(cfg)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if got := c.Evaluate(start, focusedOutcome(), status); got != nil {
		t.Fatal("started before stability threshold")
	}
	if got := c.Evaluate(start.Add(2*time.Second), focusedOutcome(), status); got != nil {
		t.Fatal("started before stability threshold")
	}
	got := c.Evaluate(start.Add(3*time.Second), focusedOutcome(), status)
	if got == nil || got.Transition != state.AutomationStart || got.Task != "Go" {
		t.Fatalf("intent=%+v", got)
	}
}

func TestControllerOnlyResumesAutomationBreak(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoResume.FocusedStableSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	c := New(cfg)
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	outcome := focusedOutcome()
	outcome.UserMode = state.UserModeBreak
	for _, origin := range []state.ModeOrigin{state.ModeOriginManual, state.ModeOriginAutomation} {
		status := state.SystemStatus{UserMode: state.UserModeBreak, ModeOrigin: origin, AutoResumeEligible: origin == state.ModeOriginAutomation, PrivacyState: state.PrivacyNormal}
		if got := c.Evaluate(start, outcome, status); got != nil {
			t.Fatalf("origin=%s intent=%+v", origin, got)
		}
	}
	status := state.SystemStatus{UserMode: state.UserModeBreak, ModeOrigin: state.ModeOriginAutomation, AutoResumeEligible: true, PrivacyState: state.PrivacyNormal}
	if got := c.Evaluate(start.Add(2*time.Second), outcome, status); got == nil || got.Transition != state.AutomationResume {
		t.Fatalf("intent=%+v", got)
	}
}
