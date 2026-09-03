package review

import (
	"encoding/json"
	"strings"
	"testing"

	"study-guardian/internal/evidence"
)

func TestSanitizeRedactsSecretsEmailsAndPathsWithoutChangingStructure(t *testing.T) {
	secret := "super-secret-token"
	input := ReviewInput{
		SchemaVersion: 1, Date: "2026-09-03", Timezone: "Asia/Shanghai",
		Sessions:          []evidence.SessionSummary{{Ref: "session:s1", ID: "s1", Task: `C:\Users\Lenovo\study`}},
		Distractions:      []evidence.DistractionSummary{{Ref: "distraction:d1", ID: "d1", Domain: "https://example.test/path?api_key=" + secret, Title: "title"}},
		Semantic:          []evidence.SemanticSummary{{Ref: "semantic:s1", ID: 7, Relation: "FOCUSED", Activity: "coding", Domain: "example.test"}},
		ChatConversations: []CompactedConversation{{ConversationID: "conversation-1", Title: "Study", Turns: []CompactedTurn{{Ref: "chat_turn:t1", UserContent: "Authorization: Bearer " + secret + ` password="` + secret + `" email me at user@example.com and \\wsl.localhost\Ubuntu-22.04\home\lls`, AssistantContent: "access_token=" + secret, Finalized: true}}}},
	}
	before, _ := json.Marshal(input)
	output, report, err := Sanitize(input, 6000)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("sanitizer mutated input")
	}
	encoded, _ := json.Marshal(output)
	text := string(encoded)
	if strings.Contains(text, secret) || strings.Contains(text, "user@example.com") || strings.Contains(text, `C:\Users\Lenovo`) {
		t.Fatalf("sensitive value survived sanitization: %s", text)
	}
	if report.RedactedSecretCount < 3 || report.RedactedEmailCount != 1 || report.RedactedPathCount < 2 {
		t.Fatalf("report=%+v", report)
	}
	if output.ChatConversations[0].ConversationID != "conversation-1" || output.ChatConversations[0].Turns[0].Ref != "chat_turn:t1" || output.Semantic[0].Relation != "FOCUSED" || output.Semantic[0].ID != 7 {
		t.Fatalf("structural evidence changed: %+v", output)
	}
}

func TestSanitizeRechecksFinalBudget(t *testing.T) {
	input := ReviewInput{
		SchemaVersion: 1, Date: "2026-09-03",
		Warnings:          []string{strings.Repeat("warning ", 100)},
		Semantic:          []evidence.SemanticSummary{{Ref: "semantic:s1", Task: strings.Repeat("中文证据 ", 100)}},
		ChatConversations: []CompactedConversation{{ConversationID: "c", Turns: []CompactedTurn{{Ref: "chat_turn:t1", UserContent: strings.Repeat("中文问题 ", 100), AssistantContent: strings.Repeat("中文回答 ", 100)}}}},
	}
	output, report, err := Sanitize(input, 700)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(output)
	if len([]rune(string(encoded))) > 700 {
		t.Fatalf("sanitized input exceeded budget: %d", len([]rune(string(encoded))))
	}
	if len(report.Warnings) == 0 || !output.Truncated {
		t.Fatalf("expected budget warning/truncation: report=%+v input=%+v", report, output)
	}
}
