package review

import (
	"math"
	"reflect"
	"testing"

	"study-guardian/internal/evidence"
)

func TestValidateDocumentKeepsOnlyExistingRefsAndNormalizesEvidenceMetrics(t *testing.T) {
	input := ReviewInput{
		Date:              "2026-09-03",
		Sessions:          []evidence.SessionSummary{{Ref: "session:s1"}},
		Distractions:      []evidence.DistractionSummary{{Ref: "distraction:d1", DurationSeconds: 42}},
		ChatConversations: []CompactedConversation{{Turns: []CompactedTurn{{Ref: "chat_turn:t1"}}}},
	}
	document := Document{
		SchemaVersion: 1, Date: "2026-09-02",
		Topics:   []Topic{{Name: "Go", EvidenceRefs: []string{"chat_turn:t1", "missing", "chat_turn:t1"}, Confidence: .8}},
		Behavior: Behavior{DistractionCount: 99, LargestDistractionSec: 999, AverageRecoverySec: -1},
	}
	validated, report, err := ValidateDocument(input, document)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Date != input.Date || len(validated.Topics) != 1 || !reflect.DeepEqual(validated.Topics[0].EvidenceRefs, []string{"chat_turn:t1"}) {
		t.Fatalf("validated=%+v", validated)
	}
	if validated.Behavior.DistractionCount != 1 || validated.Behavior.LargestDistractionSec != 42 || validated.Behavior.AverageRecoverySec != 0 {
		t.Fatalf("behavior=%+v", validated.Behavior)
	}
	if report.InvalidReferenceCount != 1 || len(report.Warnings) == 0 {
		t.Fatalf("report=%+v", report)
	}
	if document.Date != "2026-09-02" || len(document.Topics[0].EvidenceRefs) != 3 {
		t.Fatal("validator mutated provider document")
	}
}

func TestValidateDocumentRemovesUnsupportedClaims(t *testing.T) {
	input := ReviewInput{Date: "2026-09-03", Semantic: []evidence.SemanticSummary{{Ref: "semantic:s1"}}}
	document := Document{
		SchemaVersion: 1, Date: input.Date,
		Topics:          []Topic{{Name: "主题", EvidenceRefs: []string{"semantic:s1"}, Confidence: math.NaN()}, {Name: "有效", EvidenceRefs: []string{"semantic:s1"}, Confidence: .7}},
		Accomplishments: []Accomplishment{{Text: "已经完成", EvidenceRefs: []string{"semantic:s1"}, Confidence: .99}},
	}
	validated, report, err := ValidateDocument(input, document)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Topics) != 1 || validated.Topics[0].Name != "有效" {
		t.Fatalf("topics=%+v", validated.Topics)
	}
	if len(validated.Accomplishments) != 0 || report.StrippedAccomplishmentCount != 1 {
		t.Fatalf("accomplishments=%+v report=%+v", validated.Accomplishments, report)
	}
}

func TestValidateDocumentRejectsUnsupportedSchema(t *testing.T) {
	_, _, err := ValidateDocument(ReviewInput{Date: "2026-09-03"}, Document{SchemaVersion: 2})
	if err == nil {
		t.Fatal("unsupported schema should fail")
	}
}

func TestValidateDocumentSeparatesTopicAndCompletionEvidence(t *testing.T) {
	input := ReviewInput{
		Date:              "2026-09-08",
		Sessions:          []evidence.SessionSummary{{Ref: "session:focus"}},
		Semantic:          []evidence.SemanticSummary{{Ref: "semantic:learning"}},
		BehaviorSemantic:  []evidence.SemanticSummary{{Ref: "semantic:behavior"}},
		ChatConversations: []CompactedConversation{{Turns: []CompactedTurn{{Ref: "chat_turn:discussion"}}}},
		CompletedMissions: []evidence.CompletedMissionSummary{{Ref: "mission:done", Title: "完成练习"}},
	}
	document := Document{
		SchemaVersion: 1,
		Date:          input.Date,
		Topics: []Topic{
			{Name: "可验证主题", EvidenceRefs: []string{"session:focus", "semantic:learning"}, Confidence: .8},
			{Name: "行为误判主题", EvidenceRefs: []string{"semantic:behavior"}, Confidence: .8},
		},
		Accomplishments: []Accomplishment{
			{Text: "时长证明完成", EvidenceRefs: []string{"session:focus", "chat_turn:discussion", "semantic:learning"}, Confidence: .8},
			{Text: "任务确实完成", EvidenceRefs: []string{"mission:done"}, Confidence: 1},
		},
	}
	validated, report, err := ValidateDocument(input, document)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Topics) != 1 || validated.Topics[0].Name != "可验证主题" {
		t.Fatalf("topics=%+v report=%+v", validated.Topics, report)
	}
	if len(validated.Accomplishments) != 1 || validated.Accomplishments[0].Text != "任务确实完成" {
		t.Fatalf("accomplishments=%+v report=%+v", validated.Accomplishments, report)
	}
	if report.StrippedTopicCount != 1 || report.StrippedAccomplishmentCount != 1 {
		t.Fatalf("report=%+v", report)
	}
}
