package activitywatch

import (
	"testing"
	"time"
)

func TestAvailabilityDebouncerKeepsTransientFailureDegraded(t *testing.T) {
	d := NewAvailabilityDebouncerWithTiming(AvailabilityTiming{FailureThreshold: 3, FailureWindow: 20 * time.Second, RecoverySuccesses: 2})
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if got := d.Observe(start, true); !got.StableOK || got.Phase != PhaseAvailable {
		t.Fatalf("initial=%+v", got)
	}
	if got := d.Observe(start.Add(2*time.Second), false); !got.StableOK || got.Phase != PhaseDegraded || got.ConsecutiveFailures != 1 {
		t.Fatalf("transient failure=%+v", got)
	}
	if got := d.Observe(start.Add(4*time.Second), true); !got.StableOK || got.Phase != PhaseDegraded {
		t.Fatalf("first recovery=%+v", got)
	}
	got := d.Observe(start.Add(6*time.Second), true)
	if !got.StableOK || got.Phase != PhaseAvailable || got.ConsecutiveFailures != 0 || !got.LastSuccessAt.Equal(start.Add(6*time.Second)) {
		t.Fatalf("recovered=%+v", got)
	}
}

func TestAvailabilityDebouncerMarksUnavailableOnceThresholdReachedAndRecovers(t *testing.T) {
	d := NewAvailabilityDebouncerWithTiming(AvailabilityTiming{FailureThreshold: 3, FailureWindow: time.Minute, RecoverySuccesses: 2})
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	d.Observe(start, true)
	d.Observe(start.Add(time.Second), false)
	d.Observe(start.Add(2*time.Second), false)
	got := d.Observe(start.Add(3*time.Second), false)
	if got.StableOK || got.Phase != PhaseUnavailable || got.ConsecutiveFailures != 3 || got.UnavailableSince.IsZero() {
		t.Fatalf("unavailable=%+v", got)
	}
	if got := d.Observe(start.Add(4*time.Second), true); got.StableOK || got.Phase != PhaseDegraded {
		t.Fatalf("first unavailable recovery=%+v", got)
	}
	got = d.Observe(start.Add(5*time.Second), true)
	if !got.StableOK || got.Phase != PhaseAvailable || !got.UnavailableSince.IsZero() {
		t.Fatalf("second unavailable recovery=%+v", got)
	}
}
