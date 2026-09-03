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
