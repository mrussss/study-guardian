package review

import (
	"strings"
	"testing"

	"study-guardian/internal/evidence"
)

func TestFallbackDoesNotInventAccomplishments(t *testing.T) {
	bundle := evidence.DailyEvidenceBundle{Date: "2026-09-03", DailyState: evidence.DailyStateSummary{StudySeconds: 3600}, ChatTurns: []evidence.ChatTurnSummary{{Ref: "chat_turn:1", TaskAtStart: "Go interface"}}}
	doc := BuildFallback(bundle)
	if len(doc.Accomplishments) != 0 {
		t.Fatalf("fallback invented accomplishments: %+v", doc.Accomplishments)
	}
	markdown := RenderMarkdown(doc, bundle)
	if !strings.Contains(markdown, "Go interface") || !strings.Contains(markdown, "不能证明已经完成") {
		t.Fatalf("unexpected fallback markdown:\n%s", markdown)
	}
}
