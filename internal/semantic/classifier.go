package semantic

import (
	"strings"
	"unicode"

	"study-guardian/internal/state"
)

// Classify is a local, deterministic semantic classifier. It describes the
// activity separately from TaskRelation: a distracted coding window remains
// CODING with relation DISTRACTED, so consumers do not conflate the two.
func Classify(c Candidate) (Activity, float64, string) {
	if c.UserMode != state.UserModeStudy {
		return ActivityUnknown, 0, "semantic unavailable outside STUDY mode"
	}
	if !c.Fresh {
		return ActivityUnknown, 0, "ActivityWatch unavailable or stale"
	}
	if c.Privacy == state.PrivacySensitive {
		return ActivityUnknown, 0, "sensitive privacy state"
	}
	if c.Interaction == state.InteractionUnknown {
		return ActivityUnknown, 0, "interaction state unknown"
	}

	values := []string{normalize(c.App), normalize(c.Title), normalize(c.Domain), normalize(c.Task)}
	if containsAny(values, "leetcode", "codeforces", "hackerrank", "acmicpc", "algorithm", "data structure", "competitive programming", "算法", "数据结构", "编程题") {
		return ActivityAlgorithm, 0.96, "local rule: algorithm/problem-solving signal"
	}
	if c.Relation == state.RelationFocused && containsAny(values, "chatgpt", "chat.openai", "openai", "claude", "anthropic", "qwen", "通义", "deepseek", "kimi", "moonshot", "gemini", "copilot", "perplexity") {
		return ActivityAIAssisted, 0.94, "local rule: focused AI-assisted study signal"
	}
	if containsAny(values, "code.exe", "code-insiders", "visual studio", "goland", "pycharm", "intellij", "android studio", "xcode", "vim", "neovim", "emacs", "编程", "源代码") {
		return ActivityCoding, 0.95, "local rule: coding tool signal"
	}
	if containsAny(values, ".pdf", "pdf viewer", "acrobat", "sumatrapdf", "kindle", "epub", "documentation", "docs.", "developer.mozilla", "pkg.go.dev", "readthedocs", "文档", "教材", "电子书", "阅读") {
		return ActivityReading, 0.91, "local rule: reading/document signal"
	}
	if containsAny(values, "microsoft word", "word.exe", "libreoffice writer", "writer", "typora", "obsidian", "notion", "notepad", "编辑文档", "写作", "作文") {
		return ActivityWriting, 0.91, "local rule: writing/editor signal"
	}
	if containsAny(values, "youtube", "youtu.be", "bilibili", "哔哩哔哩", "netflix", "twitch", "video", "player", "视频", "课程视频") {
		return ActivityWatching, 0.90, "local rule: video signal"
	}
	if containsAny(values, "chrome", "msedge", "edge", "firefox", "safari", "browser", "浏览器") || strings.TrimSpace(c.Domain) != "" {
		return ActivityBrowsing, 0.78, "local rule: browser/web signal"
	}
	if c.Relation == state.RelationFocused && strings.TrimSpace(c.Task) != "" {
		return ActivityGeneralStudy, 0.70, "local rule: focused study task without a narrower signal"
	}
	return ActivityUnknown, 0.35, "no deterministic semantic signal"
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func containsAny(values []string, needles ...string) bool {
	for _, value := range values {
		for _, needle := range needles {
			if strings.Contains(value, normalize(needle)) {
				return true
			}
		}
	}
	return false
}
