package review

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"study-guardian/internal/storage"
)

func TestGenerateFallbackPersistsCanonicalReviewAndMarkdownAtomically(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := store.UpdateDailyState(context.Background(), "2026-09-03", 0, 1800, 0, 0, 1200, now); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	service := NewService(store, time.UTC, root)
	record, err := service.GenerateFallback(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusReady || record.GenerationMode != "FALLBACK" || record.Revision != 1 {
		t.Fatalf("record=%+v", record)
	}
	content, err := os.ReadFile(root + "/2026-09-03.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# 2026-09-03 学习复盘") {
		t.Fatalf("unexpected markdown: %s", content)
	}
	record, err = service.GenerateFallback(context.Background(), "2026-09-03")
	if err != nil || record.Revision != 2 {
		t.Fatalf("second generation record=%+v err=%v", record, err)
	}
}

func TestExclusionMarksReviewStaleAndChangesRegeneratedInput(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if _, err := store.IngestChatTurn(context.Background(), storage.ChatConversationRecord{Platform: "chatgpt", ExternalConversationID: "conversation-a", ObservedAt: now}, storage.ChatTurnRecord{TurnKey: "turn-a", ObservedAt: now, LocalDate: "2026-09-03", ModeAtStart: "STUDY", TaskAtStart: "Go", EligibleForReview: true}, nil, now); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, time.UTC, t.TempDir())
	first, err := service.GenerateFallback(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Exclude(context.Background(), "2026-09-03", "chat_conversation", "conversation-a"); err != nil {
		t.Fatal(err)
	}
	stale, err := service.Get(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != StatusStale {
		t.Fatalf("status=%s, want STALE", stale.Status)
	}
	second, err := service.GenerateFallback(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != StatusReady || second.Revision != first.Revision+1 || second.InputHash == first.InputHash {
		t.Fatalf("second=%+v first=%+v", second, first)
	}
	bundle, err := service.Evidence(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.ChatTurns) != 0 || strings.Contains(second.Markdown, "conversation-a") {
		t.Fatalf("excluded evidence remained: bundle=%+v markdown=%s", bundle.ChatTurns, second.Markdown)
	}
	if err := service.Exclude(context.Background(), "2026-09-03", "whatever", "x"); err == nil {
		t.Fatal("unsupported exclusion should fail")
	}
}
