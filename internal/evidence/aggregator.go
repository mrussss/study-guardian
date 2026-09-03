package evidence

import (
	"context"
	"time"

	"study-guardian/internal/storage"
)

type Aggregator struct {
	store    *storage.Storage
	timezone *time.Location
}

func NewAggregator(store *storage.Storage, timezone *time.Location) *Aggregator {
	if timezone == nil {
		timezone = time.Local
	}
	return &Aggregator{store: store, timezone: timezone}
}

func (a *Aggregator) Build(ctx context.Context, date string) (DailyEvidenceBundle, error) {
	bundle := DailyEvidenceBundle{Date: date, Timezone: a.timezone.String()}
	statePresent := false
	standby, study, breakSeconds, off, active, err := a.store.LoadDailyState(ctx, date)
	if err == nil {
		statePresent = true
		bundle.DailyState = DailyStateSummary{StandbySeconds: standby, StudySeconds: study, BreakSeconds: breakSeconds, OffSeconds: off, ActiveSeconds: active}
	} else if !storage.IsNotFound(err) {
		return DailyEvidenceBundle{}, err
	} else {
		bundle.Warnings = append(bundle.Warnings, "daily_state unavailable")
	}

	sessions, err := a.store.ListSessionsForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	for _, item := range sessions {
		bundle.Sessions = append(bundle.Sessions, SessionSummary{Ref: "session:" + item.ID, ID: item.ID, Mode: item.Mode, Task: item.Task, StartedAt: item.StartedAt, EndedAt: item.EndedAt, DurationSeconds: item.DurationSeconds})
	}
	distractions, err := a.store.ListDistractionsForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	for _, item := range distractions {
		bundle.Distractions = append(bundle.Distractions, DistractionSummary{Ref: "distraction:" + item.ID, ID: item.ID, DurationSeconds: item.DurationSeconds, App: item.App, Title: item.Title, Domain: item.Domain, Task: item.Task})
	}
	reminders, err := a.store.ListRemindersForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	for _, item := range reminders {
		bundle.Reminders = append(bundle.Reminders, ReminderSummary{Ref: "reminder:" + item.ID, ID: item.ID, Level: item.Level, Message: item.Message, CreatedAt: item.CreatedAt})
	}
	daily, err := a.store.GetMotivationDaily(ctx, date)
	if err == nil {
		bundle.Motivation = MotivationSummary{CreditedFocusSeconds: daily.CreditedFocusSeconds, DailyTargetSeconds: daily.DailyTargetSeconds, CheckinCompleted: daily.CheckinCompleted, TargetCompleted: daily.TargetCompleted}
	} else if !storage.IsNotFound(err) {
		return DailyEvidenceBundle{}, err
	}
	exclusions, err := a.store.ListReviewExclusionsForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	excludedTurns := map[string]struct{}{}
	excludedConversations := map[string]struct{}{}
	for _, exclusion := range exclusions {
		switch exclusion.SourceType {
		case "chat_turn":
			excludedTurns[exclusion.SourceID] = struct{}{}
		case "chat_conversation":
			excludedConversations[exclusion.SourceID] = struct{}{}
		}
	}
	chatTurns, err := a.store.ListChatTurnsForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	for _, item := range chatTurns {
		if !item.EligibleForReview || item.CapturePolicy == "ALWAYS_EXCLUDE" {
			continue
		}
		if _, excluded := excludedTurns[item.TurnKey]; excluded {
			continue
		}
		if _, excluded := excludedConversations[item.ExternalConversationID]; excluded {
			continue
		}
		bundle.ChatTurns = append(bundle.ChatTurns, ChatTurnSummary{Ref: "chat_turn:" + item.TurnKey, ID: item.ID, TurnKey: item.TurnKey, ExternalConversationID: item.ExternalConversationID, ConversationTitle: item.ConversationTitle, ObservedAt: item.ObservedAt, TaskAtStart: item.TaskAtStart, EligibleForReview: true, Finalized: item.Finalized, UserContent: item.UserContent, AssistantContent: item.AssistantContent})
	}
	semantic, err := a.store.ListSemanticSnapshotsForDate(ctx, date)
	if err != nil {
		return DailyEvidenceBundle{}, err
	}
	for _, item := range semantic {
		bundle.Semantic = append(bundle.Semantic, SemanticSummary{ID: item.ID, Ref: "semantic:" + itoa64(item.ID), ObservedAt: item.ObservedAt, Task: item.Task, App: item.App, Title: item.Title, Domain: item.Domain, Relation: item.Relation, Confidence: item.Confidence, Activity: item.Activity, SourceKind: item.SourceKind})
	}
	if len(exclusions) > 0 {
		bundle.Warnings = append(bundle.Warnings, "review exclusions applied")
	}
	bundle.Quality = EvidenceQuality{
		Score:             qualityScore(bundle),
		StudyStatePresent: statePresent,
		HasEligibleChat:   len(bundle.ChatTurns) > 0,
		HasSemantic:       len(bundle.Semantic) > 0,
		HasAccomplishment: false,
	}
	return bundle, nil
}

func qualityScore(bundle DailyEvidenceBundle) float64 {
	score := 0.0
	if bundle.DailyState.StudySeconds > 0 {
		score += .35
	}
	if len(bundle.Sessions) > 0 {
		score += .2
	}
	if len(bundle.ChatTurns) > 0 {
		score += .25
	}
	if len(bundle.Semantic) > 0 {
		score += .2
	}
	return score
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	return string(buf)
}

func itoa64(value int64) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	return string(buf)
}
