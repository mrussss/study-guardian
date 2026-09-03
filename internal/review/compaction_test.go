package review

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"study-guardian/internal/evidence"
)

func chatTurn(conversation, key string, at time.Time, user, assistant string, finalized bool) evidence.ChatTurnSummary {
	return evidence.ChatTurnSummary{
		Ref: "chat_turn:" + key, TurnKey: key, ExternalConversationID: conversation,
		ConversationTitle: conversation, ObservedAt: at, TaskAtStart: "Go 学习",
		EligibleForReview: true, Finalized: finalized, UserContent: user, AssistantContent: assistant,
	}
}

func compactTestBundle(turns ...evidence.ChatTurnSummary) evidence.DailyEvidenceBundle {
	return evidence.DailyEvidenceBundle{Date: "2026-09-03", Timezone: "Asia/Shanghai", ChatTurns: turns}
}

func TestCompactGroupsSortsAndSeparatesEmptyConversationIDs(t *testing.T) {
	base := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	bundle := compactTestBundle(
		chatTurn("b", "b2", base.Add(2*time.Minute), "第二个", "答复", true),
		chatTurn("", "orphan", base, "无会话 ID", "部分", false),
		chatTurn("b", "b1", base, "第一个", "答复", true),
		chatTurn("a", "a1", base, "A", "答复", true),
	)
	input, err := Compact(bundle, ReviewLimits{MaxTurnChars: 100, MaxConversationChars: 1000, MaxFinalInputChars: 6000})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.ChatConversations) != 3 {
		t.Fatalf("conversations=%+v", input.ChatConversations)
	}
	if got := input.ChatConversations[0].ConversationID; got != "a" {
		t.Fatalf("first conversation=%q", got)
	}
	if got := input.ChatConversations[1].ConversationID; got != "b" {
		t.Fatalf("second conversation=%q", got)
	}
	if got := input.ChatConversations[2].ConversationID; got != "unassigned:orphan" {
		t.Fatalf("empty conversation group=%q", got)
	}
	if got := input.ChatConversations[1].Turns[0].Ref; got != "chat_turn:b1" {
		t.Fatalf("chronological turns=%q", got)
	}
}

func TestCompactIsDeterministicAndUsesHeadTailRuneBudget(t *testing.T) {
	base := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	longUser := "问题开头 中文内容 中文内容 中文内容 问题结尾"
	longAssistant := "回答开头 细节 细节 细节 细节 回答结尾"
	bundle := compactTestBundle(chatTurn("c", "t1", base, longUser, longAssistant, true))
	limits := ReviewLimits{MaxTurnChars: 16, MaxConversationChars: 16, MaxFinalInputChars: 6000}
	first, err := Compact(bundle, limits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compact(bundle, limits)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("non-deterministic output:\n%s\n%s", a, b)
	}
	turn := first.ChatConversations[0].Turns[0]
	if !turn.Truncated || !strings.Contains(turn.UserContent, "…") || len([]rune(turn.UserContent+turn.AssistantContent)) > 16 {
		t.Fatalf("unexpected bounded turn=%+v", turn)
	}
}

func TestCompactConversationHeadTailPartialAssistantAndCaps(t *testing.T) {
	base := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	bundle := compactTestBundle(
		chatTurn("c", "t1", base, "头部问题", "头部回答", true),
		chatTurn("c", "t2", base.Add(time.Minute), "中间问题", "中间回答", true),
		chatTurn("c", "t3", base.Add(2*time.Minute), "尾部问题", "尾部流式输出", false),
	)
	input, err := Compact(bundle, ReviewLimits{MaxTurnChars: 20, MaxConversationChars: 17, MaxFinalInputChars: 6000})
	if err != nil {
		t.Fatal(err)
	}
	conversation := input.ChatConversations[0]
	if len(conversation.Turns) != 2 || input.OmittedTurnCount != 1 {
		t.Fatalf("head/tail turns=%+v omitted=%d", conversation.Turns, input.OmittedTurnCount)
	}
	if conversation.Turns[0].Ref != "chat_turn:t1" || conversation.Turns[1].Ref != "chat_turn:t3" {
		t.Fatalf("head/tail order=%+v", conversation.Turns)
	}
	if conversation.Turns[1].Finalized {
		t.Fatal("partial assistant was marked finalized")
	}
}

func TestCompactFinalInputCapAndRawBundleUnchanged(t *testing.T) {
	base := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	original := strings.Repeat("中文证据 ", 100)
	bundle := compactTestBundle(
		chatTurn("c", "t1", base, original, original, true),
		chatTurn("c", "t2", base.Add(time.Minute), original, original, true),
	)
	before, _ := json.Marshal(bundle)
	input, err := Compact(bundle, ReviewLimits{MaxTurnChars: 1000, MaxConversationChars: 3000, MaxFinalInputChars: 900})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(bundle)
	if string(before) != string(after) {
		t.Fatal("compaction mutated raw evidence bundle")
	}
	encoded, _ := json.Marshal(input)
	if len([]rune(string(encoded))) > 900 {
		t.Fatalf("final input exceeded cap: %d", len([]rune(string(encoded))))
	}
	if input.OmittedTurnCount == 0 && !input.Truncated {
		t.Fatalf("expected bounded projection metadata: %+v", input)
	}
}
