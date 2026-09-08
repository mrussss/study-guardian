package state

import "time"

type UserMode string

const (
	UserModeStandby UserMode = "STANDBY"
	UserModeStudy   UserMode = "STUDY"
	UserModeBreak   UserMode = "BREAK"
	UserModeOff     UserMode = "OFF"
)

type ModeOrigin string

const (
	ModeOriginManual     ModeOrigin = "MANUAL"
	ModeOriginAutomation ModeOrigin = "AUTOMATION"
)

type PauseReason string

const (
	PauseReasonNone              PauseReason = "NONE"
	PauseReasonIdle              PauseReason = "IDLE"
	PauseReasonLocked            PauseReason = "LOCKED"
	PauseReasonSleep             PauseReason = "SLEEP"
	PauseReasonSensorUnavailable PauseReason = "SENSOR_UNAVAILABLE"
)

type AutomationTransition string

const (
	AutomationStart  AutomationTransition = "AUTO_START"
	AutomationPause  AutomationTransition = "AUTO_PAUSE"
	AutomationResume AutomationTransition = "AUTO_RESUME"
)

type AutomationIntent struct {
	Transition AutomationTransition
	Task       string
	Reason     PauseReason
}

type InteractionState string

const (
	InteractionActive      InteractionState = "ACTIVE"
	InteractionIdleStatic  InteractionState = "IDLE_STATIC"
	InteractionIdleDynamic InteractionState = "IDLE_DYNAMIC"
	InteractionUnknown     InteractionState = "UNKNOWN"
)

type TaskRelation string

const (
	RelationFocused    TaskRelation = "FOCUSED"
	RelationDistracted TaskRelation = "DISTRACTED"
	RelationUnknown    TaskRelation = "UNKNOWN"
)

type PrivacyState string

const (
	PrivacyNormal    PrivacyState = "NORMAL"
	PrivacySensitive PrivacyState = "SENSITIVE"
)

// The semantic vocabulary is shared by local rules, text AI and vision AI.
// Keeping these values in the state package prevents each consumer from
// inventing a subtly different interpretation of one observation.
const (
	SourceKindLocalRule = "LOCAL_RULE"
	SourceKindTextAI    = "TEXT_AI"
	SourceKindVisionAI  = "VISION_AI"

	ProgressObserving  = "OBSERVING"
	ProgressReading    = "READING"
	ProgressPracticing = "PRACTICING"
	ProgressCoding     = "CODING"
	ProgressWriting    = "WRITING"
	ProgressDebugging  = "DEBUGGING"
	ProgressReviewing  = "REVIEWING"
	ProgressUnknown    = "UNKNOWN"
)

type Observation struct {
	Interaction InteractionState `json:"interaction"`
	Relation    TaskRelation     `json:"relation"`
	Privacy     PrivacyState     `json:"privacy"`
	Confidence  float64          `json:"confidence"`
	Reason      string           `json:"reason,omitempty"`
	Timestamp   time.Time        `json:"timestamp"`
}

type ReminderLevel string

const (
	ReminderLevelNone   ReminderLevel = "NONE"
	ReminderLevelPet    ReminderLevel = "PET"
	ReminderLevelBubble ReminderLevel = "BUBBLE"
	ReminderLevelToast  ReminderLevel = "TOAST"
)

type ReminderEvent struct {
	ID        string        `json:"id"`
	Level     ReminderLevel `json:"level"`
	Message   string        `json:"message"`
	Reason    string        `json:"reason"`
	CreatedAt time.Time     `json:"created_at"`
}

type FeedbackRecord struct {
	EventID   string    `json:"event_id"`
	Feedback  string    `json:"feedback"`
	CreatedAt time.Time `json:"created_at"`
}

type ClassificationResult struct {
	Relation       TaskRelation `json:"relation"`
	Activity       string       `json:"activity,omitempty"`
	Topic          string       `json:"topic,omitempty"`
	Subtopic       string       `json:"subtopic,omitempty"`
	Action         string       `json:"action,omitempty"`
	ProgressSignal string       `json:"progress_signal,omitempty"`
	Confidence     float64      `json:"confidence"`
	Reason         string       `json:"reason"`
	SourceKind     string       `json:"source_kind,omitempty"`
	IsFromRule     bool         `json:"is_from_rule"`
}

type ReminderDecisionInput struct {
	Now               time.Time
	UserMode          UserMode
	Task              string
	Interaction       InteractionState
	Relation          TaskRelation
	Privacy           PrivacyState
	Confidence        float64
	ActiveSeconds     int64
	StudySeconds      int64
	BreakSeconds      int64
	DistractedSeconds int64
	IdleStaticSeconds int64
}

// TickOutcome is the single authoritative time delta produced by Manager.
// Downstream product features must consume this result instead of creating a
// second wall-clock timer.
type TickOutcome struct {
	Now               time.Time
	DeltaSeconds      int64
	UserMode          UserMode
	Interaction       InteractionState
	Relation          TaskRelation
	ActivityValid     bool
	Locked            bool
	IdleStaticSeconds int64
	Classification    ClassificationResult
}

type SystemStatus struct {
	UserMode            UserMode         `json:"user_mode"`
	InteractionState    InteractionState `json:"interaction_state"`
	TaskRelation        TaskRelation     `json:"task_relation"`
	PrivacyState        PrivacyState     `json:"privacy_state"`
	Confidence          float64          `json:"confidence"`
	Task                string           `json:"task"`
	StudySeconds        int64            `json:"study_seconds"`
	BreakSeconds        int64            `json:"break_seconds"`
	ActiveSeconds       int64            `json:"active_seconds"`
	LastActivityAt      *time.Time       `json:"last_activity_at,omitempty"`
	ActivityWatchOK     bool             `json:"activitywatch_ok"`
	ScreenSensorOK      bool             `json:"screen_sensor_ok"`
	CurrentReminder     *ReminderEvent   `json:"current_reminder,omitempty"`
	ModeOrigin          ModeOrigin       `json:"mode_origin"`
	PauseReason         PauseReason      `json:"pause_reason"`
	AutoResumeEligible  bool             `json:"auto_resume_eligible"`
	ManualOverrideUntil *time.Time       `json:"manual_override_until,omitempty"`
}
