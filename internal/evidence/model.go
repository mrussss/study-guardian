package evidence

import "time"

type DailyEvidenceBundle struct {
	Date         string               `json:"date"`
	Timezone     string               `json:"timezone"`
	DailyState   DailyStateSummary    `json:"daily_state"`
	Sessions     []SessionSummary     `json:"sessions"`
	Distractions []DistractionSummary `json:"distractions"`
	Reminders    []ReminderSummary    `json:"reminders"`
	Motivation   MotivationSummary    `json:"motivation"`
	ChatTurns    []ChatTurnSummary    `json:"chat_turns"`
	Semantic     []SemanticSummary    `json:"semantic"`
	Quality      EvidenceQuality      `json:"quality"`
	Warnings     []string             `json:"warnings"`
}

type DailyStateSummary struct {
	StandbySeconds int64 `json:"standby_seconds"`
	StudySeconds   int64 `json:"study_seconds"`
	BreakSeconds   int64 `json:"break_seconds"`
	OffSeconds     int64 `json:"off_seconds"`
	ActiveSeconds  int64 `json:"active_seconds"`
}

type SessionSummary struct {
	Ref             string     `json:"ref"`
	ID              string     `json:"id"`
	Mode            string     `json:"mode"`
	Task            string     `json:"task"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds"`
}

type DistractionSummary struct {
	Ref             string `json:"ref"`
	ID              string `json:"id"`
	DurationSeconds int64  `json:"duration_seconds"`
	App             string `json:"app"`
	Title           string `json:"title"`
	Domain          string `json:"domain"`
	Task            string `json:"task"`
}

type ReminderSummary struct {
	Ref       string    `json:"ref"`
	ID        string    `json:"id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type MotivationSummary struct {
	CreditedFocusSeconds int64 `json:"credited_focus_seconds"`
	DailyTargetSeconds   int64 `json:"daily_target_seconds"`
	CheckinCompleted     bool  `json:"checkin_completed"`
	TargetCompleted      bool  `json:"target_completed"`
}

type ChatTurnSummary struct {
	Ref               string    `json:"ref"`
	ID                int64     `json:"id"`
	TurnKey           string    `json:"turn_key"`
	ObservedAt        time.Time `json:"observed_at"`
	TaskAtStart       string    `json:"task_at_start"`
	EligibleForReview bool      `json:"eligible_for_review"`
	Finalized         bool      `json:"finalized"`
	UserContent       string    `json:"user_content"`
	AssistantContent  string    `json:"assistant_content"`
}

type SemanticSummary struct {
	Ref        string    `json:"ref"`
	ObservedAt time.Time `json:"observed_at"`
	Task       string    `json:"task"`
	App        string    `json:"app"`
	Title      string    `json:"title"`
	Domain     string    `json:"domain"`
	Relation   string    `json:"relation"`
	Confidence float64   `json:"confidence"`
	Activity   string    `json:"activity"`
	SourceKind string    `json:"source_kind"`
}

type EvidenceQuality struct {
	Score             float64 `json:"score"`
	StudyStatePresent bool    `json:"study_state_present"`
	HasEligibleChat   bool    `json:"has_eligible_chat"`
	HasSemantic       bool    `json:"has_semantic"`
	HasAccomplishment bool    `json:"has_accomplishment"`
}
