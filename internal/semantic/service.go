package semantic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type Service struct {
	mu            sync.RWMutex
	store         *storage.Storage
	timing        Timing
	view          CurrentActivityView
	lastKey       string
	pendingKey    string
	pendingSince  time.Time
	lastPersistAt time.Time
}

func NewService(store *storage.Storage) *Service {
	return NewServiceWithTiming(store, DefaultTiming)
}

func NewServiceWithTiming(store *storage.Storage, timing Timing) *Service {
	if timing.TransitionStableFor <= 0 {
		timing.TransitionStableFor = DefaultTiming.TransitionStableFor
	}
	if timing.MinPersistInterval <= 0 {
		timing.MinPersistInterval = DefaultTiming.MinPersistInterval
	}
	if timing.HeartbeatInterval <= 0 {
		timing.HeartbeatInterval = DefaultTiming.HeartbeatInterval
	}
	if timing.LiveMaxAge <= 0 {
		timing.LiveMaxAge = DefaultTiming.LiveMaxAge
	}
	return &Service{store: store, timing: timing, view: emptyView()}
}

// Observe classifies one already-collected observation and updates the live
// contract. It does not query ActivityWatch, capture a screen, or call an AI
// provider.
func (s *Service) Observe(ctx context.Context, candidate Candidate) error {
	if s == nil {
		return errors.New("semantic service is nil")
	}
	activity, confidence, _ := Classify(candidate)
	if !candidate.Fresh || candidate.Privacy == state.PrivacySensitive {
		activity = ActivityUnknown
		confidence = 0
	}
	// Sensitive windows are deliberately indistinguishable from unavailable
	// semantic activity to consumers: the privacy bit remains visible, while
	// fresh is false and no content can enter persistence.
	if candidate.Privacy == state.PrivacySensitive {
		candidate.Fresh = false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.view = CurrentActivityView{
		SchemaVersion: SchemaVersion,
		ObservedAt:    candidate.ObservedAt,
		Fresh:         candidate.Fresh,
		UserMode:      candidate.UserMode,
		Task:          candidate.Task,
		Interaction:   candidate.Interaction,
		Relation:      candidate.Relation,
		Privacy:       candidate.Privacy,
		Activity:      activity,
		Confidence:    confidence,
	}

	if s.store == nil {
		s.resetPendingLocked()
		return nil
	}
	return s.persistIfEligibleLocked(ctx, candidate, activity, confidence)
}

// Current applies age-based freshness at read time. A stale or future event is
// never presented as a live semantic activity.
func (s *Service) Current(now time.Time) CurrentActivityView {
	if s == nil {
		return emptyView()
	}
	s.mu.RLock()
	view := s.view
	timing := s.timing
	s.mu.RUnlock()
	if view.SchemaVersion == 0 {
		view = emptyView()
	}
	if !view.Fresh || view.ObservedAt.IsZero() || now.Before(view.ObservedAt) || now.Sub(view.ObservedAt) > timing.LiveMaxAge {
		view.Fresh = false
		view.Activity = ActivityUnknown
		view.Confidence = 0
	}
	return view
}

func (s *Service) persistIfEligibleLocked(ctx context.Context, c Candidate, activity Activity, confidence float64) error {
	if c.UserMode != state.UserModeStudy || !c.Fresh || c.Privacy != state.PrivacyNormal || activity == ActivityUnknown || c.ObservedAt.IsZero() {
		s.resetPendingLocked()
		return nil
	}
	key := semanticKey(c, activity)
	if key != s.pendingKey {
		s.pendingKey = key
		s.pendingSince = c.ObservedAt
		return nil
	}
	if c.ObservedAt.Before(s.pendingSince) {
		s.pendingSince = c.ObservedAt
		return nil
	}
	if c.ObservedAt.Sub(s.pendingSince) < s.timing.TransitionStableFor {
		return nil
	}
	if s.lastPersistAt.IsZero() {
		// The first stable observation is the initial semantic snapshot.
	} else if s.lastKey == key {
		// A stable key is not written every MinPersistInterval. It receives a
		// sparse heartbeat so a long study block remains represented without
		// turning the snapshot table into a per-tick log.
		if c.ObservedAt.Sub(s.lastPersistAt) < s.timing.HeartbeatInterval {
			return nil
		}
	} else if c.ObservedAt.Sub(s.lastPersistAt) < s.timing.MinPersistInterval {
		return nil
	}

	_, err := s.store.RecordSemanticSnapshot(ctx, storage.SemanticSnapshotRecord{
		// SQLite's timestamp adapter is most portable with UTC values. The
		// original instant is preserved; LocalDate is computed from the
		// observation's local zone below rather than from insertion time.
		ObservedAt:   c.ObservedAt.UTC(),
		LocalDate:    c.ObservedAt.In(time.Local).Format("2006-01-02"),
		Task:         c.Task,
		App:          c.App,
		Title:        c.Title,
		Domain:       c.Domain,
		Relation:     string(c.Relation),
		Confidence:   confidence,
		Activity:     string(activity),
		Reason:       "deterministic local semantic rule",
		SourceKind:   "LOCAL_RULE",
		MetadataJSON: "{}",
	})
	if err != nil {
		return err
	}
	s.lastKey = key
	s.lastPersistAt = c.ObservedAt
	return nil
}

func (s *Service) resetPendingLocked() {
	s.pendingKey = ""
	s.pendingSince = time.Time{}
}

func semanticKey(c Candidate, activity Activity) string {
	return strings.Join([]string{
		normalize(c.Task),
		string(c.Relation),
		string(activity),
		string(c.Interaction),
		normalize(c.App),
		normalize(c.Domain),
	}, "|")
}
