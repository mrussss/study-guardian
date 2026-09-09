package state

import (
	"strings"
	"time"
)

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

type AutomationExpiryAction string

const (
	AutomationExpiryDismiss AutomationExpiryAction = "DISMISS"
	AutomationExpiryApply   AutomationExpiryAction = "APPLY"
)

type AutomationIntent struct {
	ID                   string                 `json:"intent_id"`
	Transition           AutomationTransition   `json:"transition"`
	Task                 string                 `json:"task,omitempty"`
	Reason               PauseReason            `json:"reason"`
	CreatedAt            time.Time              `json:"created_at"`
	ExpiresAt            time.Time              `json:"expires_at"`
	ExpiryAction         AutomationExpiryAction `json:"expiry_action,omitempty"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
}

type ActivityWatchHealthPhase string

const (
	ActivityWatchAvailable   ActivityWatchHealthPhase = "AVAILABLE"
	ActivityWatchDegraded    ActivityWatchHealthPhase = "DEGRADED"
	ActivityWatchUnavailable ActivityWatchHealthPhase = "UNAVAILABLE"
)

type ActivityWatchDiagnostics struct {
	LastSuccessAt       *time.Time
	ConsecutiveFailures int
	StableOK            bool
	Phase               ActivityWatchHealthPhase
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

	ActivityCoding       = "CODING"
	ActivityAlgorithm    = "ALGORITHM"
	ActivityReading      = "READING"
	ActivityWriting      = "WRITING"
	ActivityWatching     = "WATCHING"
	ActivityAIAssisted   = "AI_ASSISTED"
	ActivityBrowsing     = "BROWSING"
	ActivityMessaging    = "MESSAGING"
	ActivityGaming       = "GAMING"
	ActivityGeneralStudy = "GENERAL_STUDY"
	ActivityOther        = "OTHER"
	ActivityUnknown      = "UNKNOWN"

	ProgressObserving  = "OBSERVING"
	ProgressReading    = "READING"
	ProgressPracticing = "PRACTICING"
	ProgressCoding     = "CODING"
	ProgressWriting    = "WRITING"
	ProgressDebugging  = "DEBUGGING"
	ProgressReviewing  = "REVIEWING"
	ProgressUnknown    = "UNKNOWN"
)

// NormalizeActivity is the single boundary for provider and local-rule
// activity values. Free-form provider text is reduced to a bounded enum
// before it can enter cache, semantic evidence, or the UI.
func NormalizeActivity(raw string) (string, bool) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "_", " ", "_", "/", "_").Replace(value)
	switch value {
	case ActivityCoding, "PROGRAMMING", "DEVELOPMENT":
		return ActivityCoding, true
	case ActivityAlgorithm, "PROBLEM_SOLVING", "PROBLEM_SOLVING_ALGORITHM":
		return ActivityAlgorithm, true
	case ActivityReading, "READ", "READING_DOCUMENTATION":
		return ActivityReading, true
	case ActivityWriting, "WRITE", "DOCUMENTATION_WRITING":
		return ActivityWriting, true
	case ActivityWatching, "VIDEO", "WATCHING_VIDEO":
		return ActivityWatching, true
	case ActivityAIAssisted, "AI", "AI_ASSIST":
		return ActivityAIAssisted, true
	case ActivityBrowsing, "BROWSER", "WEB_BROWSING":
		return ActivityBrowsing, true
	case ActivityMessaging, "MESSAGING_CHAT", "CHAT", "SOCIAL_CHAT":
		return ActivityMessaging, true
	case ActivityGaming, "GAME", "ENTERTAINMENT", "PLAYING":
		return ActivityGaming, true
	case ActivityGeneralStudy, "STUDY", "LEARNING", "STUDYING":
		return ActivityGeneralStudy, true
	case ActivityUnknown, "UNCLASSIFIED", "UNKNOWABLE":
		return ActivityUnknown, true
	case ActivityOther, "MISC", "MISCELLANEOUS":
		return ActivityOther, true
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(lower, "brows"), strings.Contains(lower, "浏览"):
		return ActivityBrowsing, false
	case strings.Contains(lower, "wechat"), strings.Contains(lower, "微信"), strings.Contains(lower, "messag"), strings.Contains(lower, "chat"):
		return ActivityMessaging, false
	case strings.Contains(lower, "steam"), strings.Contains(lower, "game"), strings.Contains(lower, "gaming"), strings.Contains(lower, "游戏"):
		return ActivityGaming, false
	case strings.Contains(lower, "code"), strings.Contains(lower, "编程"):
		return ActivityCoding, false
	case strings.Contains(lower, "read"), strings.Contains(lower, "阅读"):
		return ActivityReading, false
	case strings.Contains(lower, "writ"), strings.Contains(lower, "写作"):
		return ActivityWriting, false
	}
	return ActivityOther, false
}

func IsExplicitStudyActivity(activity string) bool {
	normalized, _ := NormalizeActivity(activity)
	switch normalized {
	case ActivityCoding, ActivityAlgorithm, ActivityReading, ActivityWriting, ActivityAIAssisted, ActivityGeneralStudy:
		return true
	default:
		return false
	}
}

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
	ID                   string        `json:"id"`
	Level                ReminderLevel `json:"level"`
	Message              string        `json:"message"`
	Reason               string        `json:"reason"`
	CreatedAt            time.Time     `json:"created_at"`
	ExpiresAt            time.Time     `json:"expires_at,omitempty"`
	RelatedDistractionID string        `json:"related_distraction_id,omitempty"`
	Active               bool          `json:"active"`
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
	AfkSeconds        int64
	AfkSince          *time.Time
	Classification    ClassificationResult
}

type SystemStatus struct {
	UserMode                         UserMode                 `json:"user_mode"`
	InteractionState                 InteractionState         `json:"interaction_state"`
	TaskRelation                     TaskRelation             `json:"task_relation"`
	PrivacyState                     PrivacyState             `json:"privacy_state"`
	Confidence                       float64                  `json:"confidence"`
	Task                             string                   `json:"task"`
	StudySeconds                     int64                    `json:"study_seconds"`
	BreakSeconds                     int64                    `json:"break_seconds"`
	ActiveSeconds                    int64                    `json:"active_seconds"`
	LastActivityAt                   *time.Time               `json:"last_activity_at,omitempty"`
	AfkSeconds                       int64                    `json:"afk_seconds"`
	AfkSince                         *time.Time               `json:"afk_since,omitempty"`
	ActivityWatchOK                  bool                     `json:"activitywatch_ok"`
	ActivityWatchLastSuccessAt       *time.Time               `json:"activitywatch_last_success_at,omitempty"`
	ActivityWatchConsecutiveFailures int                      `json:"activitywatch_consecutive_failures"`
	ActivityWatchStableOK            bool                     `json:"activitywatch_stable_ok"`
	ActivityWatchHealthPhase         ActivityWatchHealthPhase `json:"activitywatch_health_phase"`
	ScreenSensorOK                   bool                     `json:"screen_sensor_ok"`
	CurrentReminder                  *ReminderEvent           `json:"current_reminder,omitempty"`
	ModeOrigin                       ModeOrigin               `json:"mode_origin"`
	PauseReason                      PauseReason              `json:"pause_reason"`
	AutoResumeEligible               bool                     `json:"auto_resume_eligible"`
	ManualOverrideUntil              *time.Time               `json:"manual_override_until,omitempty"`
	AutoPauseSnoozeUntil             *time.Time               `json:"auto_pause_snooze_until,omitempty"`
	PendingAutomationIntent          *AutomationIntent        `json:"pending_automation_intent,omitempty"`
}
