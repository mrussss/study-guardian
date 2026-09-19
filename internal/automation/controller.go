package automation

import (
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

const (
	candidateMinConfidence = 0.60
	diagnosticAuditEvery   = 5 * time.Minute
	maxEvidenceGap         = 240 * time.Second
)

type DiagnosticAudit struct {
	EventType  string
	Diagnostic state.AutomationDiagnostics
}

type Controller struct {
	mu                 sync.Mutex
	cfg                config.AutomationConfig
	lockedSince        time.Time
	resumeFocusedSince time.Time
	lastTransition     time.Time

	lastEvidenceAt   time.Time
	lastSignal       state.AutomationSignalKind
	strongSeconds    int64
	candidateSeconds int64
	neutralSeconds   int64

	diagnostics    state.AutomationDiagnostics
	lastAuditAt    time.Time
	lastAuditEvent string
	pendingAudit   *DiagnosticAudit
}

func New(cfg config.AutomationConfig) *Controller {
	if cfg.TransitionCooldownSeconds <= 0 {
		cfg.TransitionCooldownSeconds = 30
	}
	if cfg.ManualOverrideMinutes <= 0 {
		cfg.ManualOverrideMinutes = 30
	}
	if cfg.AutoStart.FocusedStableSeconds <= 0 {
		cfg.AutoStart.FocusedStableSeconds = 90
	}
	if cfg.AutoStart.UnclassifiedStableSeconds <= 0 {
		cfg.AutoStart.UnclassifiedStableSeconds = 180
	}
	if cfg.AutoStart.EvidenceGraceSeconds < 0 {
		cfg.AutoStart.EvidenceGraceSeconds = 20
	}
	if cfg.AutoStart.MinConfidence <= 0 {
		cfg.AutoStart.MinConfidence = .80
	}
	if cfg.AutoPause.IdleStaticSeconds <= 0 {
		cfg.AutoPause.IdleStaticSeconds = 300
	}
	if cfg.AutoPause.IdleDynamicSeconds <= 0 {
		cfg.AutoPause.IdleDynamicSeconds = 900
	}
	if cfg.AutoPause.LockedSeconds <= 0 {
		cfg.AutoPause.LockedSeconds = 15
	}
	if cfg.AutoResume.FocusedStableSeconds <= 0 {
		cfg.AutoResume.FocusedStableSeconds = 45
	}
	return &Controller{
		cfg: cfg,
		diagnostics: state.AutomationDiagnostics{
			State: state.AutomationDiagnosticInactive,
		},
	}
}

func (c *Controller) UpdateConfig(cfg config.AutomationConfig) {
	if c == nil {
		return
	}
	next := New(cfg)
	c.mu.Lock()
	c.cfg = next.cfg
	c.lockedSince = time.Time{}
	c.resumeFocusedSince = time.Time{}
	c.lastTransition = time.Time{}
	c.lastEvidenceAt = time.Time{}
	c.lastSignal = state.AutomationSignalNone
	c.strongSeconds = 0
	c.candidateSeconds = 0
	c.neutralSeconds = 0
	c.diagnostics = next.diagnostics
	c.lastAuditAt = time.Time{}
	c.lastAuditEvent = ""
	c.pendingAudit = nil
	c.mu.Unlock()
}

func (c *Controller) Diagnostics() state.AutomationDiagnostics {
	if c == nil {
		return state.AutomationDiagnostics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneDiagnostic(c.diagnostics)
}

func (c *Controller) TakeDiagnosticAudit() *DiagnosticAudit {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingAudit == nil {
		return nil
	}
	audit := *c.pendingAudit
	audit.Diagnostic = cloneDiagnostic(audit.Diagnostic)
	c.pendingAudit = nil
	return &audit
}

func cloneDiagnostic(value state.AutomationDiagnostics) state.AutomationDiagnostics {
	copy := value
	if value.ManualOverrideUntil != nil {
		deadline := *value.ManualOverrideUntil
		copy.ManualOverrideUntil = &deadline
	}
	return copy
}

func (c *Controller) resetEvidenceLocked() {
	c.lastEvidenceAt = time.Time{}
	c.lastSignal = state.AutomationSignalNone
	c.strongSeconds = 0
	c.candidateSeconds = 0
	c.neutralSeconds = 0
}

func (c *Controller) advanceEvidenceLocked(now time.Time, signal state.AutomationSignalKind) {
	if c.lastEvidenceAt.IsZero() || now.Before(c.lastEvidenceAt) || now.Sub(c.lastEvidenceAt) > maxEvidenceGap {
		c.strongSeconds = 0
		c.candidateSeconds = 0
		c.neutralSeconds = 0
	} else {
		delta := int64(now.Sub(c.lastEvidenceAt) / time.Second)
		if signal == state.AutomationSignalNeutralGap {
			c.neutralSeconds += delta
		} else {
			switch c.lastSignal {
			case state.AutomationSignalStrongFocus:
				c.strongSeconds += delta
			case state.AutomationSignalCandidate:
				c.candidateSeconds += delta
			}
		}
	}
	if signal == state.AutomationSignalHardBlocked {
		c.resetEvidenceLocked()
		return
	}
	if signal == state.AutomationSignalNeutralGap && c.neutralSeconds > int64(c.cfg.AutoStart.EvidenceGraceSeconds) {
		c.strongSeconds = 0
		c.candidateSeconds = 0
	}
	if signal != state.AutomationSignalNeutralGap {
		c.neutralSeconds = 0
	}
	c.lastEvidenceAt = now
	c.lastSignal = signal
}

func (c *Controller) setDiagnosticLocked(now time.Time, diagnostic state.AutomationDiagnostics) {
	diagnostic.UpdatedAt = now
	previous := c.diagnostics
	c.diagnostics = diagnostic
	event := ""
	switch {
	case previous.State != diagnostic.State:
		switch diagnostic.State {
		case state.AutomationDiagnosticAccumulating, state.AutomationDiagnosticGrace:
			event = "AUTO_START_ACCUMULATION_STARTED"
		case state.AutomationDiagnosticBlocked:
			event = "AUTO_START_BLOCKED"
		case state.AutomationDiagnosticReady:
			event = "AUTO_START_READY"
		case state.AutomationDiagnosticInactive, state.AutomationDiagnosticDisabled:
			if previous.State == state.AutomationDiagnosticAccumulating || previous.State == state.AutomationDiagnosticGrace {
				event = "AUTO_START_ACCUMULATION_RESET"
			}
		}
	case diagnostic.State == state.AutomationDiagnosticBlocked && previous.Blocker != diagnostic.Blocker:
		event = "AUTO_START_BLOCKED"
	}
	if event == "" {
		return
	}
	if event == c.lastAuditEvent && !c.lastAuditAt.IsZero() && now.Sub(c.lastAuditAt) < diagnosticAuditEvery {
		return
	}
	c.lastAuditEvent = event
	c.lastAuditAt = now
	c.pendingAudit = &DiagnosticAudit{EventType: event, Diagnostic: cloneDiagnostic(diagnostic)}
}

func (c *Controller) hardBlocker(outcome state.TickOutcome, status state.SystemStatus) state.AutomationBlocker {
	if !outcome.ActivityValid {
		return state.AutomationBlockerActivityUnavailable
	}
	if outcome.Locked {
		return state.AutomationBlockerLocked
	}
	if outcome.AfkSeconds > 0 {
		return state.AutomationBlockerAFK
	}
	if status.PrivacyState == state.PrivacySensitive {
		return state.AutomationBlockerPrivacySensitive
	}
	if outcome.Relation == state.RelationDistracted {
		return state.AutomationBlockerDistracted
	}
	if strings.TrimSpace(status.Task) == "" {
		return state.AutomationBlockerNoTask
	}
	return state.AutomationBlockerNone
}

func (c *Controller) signalKind(outcome state.TickOutcome, status state.SystemStatus) state.AutomationSignalKind {
	if outcome.Relation == state.RelationFocused &&
		outcome.Interaction == state.InteractionActive &&
		outcome.Classification.Confidence >= c.cfg.AutoStart.MinConfidence {
		return state.AutomationSignalStrongFocus
	}
	if !c.cfg.AutoStart.AllowUnclassified ||
		outcome.Interaction != state.InteractionActive ||
		outcome.Relation == state.RelationDistracted ||
		strings.TrimSpace(status.Task) == "" ||
		outcome.Classification.Confidence < candidateMinConfidence {
		return state.AutomationSignalNeutralGap
	}
	if outcome.Relation == state.RelationUnknown && state.IsExplicitStudyActivity(outcome.Classification.Activity) {
		return state.AutomationSignalCandidate
	}
	if outcome.Relation == state.RelationFocused {
		return state.AutomationSignalCandidate
	}
	return state.AutomationSignalNeutralGap
}

func (c *Controller) updateDiagnosticLocked(now time.Time, signal state.AutomationSignalKind, blocker state.AutomationBlocker, status state.SystemStatus) {
	required := int64(0)
	accumulated := int64(0)
	switch signal {
	case state.AutomationSignalStrongFocus:
		required = int64(c.cfg.AutoStart.FocusedStableSeconds)
		accumulated = c.strongSeconds
	case state.AutomationSignalCandidate:
		required = int64(c.cfg.AutoStart.UnclassifiedStableSeconds)
		accumulated = c.candidateSeconds
	}
	graceRemaining := int64(c.cfg.AutoStart.EvidenceGraceSeconds) - c.neutralSeconds
	if graceRemaining < 0 {
		graceRemaining = 0
	}
	diagnostic := state.AutomationDiagnostics{
		State:                 state.AutomationDiagnosticAccumulating,
		SignalKind:            signal,
		AccumulatedSeconds:    accumulated,
		RequiredSeconds:       required,
		GraceRemainingSeconds: graceRemaining,
		Blocker:               blocker,
	}
	if status.ManualOverrideUntil != nil && now.Before(*status.ManualOverrideUntil) {
		deadline := *status.ManualOverrideUntil
		diagnostic.ManualOverrideUntil = &deadline
	}
	c.updateDiagnosticStateLocked(diagnostic, now, status)
}

func (c *Controller) updateDiagnosticStateLocked(diagnostic state.AutomationDiagnostics, now time.Time, status state.SystemStatus) {
	if !c.cfg.Enabled {
		diagnostic.State = state.AutomationDiagnosticDisabled
		diagnostic.Blocker = state.AutomationBlockerAutomationDisabled
	} else if !c.cfg.AutoStart.Enabled {
		diagnostic.State = state.AutomationDiagnosticDisabled
		diagnostic.Blocker = state.AutomationBlockerAutoStartDisabled
	} else if status.UserMode != state.UserModeStandby {
		diagnostic.State = state.AutomationDiagnosticInactive
		diagnostic.Blocker = state.AutomationBlockerNotStandby
	} else if status.PendingAutomationIntent != nil {
		diagnostic.State = state.AutomationDiagnosticSuppressed
		diagnostic.Blocker = state.AutomationBlockerPendingIntent
	} else if diagnostic.Blocker != state.AutomationBlockerNone {
		diagnostic.State = state.AutomationDiagnosticBlocked
	} else if diagnostic.SignalKind == state.AutomationSignalNeutralGap && c.neutralSeconds > 0 {
		diagnostic.State = state.AutomationDiagnosticGrace
	} else if (diagnostic.SignalKind == state.AutomationSignalStrongFocus && c.strongSeconds >= int64(c.cfg.AutoStart.FocusedStableSeconds)) ||
		(diagnostic.SignalKind == state.AutomationSignalCandidate && c.candidateSeconds >= int64(c.cfg.AutoStart.UnclassifiedStableSeconds)) {
		diagnostic.State = state.AutomationDiagnosticReady
	} else {
		diagnostic.State = state.AutomationDiagnosticAccumulating
	}
	c.setDiagnosticLocked(now, diagnostic)
}

func (c *Controller) evaluatePauseResumeLocked(now time.Time, outcome state.TickOutcome, status state.SystemStatus, focused bool) *state.AutomationIntent {
	if status.UserMode == state.UserModeStudy && c.cfg.AutoPause.Enabled {
		snoozed := status.AutoPauseSnoozeUntil != nil && now.Before(*status.AutoPauseSnoozeUntil)
		if snoozed {
			c.lockedSince = time.Time{}
		} else {
			if outcome.Locked {
				if c.lockedSince.IsZero() {
					c.lockedSince = now
				}
				if now.Sub(c.lockedSince) >= time.Duration(c.cfg.AutoPause.LockedSeconds)*time.Second {
					c.lastTransition = now
					return &state.AutomationIntent{Transition: state.AutomationPause, Reason: state.PauseReasonLocked, RequiresConfirmation: c.cfg.AutoPause.Confirm}
				}
			} else {
				c.lockedSince = time.Time{}
			}
			threshold := 0
			switch outcome.Interaction {
			case state.InteractionIdleStatic:
				threshold = c.cfg.AutoPause.IdleStaticSeconds
			case state.InteractionIdleDynamic:
				threshold = c.cfg.AutoPause.IdleDynamicSeconds
			}
			if threshold > 0 && status.AfkSeconds >= int64(threshold) {
				c.lastTransition = now
				return &state.AutomationIntent{Transition: state.AutomationPause, Reason: state.PauseReasonIdle, RequiresConfirmation: c.cfg.AutoPause.Confirm}
			}
		}
	}
	if status.UserMode == state.UserModeBreak && status.ModeOrigin == state.ModeOriginAutomation && status.AutoResumeEligible && c.cfg.AutoResume.Enabled && focused {
		if c.resumeFocusedSince.IsZero() {
			c.resumeFocusedSince = now
		}
		if now.Sub(c.resumeFocusedSince) >= time.Duration(c.cfg.AutoResume.FocusedStableSeconds)*time.Second {
			c.lastTransition = now
			return &state.AutomationIntent{Transition: state.AutomationResume, Reason: state.PauseReasonNone}
		}
	} else if status.UserMode == state.UserModeBreak {
		c.resumeFocusedSince = time.Time{}
	}
	return nil
}

// Evaluate is pure with respect to the Manager: it only returns an intent.
// The caller applies that intent after TickWithClassification has released its
// mutex, preventing automation from re-entering time accounting.
func (c *Controller) Evaluate(now time.Time, outcome state.TickOutcome, status state.SystemStatus) *state.AutomationIntent {
	if c == nil || now.IsZero() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cfg.Enabled {
		c.resetEvidenceLocked()
		c.updateDiagnosticStateLocked(state.AutomationDiagnostics{State: state.AutomationDiagnosticDisabled, Blocker: state.AutomationBlockerAutomationDisabled}, now, status)
		return c.evaluatePauseResumeLocked(now, outcome, status, false)
	}
	if status.PendingAutomationIntent != nil {
		c.resetEvidenceLocked()
		c.updateDiagnosticStateLocked(state.AutomationDiagnostics{State: state.AutomationDiagnosticSuppressed, Blocker: state.AutomationBlockerPendingIntent}, now, status)
		return nil
	}
	if !c.lastTransition.IsZero() && now.Sub(c.lastTransition) < time.Duration(c.cfg.TransitionCooldownSeconds)*time.Second {
		return nil
	}

	focusedForResume := outcome.Relation == state.RelationFocused &&
		outcome.Interaction == state.InteractionActive &&
		outcome.ActivityValid &&
		!outcome.Locked &&
		status.PrivacyState == state.PrivacyNormal &&
		outcome.Classification.Confidence >= c.cfg.AutoStart.MinConfidence

	if status.UserMode != state.UserModeStandby {
		c.resetEvidenceLocked()
		c.setDiagnosticLocked(now, state.AutomationDiagnostics{
			State:      state.AutomationDiagnosticInactive,
			SignalKind: state.AutomationSignalNone,
			Blocker:    state.AutomationBlockerNotStandby,
		})
		return c.evaluatePauseResumeLocked(now, outcome, status, focusedForResume)
	}
	c.resumeFocusedSince = time.Time{}

	blocker := c.hardBlocker(outcome, status)
	if blocker != state.AutomationBlockerNone {
		c.resetEvidenceLocked()
		c.setDiagnosticLocked(now, state.AutomationDiagnostics{
			State:      state.AutomationDiagnosticBlocked,
			SignalKind: state.AutomationSignalHardBlocked,
			Blocker:    blocker,
		})
		return nil
	}
	if !c.cfg.AutoStart.Enabled {
		c.resetEvidenceLocked()
		c.setDiagnosticLocked(now, state.AutomationDiagnostics{
			State:   state.AutomationDiagnosticDisabled,
			Blocker: state.AutomationBlockerAutoStartDisabled,
		})
		return nil
	}

	signal := c.signalKind(outcome, status)
	c.advanceEvidenceLocked(now, signal)
	c.updateDiagnosticLocked(now, signal, state.AutomationBlockerNone, status)
	strongReady := c.strongSeconds >= int64(c.cfg.AutoStart.FocusedStableSeconds)
	candidateReady := c.candidateSeconds >= int64(c.cfg.AutoStart.UnclassifiedStableSeconds)
	if status.ManualOverrideUntil != nil && now.Before(*status.ManualOverrideUntil) {
		diagnostic := c.diagnostics
		diagnostic.State = state.AutomationDiagnosticBlocked
		diagnostic.Blocker = state.AutomationBlockerManualOverride
		c.setDiagnosticLocked(now, diagnostic)
		return nil
	}
	if (signal == state.AutomationSignalStrongFocus && strongReady) ||
		(signal == state.AutomationSignalCandidate && candidateReady) {
		diagnostic := c.diagnostics
		diagnostic.State = state.AutomationDiagnosticReady
		diagnostic.Blocker = state.AutomationBlockerNone
		c.setDiagnosticLocked(now, diagnostic)
		if c.lastTransition.IsZero() || now.Sub(c.lastTransition) >= time.Duration(c.cfg.TransitionCooldownSeconds)*time.Second {
			c.lastTransition = now
			return &state.AutomationIntent{
				Transition:           state.AutomationStart,
				Task:                 strings.TrimSpace(status.Task),
				Reason:               state.PauseReasonNone,
				RequiresConfirmation: c.cfg.AutoStart.Confirm,
			}
		}
	}
	return nil
}
