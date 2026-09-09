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

func TestControllerPreservesBreakFocusAcrossLongSamplingIntervals(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.Enabled = false
	cfg.AutoResume.FocusedStableSeconds = 120
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	outcome := focusedOutcome()
	outcome.UserMode = state.UserModeBreak
	status := state.SystemStatus{UserMode: state.UserModeBreak, ModeOrigin: state.ModeOriginAutomation, AutoResumeEligible: true, PrivacyState: state.PrivacyNormal}
	if got := controller.Evaluate(start, outcome, status); got != nil {
		t.Fatalf("resumed before stability threshold: %+v", got)
	}
	if got := controller.Evaluate(start.Add(60*time.Second), outcome, status); got != nil {
		t.Fatalf("a sampling interval reset the focus timer: %+v", got)
	}
	if got := controller.Evaluate(start.Add(119*time.Second), outcome, status); got != nil {
		t.Fatalf("resumed before 120 seconds: %+v", got)
	}
	if got := controller.Evaluate(start.Add(120*time.Second), outcome, status); got == nil || got.Transition != state.AutomationResume {
		t.Fatalf("did not resume after cumulative focus: %+v", got)
	}
}

func TestControllerBreakResumeFocusInterruptionsResetStability(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*state.TickOutcome, *state.SystemStatus)
	}{
		{name: "afk", mutate: func(outcome *state.TickOutcome, _ *state.SystemStatus) {
			outcome.Interaction = state.InteractionIdleStatic
		}},
		{name: "locked", mutate: func(outcome *state.TickOutcome, _ *state.SystemStatus) { outcome.Locked = true }},
		{name: "activitywatch unavailable", mutate: func(outcome *state.TickOutcome, _ *state.SystemStatus) { outcome.ActivityValid = false }},
		{name: "privacy", mutate: func(_ *state.TickOutcome, status *state.SystemStatus) { status.PrivacyState = state.PrivacySensitive }},
		{name: "not focused", mutate: func(outcome *state.TickOutcome, _ *state.SystemStatus) { outcome.Relation = state.RelationDistracted }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig().Automation
			cfg.Enabled = true
			cfg.AutoStart.Enabled = false
			cfg.AutoResume.FocusedStableSeconds = 10
			cfg.TransitionCooldownSeconds = 1
			controller := New(cfg)
			start := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)
			focused := focusedOutcome()
			focused.UserMode = state.UserModeBreak
			status := state.SystemStatus{UserMode: state.UserModeBreak, ModeOrigin: state.ModeOriginAutomation, AutoResumeEligible: true, PrivacyState: state.PrivacyNormal}
			controller.Evaluate(start, focused, status)
			interrupted := focused
			interruptedStatus := status
			tc.mutate(&interrupted, &interruptedStatus)
			if got := controller.Evaluate(start.Add(5*time.Second), interrupted, interruptedStatus); got != nil {
				t.Fatalf("interrupted focus resumed: %+v", got)
			}
			if got := controller.Evaluate(start.Add(10*time.Second), focused, status); got != nil {
				t.Fatalf("timer was not reset after interruption: %+v", got)
			}
			if got := controller.Evaluate(start.Add(20*time.Second), focused, status); got == nil || got.Transition != state.AutomationResume {
				t.Fatalf("focus did not resume after a fresh stable interval: %+v", got)
			}
		})
	}
}

func TestControllerAllowsAutoPauseAfterManualStudy(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.Enabled = false
	cfg.AutoPause.LockedSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual, PrivacyState: state.PrivacyNormal, Task: "Go"}
	locked := state.TickOutcome{UserMode: state.UserModeStudy, Interaction: state.InteractionUnknown, ActivityValid: true, Locked: true, Classification: state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1}}
	if got := controller.Evaluate(start, locked, status); got != nil {
		t.Fatal("paused before lock threshold")
	}
	got := controller.Evaluate(start.Add(2*time.Second), locked, status)
	if got == nil || got.Transition != state.AutomationPause {
		t.Fatalf("intent=%+v", got)
	}
}

func TestControllerAllowUnclassifiedRequiresExplicitStudyActivityAndLongStability(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 1
	cfg.AutoStart.AllowUnclassified = true
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "算法"}
	unknown := state.TickOutcome{UserMode: state.UserModeStandby, Interaction: state.InteractionActive, Relation: state.RelationUnknown, ActivityValid: true, Classification: state.ClassificationResult{Relation: state.RelationUnknown, Activity: state.ActivityBrowsing, Confidence: .9}}
	controller.Evaluate(start, unknown, status)
	if got := controller.Evaluate(start.Add(180*time.Second), unknown, status); got != nil {
		t.Fatalf("browsing unknown must not start: %+v", got)
	}

	study := unknown
	study.Classification.Activity = state.ActivityCoding
	controller.UpdateConfig(cfg)
	controller.Evaluate(start, study, status)
	if got := controller.Evaluate(start.Add(179*time.Second), study, status); got != nil {
		t.Fatalf("unclassified study started too early: %+v", got)
	}
	if got := controller.Evaluate(start.Add(180*time.Second), study, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("intent=%+v", got)
	}
}

func TestControllerUpdateConfigResetsStabilityTimers(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 3
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	outcome := focusedOutcome()
	controller.Evaluate(start, outcome, status)
	controller.Evaluate(start.Add(2*time.Second), outcome, status)
	controller.UpdateConfig(cfg)
	if got := controller.Evaluate(start.Add(3*time.Second), outcome, status); got != nil {
		t.Fatalf("updated config reused old timer: %+v", got)
	}
	if got := controller.Evaluate(start.Add(6*time.Second), outcome, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("intent=%+v", got)
	}
}

func TestControllerUsesCumulativeAfkForStaticAndDynamicThresholds(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.Enabled = false
	cfg.AutoPause.IdleStaticSeconds = 300
	cfg.AutoPause.IdleDynamicSeconds = 900
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	now := time.Date(2026, 9, 9, 14, 0, 0, 0, time.UTC)
	base := state.SystemStatus{UserMode: state.UserModeStudy, PrivacyState: state.PrivacyNormal, Task: "Go"}
	dynamic := state.TickOutcome{UserMode: state.UserModeStudy, Interaction: state.InteractionIdleDynamic, Relation: state.RelationUnknown, ActivityValid: true, AfkSeconds: 899}
	if got := controller.Evaluate(now, dynamic, base); got != nil {
		t.Fatalf("dynamic AFK paused before 900 seconds: %+v", got)
	}
	dynamic.AfkSeconds = 900
	base.AfkSeconds = dynamic.AfkSeconds
	if got := controller.Evaluate(now.Add(time.Second), dynamic, base); got == nil || got.Transition != state.AutomationPause {
		t.Fatalf("dynamic AFK did not pause at threshold: %+v", got)
	}
	controller.UpdateConfig(cfg)
	static := dynamic
	static.Interaction = state.InteractionIdleStatic
	static.AfkSeconds = 300
	base.AfkSeconds = static.AfkSeconds
	if got := controller.Evaluate(now, static, base); got == nil || got.Transition != state.AutomationPause {
		t.Fatalf("static AFK did not pause at threshold: %+v", got)
	}
}
