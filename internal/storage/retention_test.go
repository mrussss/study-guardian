package storage

import (
	"context"
	"testing"
	"time"
)

func TestPruneRetentionDeletesOnlyExpiredRawEvidence(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	oldChat := now.Add(-31 * 24 * time.Hour)
	newChat := now.Add(-29 * 24 * time.Hour)
	oldSemantic := now.Add(-181 * 24 * time.Hour)
	newSemantic := now.Add(-179 * 24 * time.Hour)

	if _, err := store.IngestChatTurn(ctx,
		ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "old", ObservedAt: oldChat},
		ChatTurnRecord{TurnKey: "old-turn", ObservedAt: oldChat, LocalDate: "2026-08-04", ModeAtStart: "STUDY"},
		[]ChatMessageRecord{{ExternalMessageID: "old-message", Role: "user", Content: "old", ObservedAt: oldChat}}, oldChat); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IngestChatTurn(ctx,
		ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "new", ObservedAt: newChat},
		ChatTurnRecord{TurnKey: "new-turn", ObservedAt: newChat, LocalDate: "2026-08-06", ModeAtStart: "STUDY"},
		[]ChatMessageRecord{{ExternalMessageID: "new-message", Role: "user", Content: "new", ObservedAt: newChat}}, newChat); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordSemanticSnapshot(ctx, SemanticSnapshotRecord{ObservedAt: oldSemantic, LocalDate: "2026-03-07", Relation: "FOCUSED", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordSemanticSnapshot(ctx, SemanticSnapshotRecord{ObservedAt: newSemantic, LocalDate: "2026-03-09", Relation: "FOCUSED", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(ctx, SessionRecord{ID: "session-keep", Mode: "STUDY", StartedAt: oldChat}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDailyReview(ctx, DailyReviewRecord{Date: "2026-08-04", Status: "READY", UpdatedAt: oldChat}); err != nil {
		t.Fatal(err)
	}

	stats, err := store.PruneRetention(ctx, now, 30, 180)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RawMessagesDeleted != 1 || stats.RawTurnsDeleted != 1 || stats.RawConversationsDeleted != 1 || stats.SemanticDeleted != 1 {
		t.Fatalf("unexpected retention stats: %+v", stats)
	}

	var count int
	for _, check := range []struct {
		name  string
		query string
		want  int
	}{
		{"chat messages", "SELECT COUNT(*) FROM chat_messages", 1},
		{"chat turns", "SELECT COUNT(*) FROM chat_turns", 1},
		{"chat conversations", "SELECT COUNT(*) FROM chat_conversations", 1},
		{"semantic snapshots", "SELECT COUNT(*) FROM semantic_snapshots", 1},
		{"sessions", "SELECT COUNT(*) FROM sessions", 1},
		{"daily reviews", "SELECT COUNT(*) FROM daily_reviews", 1},
	} {
		if err := store.db.QueryRowContext(ctx, check.query).Scan(&count); err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if count != check.want {
			t.Fatalf("%s: got %d want %d", check.name, count, check.want)
		}
	}
}

func TestPruneRetentionTreatsZeroDaysAsDisabled(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if _, err := store.RecordSemanticSnapshot(context.Background(), SemanticSnapshotRecord{ObservedAt: now.Add(-365 * 24 * time.Hour), LocalDate: "2025-09-05", Relation: "FOCUSED", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	stats, err := store.PruneRetention(context.Background(), now, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (RetentionStats{}) {
		t.Fatalf("zero-day cleanup should be disabled: %+v", stats)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM semantic_snapshots`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("snapshot was deleted with disabled retention: %d", count)
	}
}
