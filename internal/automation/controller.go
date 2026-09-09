package automation

import (
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

type Controller struct {
	mu             sync.Mutex
	cfg            config.AutomationConfig
	focusedSince   time.Time
	focusedKind    string
	lockedSince    time.Time
	staticSince    time.Time
	lastTransition time.Time
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
	if cfg.AutoStart.MinConfidence <= 0 {
		cfg.AutoStart.MinConfidence = .80
	}
	if cfg.AutoPause.IdleStaticSeconds <= 0 {
		cfg.AutoPause.IdleStaticSeconds = 300
	}
	if cfg.AutoPause.LockedSeconds <= 0 {
		cfg.AutoPause.LockedSeconds = 15
	}
	if cfg.AutoResume.FocusedStableSeconds <= 0 {
		cfg.AutoResume.FocusedStableSeconds = 45
	}
	return &Controller{cfg: cfg}
}

func (c *Controller) UpdateConfig(cfg config.AutomationConfig) {
	if c == nil {
		return
	}
	next := New(cfg)
	c.mu.Lock()
	c.cfg = next.cfg
	c.focusedSince = time.Time{}
	c.lockedSince = time.Time{}
	c.staticSince = time.Time{}
	c.lastTransition = time.Time{}
	c.focusedKind = ""
	c.mu.Unlock()
}

// Evaluate is pure with respect to the Manager: it only returns an intent.
// The caller applies that intent after TickWithClassification has released its
// mutex, preventing automation from re-entering time accounting.
func (c *Controller) Evaluate(now time.Time, outcome state.TickOutcome, status state.SystemStatus) *state.AutomationIntent {
	if c == nil || !c.cfg.Enabled || now.IsZero() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if status.PendingAutomationIntent != nil {
		return nil
	}
	if !c.lastTransition.IsZero() && now.Sub(c.lastTransition) < time.Duration(c.cfg.TransitionCooldownSeconds)*time.Second {
		return nil
	}

	focusKind := ""
	stableSeconds := c.cfg.AutoStart.FocusedStableSeconds
	focused := outcome.Relation == state.RelationFocused && outcome.Interaction == state.InteractionActive && outcome.ActivityValid && status.PrivacyState == state.PrivacyNormal && outcome.Classification.Confidence >= c.cfg.AutoStart.MinConfidence
	if focused {
		focusKind = "FOCUSED"
	} else if c.cfg.AutoStart.AllowUnclassified &&
		outcome.Relation == state.RelationUnknown &&
		outcome.Interaction == state.InteractionActive &&
		outcome.ActivityValid &&
		status.PrivacyState == state.PrivacyNormal &&
		strings.TrimSpace(status.Task) != "" &&
		state.IsExplicitStudyActivity(outcome.Classification.Activity) {
		focusKind = "UNCLASSIFIED_STUDY"
		if stableSeconds < 180 {
			stableSeconds = 180
		}
	}
	if focusKind != "" {
		if c.focusedSince.IsZero() || c.focusedKind != focusKind {
			c.focusedSince = now
			c.focusedKind = focusKind
		}
	} else {
		c.focusedSince = time.Time{}
		c.focusedKind = ""
	}

	if status.UserMode == state.UserModeStandby && c.cfg.AutoStart.Enabled && focusKind != "" && strings.TrimSpace(status.Task) != "" {
		if now.Sub(c.focusedSince) >= time.Duration(stableSeconds)*time.Second {
			c.lastTransition = now
			return &state.AutomationIntent{Transition: state.AutomationStart, Task: strings.TrimSpace(status.Task), Reason: state.PauseReasonNone, RequiresConfirmation: c.cfg.AutoStart.Confirm}
		}
	}

	if status.UserMode == state.UserModeStudy && c.cfg.AutoPause.Enabled {
		snoozed := status.AutoPauseSnoozeUntil != nil && now.Before(*status.AutoPauseSnoozeUntil)
		if snoozed {
			c.lockedSince = time.Time{}
			c.staticSince = time.Time{}
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
			if outcome.Interaction == state.InteractionIdleStatic {
				if c.staticSince.IsZero() {
					c.staticSince = now
				}
				if now.Sub(c.staticSince) >= time.Duration(c.cfg.AutoPause.IdleStaticSeconds)*time.Second {
					c.lastTransition = now
					return &state.AutomationIntent{Transition: state.AutomationPause, Reason: state.PauseReasonIdle, RequiresConfirmation: c.cfg.AutoPause.Confirm}
				}
			} else {
				c.staticSince = time.Time{}
			}
		}
	}

	if status.UserMode == state.UserModeBreak && status.ModeOrigin == state.ModeOriginAutomation && status.AutoResumeEligible && c.cfg.AutoResume.Enabled && focused {
		if now.Sub(c.focusedSince) >= time.Duration(c.cfg.AutoResume.FocusedStableSeconds)*time.Second {
			c.lastTransition = now
			return &state.AutomationIntent{Transition: state.AutomationResume, Reason: state.PauseReasonNone}
		}
	}
	return nil
}
