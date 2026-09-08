package distraction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type Timing struct {
	DistractedStableFor time.Duration
	RecoveryStableFor   time.Duration
	UnknownGrace        time.Duration
}

var DefaultTiming = Timing{
	DistractedStableFor: 12 * time.Second,
	RecoveryStableFor:   20 * time.Second,
	UnknownGrace:        30 * time.Second,
}

type Input struct {
	Now             time.Time
	UserMode        state.UserMode
	ActivityWatchOK bool
	ActivityFresh   bool
	Privacy         state.PrivacyState
	Relation        state.TaskRelation
	Confidence      float64
	Source          string
	Interaction     state.InteractionState
	Locked          bool
	App             string
	Title           string
	Domain          string
	Task            string
	ReminderLevel   string
}

type Tracker struct {
	mu            sync.Mutex
	store         *storage.Storage
	timing        Timing
	pendingSince  time.Time
	unknownSince  time.Time
	recoverySince time.Time
	current       *storage.DistractionEventRecord
}

var sequence atomic.Uint64

func New(store *storage.Storage) *Tracker {
	return NewWithTiming(store, DefaultTiming)
}

func NewWithTiming(store *storage.Storage, timing Timing) *Tracker {
	if timing.DistractedStableFor <= 0 {
		timing.DistractedStableFor = DefaultTiming.DistractedStableFor
	}
	if timing.RecoveryStableFor <= 0 {
		timing.RecoveryStableFor = DefaultTiming.RecoveryStableFor
	}
	if timing.UnknownGrace <= 0 {
		timing.UnknownGrace = DefaultTiming.UnknownGrace
	}
	return &Tracker{store: store, timing: timing}
}

// Restore re-attaches to one open event after a process restart. A later
// observation closes it if its local date or current mode makes it invalid.
func (t *Tracker) Restore(ctx context.Context) error {
	if t == nil || t.store == nil {
		return nil
	}
	event, err := t.store.OpenDistractionEvent(ctx)
	if err != nil {
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	}
	t.mu.Lock()
	t.current = &event
	t.mu.Unlock()
	return nil
}

func (t *Tracker) Observe(ctx context.Context, input Input) error {
	if t == nil {
		return errors.New("distraction tracker is nil")
	}
	if input.Now.IsZero() {
		return errors.New("distraction observation time is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if input.UserMode != state.UserModeStudy || !input.ActivityWatchOK || !input.ActivityFresh || input.Privacy != state.PrivacyNormal || input.Locked {
		t.pendingSince = time.Time{}
		t.unknownSince = time.Time{}
		t.recoverySince = time.Time{}
		return t.closeLocked(ctx, input.Now, closeReason(input))
	}

	if t.current != nil && storage.LocalDate(t.current.StartedAt) != storage.LocalDate(input.Now) {
		if err := t.closeLocked(ctx, input.Now, "MIDNIGHT"); err != nil {
			return err
		}
	}

	switch input.Relation {
	case state.RelationDistracted:
		t.unknownSince = time.Time{}
		t.recoverySince = time.Time{}
		if t.current == nil {
			if t.pendingSince.IsZero() {
				t.pendingSince = input.Now
				return nil
			}
			if input.Now.Sub(t.pendingSince) < t.timing.DistractedStableFor {
				return nil
			}
			event := storage.DistractionEventRecord{
				ID:            fmt.Sprintf("distraction-%d-%d", input.Now.UnixNano(), sequence.Add(1)),
				StartedAt:     t.pendingSince,
				LocalDate:     storage.LocalDate(t.pendingSince),
				App:           bounded(input.App),
				Title:         boundedTitle(input.Title),
				Domain:        boundedDomain(input.Domain),
				Task:          bounded(input.Task),
				ReminderLevel: input.ReminderLevel,
				Source:        input.Source,
				Confidence:    clampConfidence(input.Confidence),
			}
			if t.store != nil {
				if err := t.store.CreateDistractionEvent(ctx, event); err != nil {
					return err
				}
			}
			t.current = &event
		}
		return t.updateOpenLocked(ctx, input)
	case state.RelationFocused:
		t.pendingSince = time.Time{}
		t.unknownSince = time.Time{}
		if t.current == nil {
			return nil
		}
		if t.recoverySince.IsZero() {
			t.recoverySince = input.Now
			return nil
		}
		if input.Now.Sub(t.recoverySince) >= t.timing.RecoveryStableFor {
			return t.closeLocked(ctx, input.Now, "FOCUS_RECOVERED")
		}
		return t.updateOpenLocked(ctx, input)
	default:
		t.pendingSince = time.Time{}
		t.recoverySince = time.Time{}
		if t.current == nil {
			return nil
		}
		if t.unknownSince.IsZero() {
			t.unknownSince = input.Now
			return nil
		}
		if input.Now.Sub(t.unknownSince) >= t.timing.UnknownGrace {
			return t.closeLocked(ctx, input.Now, "UNKNOWN_GRACE_EXPIRED")
		}
		return t.updateOpenLocked(ctx, input)
	}
}

func (t *Tracker) Close(ctx context.Context, now time.Time, reason string) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeLocked(ctx, now, reason)
}

func (t *Tracker) updateOpenLocked(ctx context.Context, input Input) error {
	if t.current == nil {
		return nil
	}
	event := *t.current
	event.DurationSeconds = maxSeconds(input.Now.Sub(event.StartedAt))
	if input.ReminderLevel != "" {
		event.ReminderLevel = input.ReminderLevel
	}
	if input.Source != "" {
		event.Source = input.Source
	}
	if input.Confidence > event.Confidence {
		event.Confidence = clampConfidence(input.Confidence)
	}
	if t.store != nil {
		if err := t.store.UpdateDistractionEvent(ctx, event); err != nil {
			return err
		}
	}
	t.current = &event
	return nil
}

func (t *Tracker) closeLocked(ctx context.Context, now time.Time, reason string) error {
	if t.current == nil {
		return nil
	}
	event := *t.current
	event.EndedAt = &now
	event.DurationSeconds = maxSeconds(now.Sub(event.StartedAt))
	event.EndReason = reason
	if t.store != nil {
		if err := t.store.UpdateDistractionEvent(ctx, event); err != nil {
			return err
		}
		if _, err := t.store.BumpEvidenceRevision(ctx, event.LocalDate, now); err != nil {
			return err
		}
	}
	t.current = nil
	return nil
}

func closeReason(input Input) string {
	switch {
	case input.Locked:
		return "LOCKED"
	case input.UserMode == state.UserModeBreak:
		return "BREAK"
	case input.UserMode == state.UserModeOff:
		return "OFF"
	case input.Privacy != state.PrivacyNormal:
		return "PRIVACY"
	case !input.ActivityWatchOK || !input.ActivityFresh:
		return "ACTIVITYWATCH_UNAVAILABLE"
	default:
		return "STUDY_ENDED"
	}
}

func bounded(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 120 {
		return string([]rune(value)[:120]) + "…"
	}
	return value
}

func boundedTitle(value string) string {
	return bounded(value)
}

func boundedDomain(value string) string {
	value = strings.Split(value, "?")[0]
	value = strings.Split(value, "#")[0]
	return bounded(value)
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func maxSeconds(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value.Seconds())
}
