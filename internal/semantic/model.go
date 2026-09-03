package semantic

import (
	"time"

	"study-guardian/internal/state"
)

const SchemaVersion = 1

type Activity string

const (
	ActivityCoding       Activity = "CODING"
	ActivityAlgorithm    Activity = "ALGORITHM"
	ActivityReading      Activity = "READING"
	ActivityWriting      Activity = "WRITING"
	ActivityWatching     Activity = "WATCHING"
	ActivityAIAssisted   Activity = "AI_ASSISTED"
	ActivityBrowsing     Activity = "BROWSING"
	ActivityGeneralStudy Activity = "GENERAL_STUDY"
	ActivityUnknown      Activity = "UNKNOWN"
)

// Timing makes the throttle policy explicit and allows deterministic tests to
// use a shorter FakeClock window without changing production behavior.
type Timing struct {
	TransitionStableFor time.Duration
	MinPersistInterval  time.Duration
	HeartbeatInterval   time.Duration
	LiveMaxAge          time.Duration
}

var DefaultTiming = Timing{
	TransitionStableFor: 6 * time.Second,
	MinPersistInterval:  15 * time.Second,
	HeartbeatInterval:   180 * time.Second,
	LiveMaxAge:          10 * time.Second,
}

// Candidate is built from the existing ActivityWatch snapshot and the
// TickOutcome/state values already produced by Supervisor. It intentionally
// contains no screenshot or AI-specific input.
type Candidate struct {
	ObservedAt  time.Time
	Fresh       bool
	UserMode    state.UserMode
	Task        string
	Interaction state.InteractionState
	Relation    state.TaskRelation
	Privacy     state.PrivacyState
	App         string
	Title       string
	Domain      string
}

type CurrentActivityView struct {
	SchemaVersion int                    `json:"schema_version"`
	ObservedAt    time.Time              `json:"observed_at"`
	Fresh         bool                   `json:"fresh"`
	UserMode      state.UserMode         `json:"user_mode"`
	Task          string                 `json:"task"`
	Interaction   state.InteractionState `json:"interaction"`
	Relation      state.TaskRelation     `json:"relation"`
	Privacy       state.PrivacyState     `json:"privacy"`
	Activity      Activity               `json:"activity"`
	Confidence    float64                `json:"confidence"`
}

func emptyView() CurrentActivityView {
	return CurrentActivityView{
		SchemaVersion: SchemaVersion,
		Fresh:         false,
		UserMode:      state.UserModeStandby,
		Interaction:   state.InteractionUnknown,
		Relation:      state.RelationUnknown,
		Privacy:       state.PrivacyNormal,
		Activity:      ActivityUnknown,
	}
}
