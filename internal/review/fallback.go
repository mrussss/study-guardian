package review

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"study-guardian/internal/evidence"
)

type taskInvestment struct {
	Name       string
	Seconds    int64
	Sessions   int
	LastAt     time.Time
	References []string
}

func BuildFallback(bundle evidence.DailyEvidenceBundle) Document {
	tasks := rankTasks(bundle)
	doc := Document{SchemaVersion: 1, Date: bundle.Date, Behavior: Behavior{DistractionCount: len(bundle.Distractions), LargestDistractionSec: largestDistraction(bundle)}}
	doc.Headline = fallbackHeadline(bundle, tasks)
	if len(tasks) > 0 {
		doc.TomorrowPriority = fmt.Sprintf("明天优先继续 %s；先完成一个可以独立验证的小任务。", tasks[0].Name)
	} else {
		doc.TomorrowPriority = "先选择一个明确学习任务，再完成一个可验证的小步骤。"
	}

	seen := make(map[string]bool)
	addTopic := func(name, summary, ref string, confidence float64) {
		name = cleanTopic(name)
		key := strings.ToLower(name)
		if name == "" || seen[key] || len(doc.Topics) >= 8 {
			return
		}
		seen[key] = true
		doc.Topics = append(doc.Topics, Topic{Name: name, Summary: summary, EvidenceRefs: []string{ref}, Confidence: confidence})
	}
	for _, task := range tasks {
		addTopic(task.Name, fmt.Sprintf("今天记录到 %s 的相关学习投入；时长不能证明已经完成或掌握。", formatDuration(task.Seconds)), task.References[0], .9)
	}
	for _, semantic := range bundle.Semantic {
		if semantic.Confidence < .6 {
			continue
		}
		addTopic(semantic.Activity, "来自稳定的本地语义记录；只说明出现过该活动。", semantic.Ref, semantic.Confidence)
	}
	for _, turn := range bundle.ChatTurns {
		name := cleanTopic(turn.TaskAtStart)
		if name == "" {
			name = boundedChatTopic(turn.ConversationTitle, turn.UserContent)
		}
		addTopic(name, "今天有与该主题相关的 ChatGPT 学习讨论；讨论本身不代表已经独立掌握。", turn.Ref, .72)
	}

	for index, task := range tasks {
		if index >= 3 {
			break
		}
		doc.Unfinished = append(doc.Unfinished, fmt.Sprintf("%s：今天有持续学习记录，但没有可验证的“已完成”证据。", task.Name))
	}
	if len(bundle.ChatTurns) > 0 {
		doc.Unfinished = append(doc.Unfinished, "ChatGPT 中记录了学习讨论，但不能证明已经完成或独立掌握。")
	}
	if bundle.DailyState.StudySeconds > 0 && len(doc.Topics) == 0 {
		doc.Unfinished = append(doc.Unfinished, "有学习时长记录，但主题证据不足。")
	}
	if len(doc.Unfinished) == 0 {
		doc.Unfinished = append(doc.Unfinished, "没有记录到可验证的完成项；下次可用一个具体产出确认进展。")
	}
	return doc
}

func fallbackHeadline(bundle evidence.DailyEvidenceBundle, tasks []taskInvestment) string {
	focusMinutes := bundle.Motivation.CreditedFocusSeconds / 60
	if focusMinutes > 0 && len(tasks) > 0 {
		return fmt.Sprintf("今天有效专注 %d 分钟，主要投入 %s", focusMinutes, tasks[0].Name)
	}
	if len(tasks) > 0 {
		return fmt.Sprintf("今天记录了 %d 个学习任务，主要投入 %s", len(tasks), tasks[0].Name)
	}
	if focusMinutes > 0 {
		return fmt.Sprintf("今天有效专注 %d 分钟，但主题证据不足", focusMinutes)
	}
	if bundle.DailyState.StudySeconds > 0 {
		return "今天有学习记录，但主题证据不足"
	}
	return "今天尚无足够的学习记录"
}

func rankTasks(bundle evidence.DailyEvidenceBundle) []taskInvestment {
	byKey := map[string]*taskInvestment{}
	for _, session := range bundle.Sessions {
		if session.Mode != "STUDY" {
			continue
		}
		name := cleanTopic(session.Task)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		item := byKey[key]
		if item == nil {
			item = &taskInvestment{Name: name}
			byKey[key] = item
		}
		item.Seconds += max64(session.DurationSeconds, 0)
		item.Sessions++
		item.References = append(item.References, session.Ref)
		last := session.StartedAt
		if session.EndedAt != nil {
			last = *session.EndedAt
		}
		if last.After(item.LastAt) {
			item.LastAt = last
		}
	}
	items := make([]taskInvestment, 0, len(byKey))
	for _, item := range byKey {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Seconds != items[j].Seconds {
			return items[i].Seconds > items[j].Seconds
		}
		return items[i].LastAt.After(items[j].LastAt)
	})
	if len(items) > 5 {
		items = items[:5]
	}
	return items
}

func RenderMarkdown(doc Document, bundle evidence.DailyEvidenceBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 学习复盘\n\n", doc.Date)
	fmt.Fprintf(&b, "%s\n\n", safeLine(doc.Headline))
	b.WriteString("## 今日记录\n")
	fmt.Fprintf(&b, "- STUDY：%s\n- 有效专注：%s\n- 学习会话：%d 次\n- ChatGPT 学习 Turn：%d\n- 语义记录：%d 条\n- 跑偏：%d 次\n- 最大跑偏：%s\n\n", formatDuration(bundle.DailyState.StudySeconds), formatDuration(bundle.Motivation.CreditedFocusSeconds), studySessionCount(bundle), len(bundle.ChatTurns), len(bundle.Semantic), len(bundle.Distractions), formatDuration(doc.Behavior.LargestDistractionSec))
	tasks := rankTasks(bundle)
	b.WriteString("## 主要任务\n")
	if len(tasks) == 0 {
		b.WriteString("- 未记录具体任务\n")
	}
	for index, task := range tasks {
		fmt.Fprintf(&b, "%d. %s — %s（%d 次学习会话）\n", index+1, safeLine(task.Name), formatDuration(task.Seconds), task.Sessions)
	}
	b.WriteString("\n## 今日进展\n")
	if len(tasks) > 0 {
		fmt.Fprintf(&b, "- %s 相关学习记录持续 %s\n", safeLine(tasks[0].Name), formatDuration(tasks[0].Seconds))
	}
	fmt.Fprintf(&b, "- 有效专注 %s\n", formatDuration(bundle.Motivation.CreditedFocusSeconds))
	if len(bundle.ChatTurns) > 0 {
		fmt.Fprintf(&b, "- ChatGPT 记录到 %d 个学习 Turn\n", len(bundle.ChatTurns))
	}
	if len(bundle.Semantic) > 0 {
		fmt.Fprintf(&b, "- 本地记录到 %d 条语义快照\n", len(bundle.Semantic))
	}
	b.WriteString("\n## 今天出现较多的学习主题\n")
	for _, topic := range doc.Topics {
		fmt.Fprintf(&b, "- %s：%s（证据：%s）\n", safeLine(topic.Name), safeLine(topic.Summary), strings.Join(topic.EvidenceRefs, ", "))
	}
	if len(doc.Topics) == 0 {
		b.WriteString("- 暂无足够主题证据\n")
	}
	b.WriteString("\n## 可以确认\n- 上述时长、任务标签和记录数量来自本地事实记录。\n")
	b.WriteString("\n## 未完成 / 不能确认\n")
	for _, item := range doc.Unfinished {
		fmt.Fprintf(&b, "- %s\n", safeLine(item))
	}
	b.WriteString("\n## 明日\n")
	fmt.Fprintf(&b, "%s\n", safeLine(doc.TomorrowPriority))
	warnings := append(append([]string(nil), bundle.Warnings...), doc.Warnings...)
	if len(warnings) > 0 {
		b.WriteString("\n> 证据提示：")
		b.WriteString(strings.Join(warnings, "；"))
		b.WriteString("\n")
	}
	return b.String()
}

func studySessionCount(bundle evidence.DailyEvidenceBundle) int {
	count := 0
	for _, session := range bundle.Sessions {
		if session.Mode == "STUDY" {
			count++
		}
	}
	return count
}
func boundedChatTopic(title, prompt string) string {
	if topic := cleanTopic(title); topic != "" {
		return topic
	}
	words := strings.Fields(prompt)
	if len(words) > 8 {
		words = words[:8]
	}
	return cleanTopic(strings.Join(words, " "))
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
	tasks := rankTasks(bundle)
	if len(tasks) == 0 {
		return "未记录具体任务"
	}
	names := make([]string, len(tasks))
	for i := range tasks {
		names[i] = tasks[i].Name
	}
	return strings.Join(names, "、")
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
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
