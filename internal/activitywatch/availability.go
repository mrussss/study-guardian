package activitywatch

import (
	"sync"
	"time"
)

type AvailabilityPhase string

const (
	PhaseAvailable   AvailabilityPhase = "AVAILABLE"
	PhaseDegraded    AvailabilityPhase = "DEGRADED"
	PhaseUnavailable AvailabilityPhase = "UNAVAILABLE"
)

type AvailabilityStatus struct {
	LastSuccessAt       time.Time
	ConsecutiveFailures int
	UnavailableSince    time.Time
	StableOK            bool
	Phase               AvailabilityPhase
}

type AvailabilityTiming struct {
	FailureThreshold  int
	FailureWindow     time.Duration
	RecoverySuccesses int
}

var DefaultAvailabilityTiming = AvailabilityTiming{
	FailureThreshold:  3,
	FailureWindow:     20 * time.Second,
	RecoverySuccesses: 2,
}

// AvailabilityDebouncer keeps a single transient ActivityWatch failure from
// changing supervision semantics. It exposes both the debounced boolean used
// by the tracker and a phase for sparse, useful diagnostics.
type AvailabilityDebouncer struct {
	mu                   sync.Mutex
	timing               AvailabilityTiming
	lastSuccessAt        time.Time
	unavailableSince     time.Time
	consecutiveFailures  int
	consecutiveSuccesses int
	stableOK             bool
	phase                AvailabilityPhase
}

func NewAvailabilityDebouncer() *AvailabilityDebouncer {
	return NewAvailabilityDebouncerWithTiming(DefaultAvailabilityTiming)
}

func NewAvailabilityDebouncerWithTiming(timing AvailabilityTiming) *AvailabilityDebouncer {
	if timing.FailureThreshold <= 0 {
		timing.FailureThreshold = DefaultAvailabilityTiming.FailureThreshold
	}
	if timing.FailureWindow <= 0 {
		timing.FailureWindow = DefaultAvailabilityTiming.FailureWindow
	}
	if timing.RecoverySuccesses <= 0 {
		timing.RecoverySuccesses = DefaultAvailabilityTiming.RecoverySuccesses
	}
	return &AvailabilityDebouncer{
		timing:   timing,
		stableOK: true,
		phase:    PhaseAvailable,
	}
}

func (d *AvailabilityDebouncer) Observe(now time.Time, success bool) AvailabilityStatus {
	if d == nil {
		return AvailabilityStatus{StableOK: success, Phase: phaseForSuccess(success)}
	}
	if now.IsZero() {
		now = time.Now()
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if success {
		d.lastSuccessAt = now
		d.consecutiveFailures = 0
		d.consecutiveSuccesses++
		if !d.stableOK {
			if d.consecutiveSuccesses >= d.timing.RecoverySuccesses {
				d.stableOK = true
				d.phase = PhaseAvailable
				d.unavailableSince = time.Time{}
			} else {
				d.phase = PhaseDegraded
			}
		} else if d.phase == PhaseDegraded && d.consecutiveSuccesses >= d.timing.RecoverySuccesses {
			d.phase = PhaseAvailable
			d.unavailableSince = time.Time{}
		}
		return d.statusLocked()
	}

	d.consecutiveSuccesses = 0
	d.consecutiveFailures++
	if d.unavailableSince.IsZero() {
		d.unavailableSince = now
	}
	if d.stableOK {
		d.phase = PhaseDegraded
		if d.consecutiveFailures >= d.timing.FailureThreshold || now.Sub(d.unavailableSince) >= d.timing.FailureWindow {
			d.stableOK = false
			d.phase = PhaseUnavailable
		}
	} else {
		d.phase = PhaseUnavailable
	}
	return d.statusLocked()
}

func (d *AvailabilityDebouncer) statusLocked() AvailabilityStatus {
	return AvailabilityStatus{
		LastSuccessAt:       d.lastSuccessAt,
		ConsecutiveFailures: d.consecutiveFailures,
		UnavailableSince:    d.unavailableSince,
		StableOK:            d.stableOK,
		Phase:               d.phase,
	}
}

func phaseForSuccess(success bool) AvailabilityPhase {
	if success {
		return PhaseAvailable
	}
	return PhaseUnavailable
}
