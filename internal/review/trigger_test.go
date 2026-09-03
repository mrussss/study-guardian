package review

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestReviewTriggerDebounceCancelAndReentry(t *testing.T) {
	var mu sync.Mutex
	var dates []string
	trigger := NewReviewTrigger(time.Minute, func(_ context.Context, date string) error {
		mu.Lock()
		defer mu.Unlock()
		dates = append(dates, date)
		return nil
	})
	defer trigger.Close()
	now := time.Date(2026, 9, 3, 23, 59, 0, 0, time.FixedZone("test", 8*60*60))
	trigger.OnModeChanged("OFF", now)
	if !trigger.Pending() || trigger.RunDue(now.Add(59*time.Second)) {
		t.Fatal("OFF debounce fired too early")
	}
	trigger.OnModeChanged("STUDY", now.Add(59*time.Second))
	if trigger.Pending() || trigger.RunDue(now.Add(2*time.Minute)) {
		t.Fatal("leaving OFF did not cancel pending review")
	}
	trigger.OnModeChanged("OFF", now.Add(3*time.Minute))
	if !trigger.RunDue(now.Add(4 * time.Minute)) {
		t.Fatal("second OFF entry did not fire after debounce")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dates) != 1 || dates[0] != "2026-09-04" {
		t.Fatalf("dates=%v, expected captured next-day date", dates)
	}
}

func TestReviewTriggerPreventsConcurrentGeneration(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	trigger := NewReviewTrigger(time.Second, func(_ context.Context, _ string) error {
		close(started)
		<-release
		return nil
	})
	defer trigger.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	trigger.OnModeChanged("OFF", now)
	completed := make(chan bool, 1)
	go func() { completed <- trigger.RunDue(now.Add(time.Second)) }()
	<-started
	trigger.OnModeChanged("OFF", now.Add(2*time.Second))
	if trigger.RunDue(now.Add(3 * time.Second)) {
		t.Fatal("concurrent review generation was allowed")
	}
	close(release)
	if !<-completed {
		t.Fatal("first review generation did not run")
	}
}
