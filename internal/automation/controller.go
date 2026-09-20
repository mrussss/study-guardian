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
	maxEvidenceGap         = 15 * time.Second
	maxEvidenceSamples     = 256
	maxPendingAudits       = 16
)

type EvidenceSample struct {
	Start    time.Time
	End      time.Time
	Kind     state.AutomationSignalKind
	Duration int64
}

type auditRateKey struct {
	EventType string
	Blocker   state.AutomationBlocker
}

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

	evidence        []EvidenceSample
	lastEvaluatedAt time.Time
	lastSignal      state.AutomationSignalKind

	diagnostics   state.AutomationDiagnostics
	lastAuditAt   map[auditRateKey]time.Time
	pendingAudits []DiagnosticAudit
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
		lastAuditAt: make(map[auditRateKey]time.Time),
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
	c.evidence = nil
	c.lastEvaluatedAt = time.Time{}
	c.lastSignal = state.AutomationSignalNone
	c.diagnostics = next.diagnostics
	c.lastAuditAt = make(map[auditRateKey]time.Time)
	c.pendingAudits = nil
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
	if len(c.pendingAudits) == 0 {
		return nil
	}
	audit := c.pendingAudits[0]
	c.pendingAudits = c.pendingAudits[1:]
	audit.Diagnostic = cloneDiagnostic(audit.Diagnostic)
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
	c.evidence = nil
	c.lastEvaluatedAt = time.Time{}
	c.lastSignal = state.AutomationSignalNone
}

func (c *Controller) advanceEvidenceLocked(now time.Time, signal state.AutomationSignalKind) {
	if signal == state.AutomationSignalHardBlocked {
		c.resetEvidenceLocked()
		return
	}
	if c.lastEvaluatedAt.IsZero() || now.Before(c.lastEvaluatedAt) || now.Sub(c.lastEvaluatedAt) > maxEvidenceGap {
		c.resetEvidenceLocked()
		c.lastEvaluatedAt = now
		c.lastSignal = signal
		return
	}
	duration := int64(now.Sub(c.lastEvaluatedAt) / time.Second)
	if duration > 0 {
		c.evidence = append(c.evidence, EvidenceSample{
			Start: c.lastEvaluatedAt,
			End:   now,
			// The interval belongs to the signal that was already observed at
			// lastEvaluatedAt. Attribution to the new sample consumes evidence
			// one tick early and can turn a neutral gap into focus evidence.
			Kind:     c.lastSignal,
			Duration: duration,
		})
		if len(c.evidence) > maxEvidenceSamples {
			c.evidence = c.evidence[len(c.evidence)-maxEvidenceSamples:]
		}
	}
	c.trimEvidenceLocked(now)
	c.lastEvaluatedAt = now
	c.lastSignal = signal
}

func (c *Controller) trimEvidenceLocked(now time.Time) {
	window := time.Duration(c.cfg.AutoStart.UnclassifiedStableSeconds+c.cfg.AutoStart.EvidenceGraceSeconds) * time.Second
	cutoff := now.Add(-window)
	kept := c.evidence[:0]
	for _, sample := range c.evidence {
		if !sample.End.After(cutoff) {
			continue
		}
		if sample.Start.Before(cutoff) {
			sample.Start = cutoff
			sample.Duration = int64(sample.End.Sub(sample.Start) / time.Second)
		}
		kept = append(kept, sample)
	}
	c.evidence = kept
}

func (c *Controller) evidenceTotalsLocked(now time.Time, window time.Duration) (strong, candidate, neutral int64) {
	cutoff := now.Add(-window)
	for _, sample := range c.evidence {
		start := sample.Start
		if start.Before(cutoff) {
			start = cutoff
		}
		end := sample.End
		if end.After(now) {
			end = now
		}
		if !end.After(start) {
			continue
		}
		seconds := int64(end.Sub(start) / time.Second)
		if seconds <= 0 {
			continue
		}
		switch sample.Kind {
		case state.AutomationSignalStrongFocus:
			strong += seconds
		case state.AutomationSignalCandidate:
			candidate += seconds
		case state.AutomationSignalNeutralGap:
			neutral += seconds
		}
	}
	return strong, candidate, neutral
}

func (c *Controller) evidenceWindowsLocked(now time.Time) (strong, strongNeutral, candidate, candidateNeutral int64) {
	strong, _, strongNeutral = c.evidenceTotalsLocked(now, time.Duration(c.cfg.AutoStart.FocusedStableSeconds+c.cfg.AutoStart.EvidenceGraceSeconds)*time.Second)
	strongCandidate, candidateOnly, candidateNeutral := c.evidenceTotalsLocked(now, time.Duration(c.cfg.AutoStart.UnclassifiedStableSeconds+c.cfg.AutoStart.EvidenceGraceSeconds)*time.Second)
	return strong, strongNeutral, strongCandidate + candidateOnly, candidateNeutral
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
	key := auditRateKey{EventType: event, Blocker: diagnostic.Blocker}
	if last, ok := c.lastAuditAt[key]; ok && now.Sub(last) < diagnosticAuditEvery {
		return
	}
	if c.lastAuditAt == nil {
		c.lastAuditAt = make(map[auditRateKey]time.Time)
	}
	c.lastAuditAt[key] = now
	audit := DiagnosticAudit{EventType: event, Diagnostic: cloneDiagnostic(diagnostic)}
	for index := range c.pendingAudits {
		queuedKey := auditRateKey{EventType: c.pendingAudits[index].EventType, Blocker: c.pendingAudits[index].Diagnostic.Blocker}
		if queuedKey == key {
			c.pendingAudits[index] = audit
			return
		}
	}
	if len(c.pendingAudits) >= maxPendingAudits {
		c.pendingAudits = c.pendingAudits[1:]
	}
	c.pendingAudits = append(c.pendingAudits, audit)
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
	strongSeconds, strongNeutral, candidateSeconds, candidateNeutral := c.evidenceWindowsLocked(now)
	targetSignal := signal
	if signal == state.AutomationSignalNeutralGap {
		switch c.diagnostics.SignalKind {
		case state.AutomationSignalStrongFocus, state.AutomationSignalCandidate:
			targetSignal = c.diagnostics.SignalKind
		}
	}
	required := int64(0)
	accumulated := int64(0)
	neutralSeconds := int64(0)
	switch targetSignal {
	case state.AutomationSignalStrongFocus:
		required = int64(c.cfg.AutoStart.FocusedStableSeconds)
		accumulated = strongSeconds
		neutralSeconds = strongNeutral
	case state.AutomationSignalCandidate:
		required = int64(c.cfg.AutoStart.UnclassifiedStableSeconds)
		accumulated = candidateSeconds
		neutralSeconds = candidateNeutral
	}
	graceRemaining := int64(c.cfg.AutoStart.EvidenceGraceSeconds) - neutralSeconds
	if graceRemaining < 0 {
		graceRemaining = 0
	}
	diagnostic := state.AutomationDiagnostics{
		State:                 state.AutomationDiagnosticAccumulating,
		SignalKind:            targetSignal,
		AccumulatedSeconds:    accumulated,
		RequiredSeconds:       required,
		GraceRemainingSeconds: graceRemaining,
		Blocker:               blocker,
	}
	if required > 0 && neutralSeconds > int64(c.cfg.AutoStart.EvidenceGraceSeconds) {
		diagnostic.Blocker = state.AutomationBlockerInsufficientEvidence
	}
	if status.ManualOverrideUntil != nil && now.Before(*status.ManualOverrideUntil) {
		deadline := *status.ManualOverrideUntil
		diagnostic.ManualOverrideUntil = &deadline
	}
	c.updateDiagnosticStateLocked(diagnostic, now, status, signal)
}

func (c *Controller) updateDiagnosticStateLocked(diagnostic state.AutomationDiagnostics, now time.Time, status state.SystemStatus, currentSignal state.AutomationSignalKind) {
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
	} else if currentSignal == state.AutomationSignalNeutralGap && diagnostic.SignalKind != state.AutomationSignalNeutralGap && diagnostic.GraceRemainingSeconds > 0 {
		diagnostic.State = state.AutomationDiagnosticGrace
	} else if currentSignal != state.AutomationSignalNeutralGap && diagnostic.RequiredSeconds > 0 && diagnostic.AccumulatedSeconds >= diagnostic.RequiredSeconds {
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
		c.updateDiagnosticStateLocked(state.AutomationDiagnostics{State: state.AutomationDiagnosticDisabled, Blocker: state.AutomationBlockerAutomationDisabled}, now, status, state.AutomationSignalNone)
		return c.evaluatePauseResumeLocked(now, outcome, status, false)
	}
	if status.PendingAutomationIntent != nil {
		c.resetEvidenceLocked()
		c.updateDiagnosticStateLocked(state.AutomationDiagnostics{State: state.AutomationDiagnosticSuppressed, Blocker: state.AutomationBlockerPendingIntent}, now, status, state.AutomationSignalNone)
		return nil
	}
	cooldownActive := !c.lastTransition.IsZero() && now.Sub(c.lastTransition) < time.Duration(c.cfg.TransitionCooldownSeconds)*time.Second

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
		if cooldownActive {
			return nil
		}
		return c.evaluatePauseResumeLocked(now, outcome, status, focusedForResume)
	}
	c.resumeFocusedSince = time.Time{}

	if !c.cfg.AutoStart.Enabled {
		c.resetEvidenceLocked()
		c.setDiagnosticLocked(now, state.AutomationDiagnostics{
			State:   state.AutomationDiagnosticDisabled,
			Blocker: state.AutomationBlockerAutoStartDisabled,
		})
		return nil
	}
	if status.ManualOverrideUntil != nil && now.Before(*status.ManualOverrideUntil) {
		c.resetEvidenceLocked()
		deadline := *status.ManualOverrideUntil
		c.setDiagnosticLocked(now, state.AutomationDiagnostics{
			State:               state.AutomationDiagnosticBlocked,
			Blocker:             state.AutomationBlockerManualOverride,
			ManualOverrideUntil: &deadline,
		})
		return nil
	}

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

	signal := c.signalKind(outcome, status)
	c.advanceEvidenceLocked(now, signal)
	c.updateDiagnosticLocked(now, signal, state.AutomationBlockerNone, status)
	strongSeconds, strongNeutral, candidateSeconds, candidateNeutral := c.evidenceWindowsLocked(now)
	grace := int64(c.cfg.AutoStart.EvidenceGraceSeconds)
	strongReady := strongSeconds >= int64(c.cfg.AutoStart.FocusedStableSeconds) && strongNeutral <= grace
	candidateReady := candidateSeconds >= int64(c.cfg.AutoStart.UnclassifiedStableSeconds) && candidateNeutral <= grace
	if (signal == state.AutomationSignalStrongFocus || signal == state.AutomationSignalCandidate) &&
		(strongReady || candidateReady) {
		diagnostic := c.diagnostics
		diagnostic.State = state.AutomationDiagnosticReady
		diagnostic.Blocker = state.AutomationBlockerNone
		c.setDiagnosticLocked(now, diagnostic)
		if !cooldownActive {
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
