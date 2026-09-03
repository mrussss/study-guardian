package review

import (
	"fmt"
	"strings"

	"study-guardian/internal/evidence"
)

func BuildFallback(bundle evidence.DailyEvidenceBundle) Document {
	doc := Document{
		SchemaVersion: 1,
		Date:          bundle.Date,
		Headline:      "基于可验证记录的今日学习复盘",
		Behavior: Behavior{
			DistractionCount:      len(bundle.Distractions),
			LargestDistractionSec: largestDistraction(bundle),
		},
		TomorrowPriority: "从最后一个未完成或不确定的学习任务继续，先做一个可验证的小步骤。",
	}

	seen := make(map[string]bool)
	for _, turn := range bundle.ChatTurns {
		name := cleanTopic(turn.TaskAtStart)
		if name == "" {
			name = cleanTopic(turn.UserContent)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		doc.Topics = append(doc.Topics, Topic{
			Name: name, Summary: "今天出现了与该主题相关的 ChatGPT 学习提问；提问本身不代表已经掌握。",
			EvidenceRefs: []string{turn.Ref}, Confidence: .75,
		})
	}
	for _, semantic := range bundle.Semantic {
		name := cleanTopic(semantic.Task)
		if name == "" {
			name = cleanTopic(semantic.Activity)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		doc.Topics = append(doc.Topics, Topic{Name: name, Summary: "来自本地规则语义快照；仅用于说明出现过的活动，不代表有效专注或已经掌握。", EvidenceRefs: []string{semantic.Ref}, Confidence: semantic.Confidence})
	}
	if len(doc.Topics) > 8 {
		doc.Topics = doc.Topics[:8]
	}
	if bundle.DailyState.StudySeconds > 0 && len(doc.Topics) == 0 {
		doc.Unfinished = append(doc.Unfinished, "有学习时长记录，但没有足够的主题证据可供复盘。")
	}
	if len(bundle.ChatTurns) > 0 && len(doc.Accomplishments) == 0 {
		doc.Unfinished = append(doc.Unfinished, "当前证据能证明讨论过主题，不能证明已经完成或掌握；请补一次独立练习或完成确认。")
	}
	if len(doc.Unfinished) == 0 {
		doc.Unfinished = append(doc.Unfinished, "没有记录到可验证的未完成项；明天仍需用一个具体产出确认进展。")
	}
	return doc
}

func RenderMarkdown(doc Document, bundle evidence.DailyEvidenceBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 学习复盘\n\n", doc.Date)
	b.WriteString("## 今日记录\n")
	fmt.Fprintf(&b, "- STUDY：%s\n", formatDuration(bundle.DailyState.StudySeconds))
	fmt.Fprintf(&b, "- 有效专注：%s\n", formatDuration(bundle.Motivation.CreditedFocusSeconds))
	fmt.Fprintf(&b, "- 学习任务：%s\n", taskSummary(bundle))
	fmt.Fprintf(&b, "- ChatGPT 学习 Turn：%d\n", len(bundle.ChatTurns))
	fmt.Fprintf(&b, "- 跑偏：%d 次\n", len(bundle.Distractions))
	fmt.Fprintf(&b, "- 最大跑偏：%s\n\n", formatDuration(doc.Behavior.LargestDistractionSec))
	b.WriteString("## 今天出现较多的学习主题\n")
	for _, topic := range doc.Topics {
		fmt.Fprintf(&b, "- %s（证据：%s）\n", safeLine(topic.Name), strings.Join(topic.EvidenceRefs, ", "))
	}
	if len(doc.Topics) == 0 {
		b.WriteString("- 暂无足够主题证据\n")
	}
	b.WriteString("\n## 未完成 / 不确定\n")
	for _, item := range doc.Unfinished {
		fmt.Fprintf(&b, "- %s\n", safeLine(item))
	}
	b.WriteString("\n## 明日\n")
	fmt.Fprintf(&b, "%s\n", safeLine(doc.TomorrowPriority))
	warnings := append([]string(nil), bundle.Warnings...)
	warnings = append(warnings, doc.Warnings...)
	if len(warnings) > 0 {
		b.WriteString("\n> 证据提示：")
		b.WriteString(strings.Join(warnings, "；"))
		b.WriteString("\n")
	}
	return b.String()
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func taskSummary(bundle evidence.DailyEvidenceBundle) string {
	seen := map[string]bool{}
	var tasks []string
	for _, session := range bundle.Sessions {
		name := cleanTopic(session.Task)
		if name != "" && !seen[name] {
			seen[name] = true
			tasks = append(tasks, name)
		}
	}
	for _, turn := range bundle.ChatTurns {
		name := cleanTopic(turn.TaskAtStart)
		if name != "" && !seen[name] {
			seen[name] = true
			tasks = append(tasks, name)
		}
	}
	if len(tasks) == 0 {
		return "未记录具体任务"
	}
	if len(tasks) > 5 {
		tasks = tasks[:5]
	}
	return strings.Join(tasks, "、")
}

func cleanTopic(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 48 {
		value = string([]rune(value)[:48]) + "…"
	}
	return value
}

func safeLine(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "\n", " "), "\r", " ")
}

func largestDistraction(bundle evidence.DailyEvidenceBundle) int64 {
	var largest int64
	for _, item := range bundle.Distractions {
		if item.DurationSeconds > largest {
			largest = item.DurationSeconds
		}
	}
	return largest
}
