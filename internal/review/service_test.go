package review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"study-guardian/internal/storage"
)

type recordingProvider struct {
	input ReviewInput
	err   error
	calls int
}

func (p *recordingProvider) Generate(_ context.Context, input ReviewInput) (Document, ProviderMetadata, error) {
	p.calls++
	p.input = input
	if p.err != nil {
		return Document{}, ProviderMetadata{Provider: "test", Model: "test-model", PromptVersion: ReviewPromptVersion}, p.err
	}
	return Document{
		SchemaVersion:   1,
		Date:            input.Date,
		Headline:        "AI review",
		Topics:          []Topic{{Name: "local", EvidenceRefs: []string{"semantic:1"}, Confidence: .8}},
		Accomplishments: []Accomplishment{{Text: "完成", EvidenceRefs: []string{"semantic:1"}, Confidence: .9}},
		Behavior:        Behavior{DistractionCount: 999, LargestDistractionSec: 999},
	}, ProviderMetadata{Provider: "test", Model: "test-model", PromptVersion: ReviewPromptVersion}, nil
}

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

func TestReviewMarkdownFailureCannotPublishReadyOrReadyEvent(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	root := t.TempDir()
	outputPath := filepath.Join(root, "output-file")
	if err := os.WriteFile(outputPath, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, time.UTC, outputPath)
	if _, err := service.GenerateFallback(context.Background(), "2026-09-03"); err == nil {
		t.Fatal("expected Markdown write failure")
	}
	record, err := store.LoadDailyReview(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusPending {
		t.Fatalf("failed write published status=%s, want PENDING", record.Status)
	}
	events, err := store.ListUIEvents(context.Background(), 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("failed write emitted ready event: %+v", events)
	}
}

func TestGenerateUsesSanitizedInputValidatorAndPersistsAI(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if _, err := store.RecordSemanticSnapshot(context.Background(), storage.SemanticSnapshotRecord{ObservedAt: now, LocalDate: "2026-09-03", App: `C:\Users\Lenovo\study`, Relation: "FOCUSED", Confidence: .9, Activity: "coding", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{}
	service := NewService(store, time.UTC, t.TempDir())
	service.SetProvider(provider)
	service.SetLimits(ReviewLimits{MaxTurnChars: 120, MaxConversationChars: 240, MaxFinalInputChars: 4000})
	record, err := service.Generate(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if record.GenerationMode != "AI" || record.Provider != "test" || record.Model != "test-model" {
		t.Fatalf("record=%+v", record)
	}
	events, err := store.ListUIEvents(context.Background(), 0, 20)
	if err != nil || len(events) != 1 || events[0].EventType != "DAILY_REVIEW_READY" {
		t.Fatalf("ready events=%+v err=%v", events, err)
	}
	if strings.Contains(provider.input.Semantic[0].App, `C:\Users\Lenovo`) {
		t.Fatalf("provider received unsanitized path: %+v", provider.input.Semantic[0])
	}
	if len(provider.input.Semantic) != 1 || provider.input.Semantic[0].Ref != "semantic:1" {
		t.Fatalf("provider input=%+v", provider.input)
	}
	if strings.Contains(record.ReviewJSON, "完成") || !strings.Contains(record.ReviewJSON, "AI review") {
		t.Fatalf("validator did not strip unsupported accomplishment: %s", record.ReviewJSON)
	}
}

func TestGenerateProviderFailureFallsBackWithBoundedErrorCode(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	provider := &recordingProvider{err: ProviderError{Kind: ProviderErrorTimeout, Cause: errors.New("private cause")}}
	service := NewService(store, time.UTC, t.TempDir())
	service.SetProvider(provider)
	record, err := service.Generate(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if record.GenerationMode != "FALLBACK" || record.ErrorCode != string(ProviderErrorTimeout) || strings.Contains(record.ErrorCode, "private") {
		t.Fatalf("record=%+v", record)
	}
}

func TestGenerateSkipsSameReadyInputAndMarksChangedEvidenceStale(t *testing.T) {
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	provider := &recordingProvider{}
	service := NewService(store, time.UTC, t.TempDir())
	service.SetProvider(provider)
	first, err := service.Generate(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Generate(context.Background(), "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || second.Revision != first.Revision || second.InputHash != first.InputHash {
		t.Fatalf("same input was regenerated: calls=%d first=%+v second=%+v", provider.calls, first, second)
	}
	events, err := store.ListUIEvents(context.Background(), 0, 20)
	if err != nil || len(events) != 1 {
		t.Fatalf("same input duplicated ready event: events=%+v err=%v", events, err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if _, err := store.RecordSemanticSnapshot(context.Background(), storage.SemanticSnapshotRecord{ObservedAt: now, LocalDate: "2026-09-03", Relation: "FOCUSED", Confidence: .9, Activity: "coding", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	changed, err := service.MarkStaleIfChanged(context.Background(), "2026-09-03")
	if err != nil || !changed {
		t.Fatalf("changed evidence stale=%v err=%v", changed, err)
	}
	stale, err := service.Get(context.Background(), "2026-09-03")
	if err != nil || stale.Status != StatusStale {
		t.Fatalf("stale review=%+v err=%v", stale, err)
	}
}

func TestBackfillPreviousDayHonorsConfigAndRequiresEvidence(t *testing.T) {
	now := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.RecordSemanticSnapshot(context.Background(), storage.SemanticSnapshotRecord{ObservedAt: now.Add(-24 * time.Hour), LocalDate: "2026-09-03", Relation: "FOCUSED", Confidence: .8, Activity: "reading", SourceKind: "LOCAL_RULE"}); err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{}
	service := NewService(store, time.UTC, t.TempDir())
	service.SetProvider(provider)
	if err := service.BackfillPreviousDay(context.Background(), now, true); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("backfill calls=%d", provider.calls)
	}
	if _, err := service.Get(context.Background(), "2026-09-03"); err != nil {
		t.Fatalf("backfill did not persist review: %v", err)
	}
	if err := service.BackfillPreviousDay(context.Background(), now, false); err != nil {
		t.Fatal(err)
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
