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
	for _, origin := range []state.ModeOrigin{state.ModeOriginManual, state.ModeOriginEyeCare, state.ModeOriginAutomation} {
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
	for second := 0; second <= 180; second += 10 {
		if got := controller.Evaluate(start.Add(time.Duration(second)*time.Second), study, status); second < 180 && got != nil {
			t.Fatalf("unclassified study started too early at %d seconds: %+v", second, got)
		} else if second == 180 && (got == nil || got.Transition != state.AutomationStart) {
			t.Fatalf("intent=%+v", got)
		}
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

func candidateOutcome(activity string, confidence float64) state.TickOutcome {
	return state.TickOutcome{
		UserMode:      state.UserModeStandby,
		Interaction:   state.InteractionActive,
		Relation:      state.RelationUnknown,
		ActivityValid: true,
		Classification: state.ClassificationResult{
			Relation:   state.RelationUnknown,
			Activity:   activity,
			Confidence: confidence,
		},
	}
}

func TestControllerUsesLowConfidenceAndUnclassifiedCandidateEvidence(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 2
	cfg.AutoStart.UnclassifiedStableSeconds = 3
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "算法"}

	lowConfidence := focusedOutcome()
	lowConfidence.Classification.Confidence = .72
	controller.Evaluate(start, lowConfidence, status)
	if got := controller.Evaluate(start.Add(2*time.Second), lowConfidence, status); got != nil {
		t.Fatalf("candidate evidence should use the longer threshold: %+v", got)
	}
	if got := controller.Evaluate(start.Add(3*time.Second), lowConfidence, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("low-confidence focused evidence did not start: %+v", got)
	}

	controller.UpdateConfig(cfg)
	unknown := candidateOutcome(state.ActivityReading, .9)
	controller.Evaluate(start, unknown, status)
	if got := controller.Evaluate(start.Add(2*time.Second), unknown, status); got != nil {
		t.Fatalf("unknown study evidence started too early: %+v", got)
	}
	if got := controller.Evaluate(start.Add(3*time.Second), unknown, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("unknown study evidence did not start: %+v", got)
	}
}

func TestControllerPreservesCandidateEvidenceAcrossNeutralGrace(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.UnclassifiedStableSeconds = 4
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	neutral := candidate
	neutral.Relation = state.RelationUnknown
	neutral.Classification.Activity = state.ActivityOther
	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), candidate, status)
	controller.Evaluate(start.Add(3*time.Second), neutral, status)
	controller.Evaluate(start.Add(4*time.Second), candidate, status)
	if got := controller.Evaluate(start.Add(5*time.Second), candidate, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("short neutral gap did not preserve candidate evidence: %+v", got)
	}

	controller.UpdateConfig(cfg)
	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), candidate, status)
	controller.Evaluate(start.Add(5*time.Second), neutral, status)
	controller.Evaluate(start.Add(6*time.Second), neutral, status)
	controller.Evaluate(start.Add(8*time.Second), neutral, status)
	if got := controller.Evaluate(start.Add(9*time.Second), neutral, status); got != nil {
		t.Fatalf("evidence beyond grace should have reset: %+v", got)
	}
	if got := controller.Evaluate(start.Add(10*time.Second), candidate, status); got != nil {
		t.Fatalf("candidate immediately after excessive neutral gap started: %+v", got)
	}
	if got := controller.Evaluate(start.Add(15*time.Second), candidate, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("candidate evidence did not restart after a long neutral gap: %+v", got)
	}
}

func TestControllerReportsAutomaticStartDiagnosticsAndHardBlockers(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 3
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	outcome := focusedOutcome()
	controller.Evaluate(start, outcome, status)
	diagnostic := controller.Diagnostics()
	if diagnostic.State != state.AutomationDiagnosticAccumulating ||
		diagnostic.SignalKind != state.AutomationSignalStrongFocus ||
		diagnostic.RequiredSeconds != 3 {
		t.Fatalf("unexpected accumulation diagnostic: %+v", diagnostic)
	}
	if audit := controller.TakeDiagnosticAudit(); audit == nil || audit.EventType != "AUTO_START_ACCUMULATION_STARTED" {
		t.Fatalf("missing accumulation audit: %+v", audit)
	}
	blocked := outcome
	blocked.ActivityValid = false
	controller.Evaluate(start.Add(time.Second), blocked, status)
	diagnostic = controller.Diagnostics()
	if diagnostic.State != state.AutomationDiagnosticBlocked ||
		diagnostic.Blocker != state.AutomationBlockerActivityUnavailable {
		t.Fatalf("unexpected blocked diagnostic: %+v", diagnostic)
	}
	if audit := controller.TakeDiagnosticAudit(); audit == nil || audit.EventType != "AUTO_START_BLOCKED" {
		t.Fatalf("missing blocker audit: %+v", audit)
	}
}

func TestControllerUsesMixedStrongAndCandidateEvidenceWithinBoundedWindow(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 99
	cfg.AutoStart.UnclassifiedStableSeconds = 6
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	strong := focusedOutcome()
	candidate := candidateOutcome(state.ActivityCoding, .72)
	controller.Evaluate(start, strong, status)
	controller.Evaluate(start.Add(2*time.Second), strong, status)
	if got := controller.Evaluate(start.Add(4*time.Second), candidate, status); got != nil {
		t.Fatalf("mixed evidence started too early: %+v", got)
	}
	got := controller.Evaluate(start.Add(6*time.Second), candidate, status)
	if got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("mixed strong/candidate evidence did not start: %+v", got)
	}
}

func TestControllerAttributesEvidenceToPreviousSampleAcrossSignalBoundaries(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 99
	cfg.AutoStart.UnclassifiedStableSeconds = 99
	cfg.AutoStart.EvidenceGraceSeconds = 2
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 5, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	strong := focusedOutcome()
	neutral := candidateOutcome(state.ActivityOther, .72)

	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), strong, status)
	controller.Evaluate(start.Add(4*time.Second), neutral, status)
	controller.mu.Lock()
	strongSeconds, strongNeutral, candidateSeconds, candidateNeutral := controller.evidenceWindowsLocked(start.Add(4 * time.Second))
	controller.mu.Unlock()
	if strongSeconds != 2 || strongNeutral != 0 || candidateSeconds != 4 || candidateNeutral != 0 {
		t.Fatalf("signal-boundary evidence misattributed: strong=%d strongNeutral=%d candidate=%d candidateNeutral=%d", strongSeconds, strongNeutral, candidateSeconds, candidateNeutral)
	}

	controller.UpdateConfig(cfg)
	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), neutral, status)
	controller.Evaluate(start.Add(4*time.Second), strong, status)
	controller.mu.Lock()
	_, _, candidateSeconds, candidateNeutral = controller.evidenceWindowsLocked(start.Add(4 * time.Second))
	controller.mu.Unlock()
	if candidateSeconds != 2 || candidateNeutral != 2 {
		t.Fatalf("neutral-to-strong interval was credited to the new signal: candidate=%d neutral=%d", candidateSeconds, candidateNeutral)
	}
}

func TestControllerAllowsCandidateHistoryToCompleteOnStrongSample(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 99
	cfg.AutoStart.UnclassifiedStableSeconds = 4
	cfg.AutoStart.EvidenceGraceSeconds = 0
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 6, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	strong := focusedOutcome()
	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), candidate, status)
	if got := controller.Evaluate(start.Add(4*time.Second), strong, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("candidate history plus current strong sample did not complete: %+v", got)
	}
}

func TestControllerDropsEvidenceAcrossSamplingGap(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.UnclassifiedStableSeconds = 2
	cfg.AutoStart.EvidenceGraceSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 7, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	controller.Evaluate(start, candidate, status)
	if got := controller.Evaluate(start.Add(maxEvidenceGap+time.Second), candidate, status); got != nil {
		t.Fatalf("sampling gap incorrectly bridged evidence: %+v", got)
	}
	controller.mu.Lock()
	_, _, candidateSeconds, _ := controller.evidenceWindowsLocked(start.Add(maxEvidenceGap + time.Second))
	controller.mu.Unlock()
	if candidateSeconds != 0 {
		t.Fatalf("sampling gap retained %d seconds of old evidence", candidateSeconds)
	}
}

func TestControllerExpiresEvidenceOutsideTheStrongWindow(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 6
	cfg.AutoStart.UnclassifiedStableSeconds = 100
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 10, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	strong := focusedOutcome()
	candidate := candidateOutcome(state.ActivityCoding, .72)
	controller.Evaluate(start, strong, status)
	controller.Evaluate(start.Add(2*time.Second), strong, status)
	controller.Evaluate(start.Add(4*time.Second), candidate, status)
	if got := controller.Evaluate(start.Add(13*time.Second), candidate, status); got != nil {
		t.Fatalf("expired strong evidence started: %+v", got)
	}
	controller.mu.Lock()
	strongSeconds, _, candidateSeconds, _ := controller.evidenceWindowsLocked(start.Add(13 * time.Second))
	controller.mu.Unlock()
	diagnostic := controller.Diagnostics()
	if strongSeconds != 0 || candidateSeconds != 13 || diagnostic.SignalKind != state.AutomationSignalCandidate || diagnostic.AccumulatedSeconds != 13 {
		t.Fatalf("bounded windows were calculated incorrectly: strong=%d candidate=%d diagnostic=%+v", strongSeconds, candidateSeconds, diagnostic)
	}
}

func TestControllerCountsNeutralTimeAcrossShortGaps(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 99
	cfg.AutoStart.UnclassifiedStableSeconds = 10
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 20, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	neutral := candidateOutcome(state.ActivityOther, .72)
	for second := 0; second <= 40; second++ {
		outcome := candidate
		if second%2 == 1 {
			outcome = neutral
		}
		if got := controller.Evaluate(start.Add(time.Duration(second)*time.Second), outcome, status); got != nil {
			t.Fatalf("alternating short gaps incorrectly started at %d seconds: %+v", second, got)
		}
	}
}

func TestControllerGraceRetainsCandidateProgressAndLongGapBlocks(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 99
	cfg.AutoStart.UnclassifiedStableSeconds = 4
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 30, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	candidate := candidateOutcome(state.ActivityCoding, .72)
	neutral := candidateOutcome(state.ActivityOther, .72)
	controller.Evaluate(start, candidate, status)
	controller.Evaluate(start.Add(2*time.Second), candidate, status)
	controller.Evaluate(start.Add(3*time.Second), neutral, status)
	diagnostic := controller.Diagnostics()
	if diagnostic.State != state.AutomationDiagnosticGrace || diagnostic.SignalKind != state.AutomationSignalCandidate || diagnostic.AccumulatedSeconds != 3 || diagnostic.RequiredSeconds != 4 || diagnostic.GraceRemainingSeconds != 2 {
		t.Fatalf("grace did not retain candidate progress: %+v", diagnostic)
	}
	controller.Evaluate(start.Add(5*time.Second), neutral, status)
	controller.Evaluate(start.Add(6*time.Second), neutral, status)
	diagnostic = controller.Diagnostics()
	if diagnostic.State != state.AutomationDiagnosticBlocked || diagnostic.Blocker != state.AutomationBlockerInsufficientEvidence {
		t.Fatalf("long neutral gap was not blocked: %+v", diagnostic)
	}
	if got := controller.Evaluate(start.Add(7*time.Second), candidate, status); got != nil {
		t.Fatalf("evidence with excessive neutral time started: %+v", got)
	}
	if got := controller.Evaluate(start.Add(13*time.Second), candidate, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("evidence did not recover after the old neutral sample left the window: %+v", got)
	}
}

func TestControllerManualOverrideClearsEvidenceBeforeAccumulation(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 4
	cfg.AutoStart.UnclassifiedStableSeconds = 6
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 40, 0, 0, time.UTC)
	deadline := start.Add(30 * time.Second)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go", ManualOverrideUntil: &deadline}
	outcome := focusedOutcome()
	for second := 0; second < 30; second += 2 {
		if got := controller.Evaluate(start.Add(time.Duration(second)*time.Second), outcome, status); got != nil {
			t.Fatalf("manual override generated intent at %d seconds: %+v", second, got)
		}
		diagnostic := controller.Diagnostics()
		if diagnostic.AccumulatedSeconds != 0 || diagnostic.Blocker != state.AutomationBlockerManualOverride {
			t.Fatalf("manual override accumulated evidence: %+v", diagnostic)
		}
	}
	if got := controller.Evaluate(deadline, outcome, status); got != nil {
		t.Fatalf("override expiry tick reused old evidence: %+v", got)
	}
	if diagnostic := controller.Diagnostics(); diagnostic.AccumulatedSeconds != 0 {
		t.Fatalf("first post-override tick was not a fresh window: %+v", diagnostic)
	}
	if got := controller.Evaluate(deadline.Add(2*time.Second), outcome, status); got != nil {
		t.Fatalf("post-override evidence started too early: %+v", got)
	}
	if got := controller.Evaluate(deadline.Add(4*time.Second), outcome, status); got == nil || got.Transition != state.AutomationStart {
		t.Fatalf("post-override evidence did not accumulate from zero: %+v", got)
	}
}

func TestControllerResetsEvidenceForClockRollbackAndLongSamplingGap(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 4
	cfg.AutoStart.UnclassifiedStableSeconds = 8
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 13, 50, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	outcome := focusedOutcome()
	controller.Evaluate(start, outcome, status)
	controller.Evaluate(start.Add(2*time.Second), outcome, status)
	controller.Evaluate(start.Add(time.Second), outcome, status)
	if diagnostic := controller.Diagnostics(); diagnostic.AccumulatedSeconds != 0 {
		t.Fatalf("clock rollback retained evidence: %+v", diagnostic)
	}
	if got := controller.Evaluate(start.Add(3*time.Second), outcome, status); got != nil {
		t.Fatalf("rollback evidence started too early: %+v", got)
	}
	controller.Evaluate(start.Add(30*time.Second), outcome, status)
	if diagnostic := controller.Diagnostics(); diagnostic.AccumulatedSeconds != 0 {
		t.Fatalf("long sampling gap retained evidence: %+v", diagnostic)
	}
}

func TestControllerAuditRateLimitUsesEventAndBlocker(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 20
	cfg.AutoStart.UnclassifiedStableSeconds = 20
	cfg.AutoStart.EvidenceGraceSeconds = 2
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	focused := focusedOutcome()
	distracted := focused
	distracted.Relation = state.RelationDistracted
	counts := map[auditRateKey]int{}
	consume := func() {
		if audit := controller.TakeDiagnosticAudit(); audit != nil {
			counts[auditRateKey{EventType: audit.EventType, Blocker: audit.Diagnostic.Blocker}]++
		}
	}
	for second := 0; second < 100; second++ {
		outcome := focused
		if second%2 == 1 {
			outcome = distracted
		}
		controller.Evaluate(start.Add(time.Duration(second)*time.Second), outcome, status)
		consume()
	}
	if counts[auditRateKey{EventType: "AUTO_START_ACCUMULATION_STARTED"}] != 1 {
		t.Fatalf("accumulation audit was not rate limited: %+v", counts)
	}
	if counts[auditRateKey{EventType: "AUTO_START_BLOCKED", Blocker: state.AutomationBlockerDistracted}] != 1 {
		t.Fatalf("distracted audit was not rate limited: %+v", counts)
	}
	controller.Evaluate(start.Add(301*time.Second), focused, status)
	consume()
	controller.Evaluate(start.Add(302*time.Second), distracted, status)
	consume()
	if counts[auditRateKey{EventType: "AUTO_START_BLOCKED", Blocker: state.AutomationBlockerDistracted}] != 2 {
		t.Fatalf("distracted audit did not reopen after five minutes: %+v", counts)
	}
	if len(controller.pendingAudits) > maxPendingAudits {
		t.Fatalf("pending audit queue exceeded bound: %d", len(controller.pendingAudits))
	}
}

func TestControllerEvidenceQueueHasHardBound(t *testing.T) {
	cfg := config.DefaultConfig().Automation
	cfg.Enabled = true
	cfg.AutoStart.FocusedStableSeconds = 1000
	cfg.AutoStart.UnclassifiedStableSeconds = 1000
	cfg.AutoStart.EvidenceGraceSeconds = 20
	cfg.TransitionCooldownSeconds = 1
	controller := New(cfg)
	start := time.Date(2026, 9, 19, 15, 0, 0, 0, time.UTC)
	status := state.SystemStatus{UserMode: state.UserModeStandby, PrivacyState: state.PrivacyNormal, Task: "Go"}
	for second := 0; second < 1000; second += 2 {
		controller.Evaluate(start.Add(time.Duration(second)*time.Second), focusedOutcome(), status)
	}
	if len(controller.evidence) > maxEvidenceSamples {
		t.Fatalf("evidence queue exceeded hard bound: %d", len(controller.evidence))
	}
}
