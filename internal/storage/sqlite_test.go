package storage

import (
	"context"
	"testing"
	"time"
)

func TestStorageLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Test Session
	session := SessionRecord{
		ID:              "sess-1",
		Mode:            "STUDY",
		Task:            "Go Lab",
		StartedAt:       now,
		DurationSeconds: 120,
	}
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Update Session
	end := now.Add(2 * time.Minute)
	session.EndedAt = &end
	session.EndReason = "USER_BREAK"
	if err := store.SaveSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Test Observation
	obs := ObservationRecord{
		Timestamp:   now,
		Interaction: "ACTIVE",
		Relation:    "FOCUSED",
		Privacy:     "NORMAL",
		Confidence:  0.95,
		Reason:      "VSCode focused on Go project",
		CurrentMode: "STUDY",
		Task:        "Go Lab",
	}
	if err := store.RecordObservation(ctx, obs); err != nil {
		t.Fatalf("failed to record observation: %v", err)
	}

	// Test Daily State
	if err := store.UpdateDailyState(ctx, "2026-09-02", 300, 1200, 300, 0, 1500, now); err != nil {
		t.Fatalf("failed to update daily state: %v", err)
	}

	// Test Classification Cache
	cacheKey := "code.exe|main.go|github.com|Go Lab|hash123"
	err = store.SetClassificationCache(ctx, cacheKey, "FOCUSED", 0.98, "Matched code editor", now, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("failed to set classification cache: %v", err)
	}

	rel, conf, reason, found := store.GetClassificationCache(ctx, cacheKey, now.Add(1*time.Minute))
	if !found || rel != "FOCUSED" || conf != 0.98 || reason != "Matched code editor" {
		t.Fatalf("cache lookup failed, got (%v, %s, %f, %s)", found, rel, conf, reason)
	}

	// Expired cache lookup
	_, _, _, found = store.GetClassificationCache(ctx, cacheKey, now.Add(15*time.Minute))
	if found {
		t.Fatalf("expected expired cache entry to not be found")
	}

	// Test Feedback
	if err := store.RecordFeedback(ctx, "rem-1", "ACTUALLY_STUDYING", now); err != nil {
		t.Fatalf("failed to record feedback: %v", err)
	}
}

func TestIngestChatTurnIsIdempotentAndKeepsEligibilityFrozen(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observed := time.Date(2026, 9, 3, 23, 58, 0, 0, time.FixedZone("CST", 8*60*60))
	turn := ChatTurnRecord{TurnKey: "turn-1", ObservedAt: observed, LocalDate: "2026-09-03", ModeAtStart: "STUDY", TaskAtStart: "Go", EligibleForReview: true, Finalized: false}
	conversation := ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "conversation-1", ObservedAt: observed}
	message := ChatMessageRecord{ExternalMessageID: "message-1", Role: "user", Content: "Explain interfaces", IsActive: true}
	if _, err := store.IngestChatTurn(context.Background(), conversation, turn, []ChatMessageRecord{message}, observed.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// A retry must update the raw message but must not turn the original STUDY
	// eligibility into BREAK after a later mode change.
	turn.ModeAtStart = "BREAK"
	turn.EligibleForReview = false
	message.Content = "Explain interfaces clearly"
	if _, err := store.IngestChatTurn(context.Background(), conversation, turn, []ChatMessageRecord{message}, observed.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var eligible, messageCount int
	if err := store.db.QueryRow(`SELECT eligible_for_review FROM chat_turns WHERE turn_key = 'turn-1'`).Scan(&eligible); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM chat_messages WHERE external_message_id = 'message-1'`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if eligible != 1 || messageCount != 1 {
		t.Fatalf("eligible=%d messages=%d, want 1/1", eligible, messageCount)
	}
}
