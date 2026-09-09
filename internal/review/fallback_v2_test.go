package review

import (
	"strings"
	"testing"
	"time"

	"study-guardian/internal/evidence"
)

func TestFallbackV2RanksTasksAndBuildsFactualDynamicSummary(t *testing.T) {
	base := time.Date(2026, 9, 5, 9, 0, 0, 0, time.Local)
	endGo := base.Add(3*time.Hour + 12*time.Minute)
	endAlgo := base.Add(5 * time.Hour)
	bundle := evidence.DailyEvidenceBundle{
		Date:       "2026-09-05",
		DailyState: evidence.DailyStateSummary{StudySeconds: 5 * 3600},
		Motivation: evidence.MotivationSummary{CreditedFocusSeconds: 168 * 60},
		Sessions: []evidence.SessionSummary{
			{Ref: "session:go", Mode: "STUDY", Task: "Go", StartedAt: base, EndedAt: &endGo, DurationSeconds: 3*3600 + 12*60},
			{Ref: "session:algo", Mode: "STUDY", Task: "算法", StartedAt: base.Add(4 * time.Hour), EndedAt: &endAlgo, DurationSeconds: 65 * 60},
			{Ref: "session:break", Mode: "BREAK", Task: "英语", StartedAt: base, DurationSeconds: 10 * 3600},
		},
		ChatTurns: []evidence.ChatTurnSummary{{Ref: "chat_turn:1", TaskAtStart: "Go Context", ConversationTitle: "Go Context cancellation"}},
		Semantic:  []evidence.SemanticSummary{{Ref: "semantic:1", Activity: "编写并测试 Go 代码", Confidence: .8}},
	}
	doc := BuildFallback(bundle)
	if doc.Headline != "今天有效专注 168 分钟，主要投入 Go" {
		t.Fatalf("headline=%q", doc.Headline)
	}
	if len(doc.Accomplishments) != 0 {
		t.Fatalf("duration invented accomplishments: %+v", doc.Accomplishments)
	}
	if !strings.Contains(doc.TomorrowPriority, "Go") {
		t.Fatalf("priority=%q", doc.TomorrowPriority)
	}
	markdown := RenderMarkdown(doc, bundle)
	for _, want := range []string{"## 主要任务", "1. Go — 3h 12m", "2. 算法 — 1h 05m", "## 今日进展", "有效专注 2h 48m", "## 可以确认", "## 未完成 / 不能确认"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "英语 — 10h") {
		t.Fatal("BREAK duration must not rank as study investment")
	}
}

func TestFallbackV2LowEvidenceHeadline(t *testing.T) {
	doc := BuildFallback(evidence.DailyEvidenceBundle{Date: "2026-09-05", DailyState: evidence.DailyStateSummary{StudySeconds: 300}})
	if doc.Headline != "今天有学习记录，但主题证据不足" {
		t.Fatalf("headline=%q", doc.Headline)
	}
}

func TestFallbackUsesCompletedMissionAsAccomplishment(t *testing.T) {
	doc := BuildFallback(evidence.DailyEvidenceBundle{
		Date:     "2026-09-05",
		Missions: []evidence.CompletedMissionSummary{{Ref: "mission:m-1", Title: "完成 Context 超时练习", CompletedAt: time.Date(2026, 9, 5, 10, 0, 0, 0, time.Local)}},
	})
	if len(doc.Accomplishments) != 1 || !strings.Contains(doc.Accomplishments[0].Text, "Context 超时练习") {
		t.Fatalf("accomplishments=%+v", doc.Accomplishments)
	}
	if !strings.Contains(RenderMarkdown(doc, evidence.DailyEvidenceBundle{Date: "2026-09-05", Missions: []evidence.CompletedMissionSummary{{Title: "完成 Context 超时练习"}}}), "已完成任务：完成 Context 超时练习") {
		t.Fatal("markdown omitted completed mission")
	}
}

func TestFallbackSeparatesFocusedTopicsFromDistractionAndUnknownSemantic(t *testing.T) {
	doc := BuildFallback(evidence.DailyEvidenceBundle{
		Date: "2026-09-08",
		Semantic: []evidence.SemanticSummary{
			{Ref: "semantic:focused", Relation: "FOCUSED", Privacy: "NORMAL", Confidence: .9, Activity: "CODING", Topic: "Go 并发"},
			{Ref: "semantic:wechat", Relation: "DISTRACTED", Privacy: "NORMAL", Confidence: .95, Activity: "MESSAGING", Topic: "微信聊天"},
			{Ref: "semantic:game", Relation: "FOCUSED", Privacy: "NORMAL", Confidence: .95, Activity: "GAMING", Topic: "游戏"},
			{Ref: "semantic:unknown", Relation: "UNKNOWN", Privacy: "NORMAL", Confidence: .9, Activity: "READING", Topic: "不确定主题"},
		},
	})
	for _, topic := range doc.Topics {
		if topic.Name == "微信聊天" || topic.Name == "游戏" || topic.Name == "不确定主题" {
			t.Fatalf("non-learning semantic entered topics: %+v", doc.Topics)
		}
	}
	if len(doc.Topics) != 1 || doc.Topics[0].Name != "Go 并发" {
		t.Fatalf("topics=%+v", doc.Topics)
	}
}

func TestFallbackCapsTaskInvestmentWhenSessionTotalsDisagree(t *testing.T) {
	bundle := evidence.DailyEvidenceBundle{
		Date:       "2026-09-08",
		DailyState: evidence.DailyStateSummary{StudySeconds: 120},
		Quality:    evidence.EvidenceQuality{SessionDurationMismatch: true},
		Sessions:   []evidence.SessionSummary{{Ref: "session:go", Mode: "STUDY", Task: "Go", StartedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.Local), DurationSeconds: 240}},
	}
	markdown := RenderMarkdown(BuildFallback(bundle), bundle)
	if !strings.Contains(markdown, "Go — 2m") || strings.Contains(markdown, "Go — 4m") {
		t.Fatalf("capped fallback markdown=%s", markdown)
	}
}

func TestFallbackBoundsDurationsToCanonicalStudyState(t *testing.T) {
	bundle := evidence.DailyEvidenceBundle{
		Date:       "2026-09-08",
		DailyState: evidence.DailyStateSummary{StudySeconds: 120},
		Quality:    evidence.EvidenceQuality{StudyStatePresent: true},
		Motivation: evidence.MotivationSummary{CreditedFocusSeconds: 600},
		Sessions: []evidence.SessionSummary{{
			Ref: "session:go", Mode: "STUDY", Task: "Go",
			StartedAt:       time.Date(2026, 9, 8, 10, 0, 0, 0, time.Local),
			DurationSeconds: 240,
		}},
	}
	doc := BuildFallback(bundle)
	markdown := RenderMarkdown(doc, bundle)
	for _, want := range []string{"STUDY：2m", "有效专注：2m", "Go — 2m", "focus_duration_mismatch"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	for _, forbidden := range []string{"有效专注：10m", "Go — 4m"} {
		if strings.Contains(markdown, forbidden) {
			t.Fatalf("markdown violates duration invariant with %q:\n%s", forbidden, markdown)
		}
	}
}
