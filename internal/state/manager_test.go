package state

import (
	"testing"
	"time"
)

func TestStateManagerTransitions(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	clock := NewFakeClock(now)
	mgr := NewManager(clock)

	// Initial status should be STANDBY
	st := mgr.GetStatus()
	if st.UserMode != UserModeStandby {
		t.Fatalf("expected initial mode STANDBY, got %s", st.UserMode)
	}

	// Transition to STUDY
	err := mgr.SetModeStudy("Go Concurrency Lab")
	if err != nil {
		t.Fatalf("failed to transition to STUDY: %v", err)
	}
	st = mgr.GetStatus()
	if st.UserMode != UserModeStudy || st.Task != "Go Concurrency Lab" {
		t.Fatalf("expected STUDY with task, got mode %s task %s", st.UserMode, st.Task)
	}

	// Advance time by 15 minutes
	clock.Advance(15 * time.Minute)
	st = mgr.GetStatus()
	if st.StudySeconds != 900 {
		t.Fatalf("expected 900 study seconds, got %d", st.StudySeconds)
	}

	// Transition to BREAK
	err = mgr.SetModeBreak()
	if err != nil {
		t.Fatalf("failed to transition to BREAK: %v", err)
	}
	st = mgr.GetStatus()
	if st.UserMode != UserModeBreak {
		t.Fatalf("expected mode BREAK, got %s", st.UserMode)
	}
	if st.StudySeconds != 900 {
		t.Fatalf("expected preserved 900 study seconds, got %d", st.StudySeconds)
	}

	// Advance 5 minutes break
	clock.Advance(5 * time.Minute)
	st = mgr.GetStatus()
	if st.BreakSeconds != 300 {
		t.Fatalf("expected 300 break seconds, got %d", st.BreakSeconds)
	}

	// Transition to OFF
	err = mgr.SetModeOff()
	if err != nil {
		t.Fatalf("failed to transition to OFF: %v", err)
	}
	st = mgr.GetStatus()
	if st.UserMode != UserModeOff {
		t.Fatalf("expected mode OFF, got %s", st.UserMode)
	}

	// Record feedback
	err = mgr.RecordFeedback("ev-123", "ACTUALLY_STUDYING")
	if err != nil {
		t.Fatalf("failed to record feedback: %v", err)
	}
}
