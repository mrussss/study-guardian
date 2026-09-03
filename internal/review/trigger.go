package review

import (
	"context"
	"sync"
	"time"

	"study-guardian/internal/storage"
)

// ReviewGenerator is the narrow background dependency used by the OFF
// trigger. It keeps state transitions independent from review latency.
type ReviewGenerator interface {
	Generate(context.Context, string) (storage.DailyReviewRecord, error)
}

type ReviewTrigger struct {
	mu       sync.Mutex
	debounce time.Duration
	generate func(context.Context, string) error
	ctx      context.Context
	cancel   context.CancelFunc
	timer    *time.Timer
	deadline time.Time
	date     string
	pending  bool
	running  bool
}

func NewReviewTrigger(debounce time.Duration, generate func(context.Context, string) error) *ReviewTrigger {
	if debounce <= 0 {
		debounce = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ReviewTrigger{debounce: debounce, generate: generate, ctx: ctx, cancel: cancel}
}

// OnModeChanged starts a fresh debounce on each OFF entry and cancels it on
// every active mode. The date is captured at the transition, not when the
// timer fires across midnight.
func (t *ReviewTrigger) OnModeChanged(mode string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if mode != "OFF" {
		t.pending = false
		t.deadline = time.Time{}
		t.date = ""
		return
	}
	t.pending = true
	t.deadline = now.Add(t.debounce)
	t.date = now.Format("2006-01-02")
	t.timer = time.AfterFunc(t.debounce, t.fire)
}

// RunDue is deterministic test/support machinery. Production uses the same
// single-flight path through time.AfterFunc; callers may invoke this after a
// fake clock advance without sleeping.
func (t *ReviewTrigger) RunDue(now time.Time) bool {
	t.mu.Lock()
	if !t.pending || now.Before(t.deadline) {
		t.mu.Unlock()
		return false
	}
	t.pending = false
	date := t.date
	t.mu.Unlock()
	return t.run(date)
}

func (t *ReviewTrigger) fire() {
	t.mu.Lock()
	if !t.pending {
		t.mu.Unlock()
		return
	}
	t.pending = false
	date := t.date
	t.timer = nil
	t.mu.Unlock()
	_ = t.run(date)
}

func (t *ReviewTrigger) run(date string) bool {
	t.mu.Lock()
	if t.running || t.generate == nil {
		t.mu.Unlock()
		return false
	}
	t.running = true
	ctx := t.ctx
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()
	_ = t.generate(ctx, date)
	return true
}

func (t *ReviewTrigger) Pending() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending
}

func (t *ReviewTrigger) Close() {
	t.mu.Lock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.pending = false
	t.cancel()
	t.mu.Unlock()
}
