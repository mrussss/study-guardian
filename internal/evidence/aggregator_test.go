package evidence

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/storage"
)

func TestAggregatorKeepsOnlyEligibleChatTurns(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := store.UpdateDailyState(context.Background(), "2026-09-03", 0, 3600, 0, 0, 3000, now); err != nil {
		t.Fatal(err)
	}
	for _, item := range []storage.ChatTurnRecord{
		{TurnKey: "study", ObservedAt: now, LocalDate: "2026-09-03", ModeAtStart: "STUDY", TaskAtStart: "Go", EligibleForReview: true},
		{TurnKey: "break", ObservedAt: now, LocalDate: "2026-09-03", ModeAtStart: "BREAK", TaskAtStart: "Go", EligibleForReview: false},
	} {
		if _, err := store.IngestChatTurn(context.Background(), storage.ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "c1", ObservedAt: now}, item, nil, now); err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := NewAggregator(store, time.UTC).Build(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.ChatTurns) != 1 || bundle.ChatTurns[0].TurnKey != "study" {
		t.Fatalf("chat turns=%+v", bundle.ChatTurns)
	}
	if !bundle.Quality.StudyStatePresent || !bundle.Quality.HasEligibleChat {
		t.Fatalf("quality=%+v", bundle.Quality)
	}
}
