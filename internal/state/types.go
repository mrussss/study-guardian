package state

import "time"

type UserMode string

const (
	UserModeStandby UserMode = "STANDBY"
	UserModeStudy   UserMode = "STUDY"
	UserModeBreak   UserMode = "BREAK"
	UserModeOff     UserMode = "OFF"
)

type InteractionState string

const (
	InteractionActive      InteractionState = "ACTIVE"
	InteractionIdleStatic  InteractionState = "IDLE_STATIC"
	InteractionIdleDynamic InteractionState = "IDLE_DYNAMIC"
	InteractionUnknown      InteractionState = "UNKNOWN"
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
	Relation   TaskRelation `json:"relation"`
	Confidence float64      `json:"confidence"`
	Reason     string       `json:"reason"`
	IsFromRule bool         `json:"is_from_rule"`
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

type SystemStatus struct {
	UserMode         UserMode         `json:"user_mode"`
	InteractionState InteractionState `json:"interaction_state"`
	TaskRelation     TaskRelation     `json:"task_relation"`
	PrivacyState     PrivacyState     `json:"privacy_state"`
	Confidence       float64          `json:"confidence"`
	Task             string           `json:"task"`
	StudySeconds     int64            `json:"study_seconds"`
	BreakSeconds     int64            `json:"break_seconds"`
	ActiveSeconds    int64            `json:"active_seconds"`
	LastActivityAt   *time.Time       `json:"last_activity_at,omitempty"`
	ActivityWatchOK  bool             `json:"activitywatch_ok"`
	ScreenSensorOK   bool             `json:"screen_sensor_ok"`
	CurrentReminder  *ReminderEvent   `json:"current_reminder,omitempty"`
}
