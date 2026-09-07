package review

import (
	"encoding/json"
	"strings"
	"testing"

	"study-guardian/internal/evidence"
)

func TestNormalizeDocumentSerializesEmptyListsAsArrays(t *testing.T) {
	doc := NormalizeDocument(Document{
		Topics:          []Topic{{Name: "Go"}},
		Accomplishments: []Accomplishment{{Text: "未确认"}},
	})
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"topics", "accomplishments", "unfinished", "difficulties", "warnings"} {
		if _, ok := value[field].([]any); !ok {
			t.Fatalf("%s was not serialized as an array: %s", field, encoded)
		}
	}
	topic := value["topics"].([]any)[0].(map[string]any)
	if _, ok := topic["evidence_refs"].([]any); !ok {
		t.Fatalf("topic evidence_refs was not serialized as an array: %s", encoded)
	}
	accomplishment := value["accomplishments"].([]any)[0].(map[string]any)
	if _, ok := accomplishment["evidence_refs"].([]any); !ok {
		t.Fatalf("accomplishment evidence_refs was not serialized as an array: %s", encoded)
	}
}

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
