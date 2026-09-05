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
