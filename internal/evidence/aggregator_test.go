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

func TestAggregatorAppliesTurnConversationAndGlobalExclusions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	newStore := func(t *testing.T) *storage.Storage {
		store, err := storage.OpenSQLite(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ conversation, turn string }{{"A", "A1"}, {"A", "A2"}, {"B", "B1"}} {
			if _, err := store.IngestChatTurn(ctx, storage.ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: item.conversation, Title: item.conversation, ObservedAt: now}, storage.ChatTurnRecord{TurnKey: item.turn, ObservedAt: now, LocalDate: "2026-09-03", ModeAtStart: "STUDY", EligibleForReview: true}, nil, now); err != nil {
				t.Fatal(err)
			}
		}
		return store
	}
	t.Run("turn exclusion", func(t *testing.T) {
		store := newStore(t)
		defer store.Close()
		if err := store.AddReviewExclusion(ctx, storage.ReviewExclusionRecord{Date: "2026-09-03", SourceType: "chat_turn", SourceID: "A1", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		bundle, err := NewAggregator(store, time.UTC).Build(ctx, "2026-09-03")
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.ChatTurns) != 2 || bundle.ChatTurns[0].TurnKey != "A2" {
			t.Fatalf("turn exclusion bundle=%+v", bundle.ChatTurns)
		}
	})
	t.Run("conversation exclusion", func(t *testing.T) {
		store := newStore(t)
		defer store.Close()
		if err := store.AddReviewExclusion(ctx, storage.ReviewExclusionRecord{Date: "2026-09-03", SourceType: "chat_conversation", SourceID: "A", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		bundle, err := NewAggregator(store, time.UTC).Build(ctx, "2026-09-03")
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.ChatTurns) != 1 || bundle.ChatTurns[0].ExternalConversationID != "B" {
			t.Fatalf("conversation exclusion bundle=%+v", bundle.ChatTurns)
		}
	})
	t.Run("always exclude", func(t *testing.T) {
		store := newStore(t)
		defer store.Close()
		if _, err := store.IngestChatTurn(ctx, storage.ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "A", CapturePolicy: "ALWAYS_EXCLUDE", ObservedAt: now}, storage.ChatTurnRecord{TurnKey: "A3", ObservedAt: now, LocalDate: "2026-09-03", ModeAtStart: "STUDY", EligibleForReview: true}, nil, now); err != nil {
			t.Fatal(err)
		}
		bundle, err := NewAggregator(store, time.UTC).Build(ctx, "2026-09-03")
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.ChatTurns) != 1 || bundle.ChatTurns[0].ExternalConversationID != "B" {
			t.Fatalf("always exclusion bundle=%+v", bundle.ChatTurns)
		}
	})
}

func TestAggregatorUsesSemanticDatabaseIDReference(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if _, err := store.RecordSemanticSnapshot(context.Background(), storage.SemanticSnapshotRecord{ObservedAt: now, LocalDate: "2026-09-03", Relation: "DISTRACTED", Confidence: .8, Activity: "BROWSING", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	bundle, err := NewAggregator(store, time.UTC).Build(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Semantic) != 1 || bundle.Semantic[0].ID <= 0 || bundle.Semantic[0].Ref != "semantic:"+itoa64(bundle.Semantic[0].ID) {
		t.Fatalf("semantic evidence=%+v", bundle.Semantic)
	}
	if !bundle.Quality.HasSemantic {
		t.Fatal("semantic evidence did not set quality.has_semantic")
	}
}
