package review

import (
	"regexp"

	"study-guardian/internal/evidence"
)

type SanitizerReport struct {
	RedactedSecretCount int      `json:"redacted_secret_count"`
	RedactedEmailCount  int      `json:"redacted_email_count"`
	RedactedPathCount   int      `json:"redacted_path_count"`
	Warnings            []string `json:"warnings"`
}

var (
	authorizationSecretRE = regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*bearer\s+[^\s"'&,;}\]]+`)
	bearerSecretRE        = regexp.MustCompile(`(?i)\bbearer\s+[^\s"'&,;}\]]+`)
	labelSecretRE         = regexp.MustCompile(`(?i)\b(?:api[-_]?key|access[_-]?token|refresh[_-]?token|password|cookie(?:\s+secret)?)\s*["']?\s*[:=]\s*["']?[^\s"'&,;}\]]+`)
	querySecretRE         = regexp.MustCompile(`(?i)(?:[?&](?:api[-_]?key|access[_-]?token|refresh[_-]?token|token|password|secret|sig|signature)=)[^&#\s"'<>]+`)
	emailRE               = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	windowsPathRE         = regexp.MustCompile(`(?i)[A-Z]:\\Users\\[^\s"'<>|]+`)
	uncPathRE             = regexp.MustCompile(`(?i)\\\\[^\s"'<>|]+`)
	unixHomePathRE        = regexp.MustCompile(`(?i)(?:/home|/mnt/[a-z]/Users)/[^\s"'<>|]+`)
)

// Sanitize creates the only representation allowed to cross a future cloud
// provider boundary. It never mutates input and reruns the final ReviewInput
// budget after redaction because replacement text and preserved structure can
// change the encoded size.
func Sanitize(input ReviewInput, maxFinalChars int) (ReviewInput, SanitizerReport, error) {
	output := cloneReviewInput(input)
	report := SanitizerReport{}
	sanitize := func(value string) string {
		return sanitizeText(value, &report)
	}
	for index := range output.Sessions {
		output.Sessions[index].Task = sanitize(output.Sessions[index].Task)
	}
	for index := range output.Distractions {
		output.Distractions[index].App = sanitize(output.Distractions[index].App)
		output.Distractions[index].Title = sanitize(output.Distractions[index].Title)
		output.Distractions[index].Domain = sanitize(output.Distractions[index].Domain)
		output.Distractions[index].Task = sanitize(output.Distractions[index].Task)
	}
	for index := range output.Reminders {
		output.Reminders[index].Message = sanitize(output.Reminders[index].Message)
	}
	for index := range output.Semantic {
		output.Semantic[index].Task = sanitize(output.Semantic[index].Task)
		output.Semantic[index].App = sanitize(output.Semantic[index].App)
		output.Semantic[index].Title = sanitize(output.Semantic[index].Title)
		output.Semantic[index].Domain = sanitize(output.Semantic[index].Domain)
	}
	for index := range output.Warnings {
		output.Warnings[index] = sanitize(output.Warnings[index])
	}
	for conversationIndex := range output.ChatConversations {
		output.ChatConversations[conversationIndex].Title = sanitize(output.ChatConversations[conversationIndex].Title)
		for turnIndex := range output.ChatConversations[conversationIndex].Turns {
			turn := &output.ChatConversations[conversationIndex].Turns[turnIndex]
			turn.TaskAtStart = sanitize(turn.TaskAtStart)
			turn.UserContent = sanitize(turn.UserContent)
			turn.AssistantContent = sanitize(turn.AssistantContent)
		}
	}
	if report.RedactedSecretCount > 0 || report.RedactedEmailCount > 0 || report.RedactedPathCount > 0 {
		report.Warnings = append(report.Warnings, "sensitive values were redacted from the provider copy")
	}
	if maxFinalChars <= 0 {
		maxFinalChars = defaultMaxFinalInputChars
	}
	before, err := encodedRuneLen(output)
	if err != nil {
		return ReviewInput{}, report, err
	}
	if before > maxFinalChars {
		if err := enforceFinalBudget(&output, maxFinalChars); err != nil {
			return ReviewInput{}, report, err
		}
		report.Warnings = append(report.Warnings, "sanitized provider input was compacted again to the final budget")
	}
	return output, report, nil
}

func sanitizeText(value string, report *SanitizerReport) string {
	value = replaceMatches(value, authorizationSecretRE, report, "secret")
	value = replaceMatches(value, bearerSecretRE, report, "secret")
	value = replaceMatches(value, labelSecretRE, report, "secret")
	value = replaceMatches(value, querySecretRE, report, "secret")
	value = replaceMatches(value, windowsPathRE, report, "path")
	value = replaceMatches(value, uncPathRE, report, "path")
	value = replaceMatches(value, unixHomePathRE, report, "path")
	value = replaceMatches(value, emailRE, report, "email")
	return value
}

func replaceMatches(value string, pattern *regexp.Regexp, report *SanitizerReport, kind string) string {
	return pattern.ReplaceAllStringFunc(value, func(string) string {
		switch kind {
		case "secret":
			report.RedactedSecretCount++
			return "[REDACTED]"
		case "email":
			report.RedactedEmailCount++
			return "[EMAIL_REDACTED]"
		case "path":
			report.RedactedPathCount++
			return "[PATH_REDACTED]"
		default:
			return "[REDACTED]"
		}
	})
}

func cloneReviewInput(input ReviewInput) ReviewInput {
	output := input
	output.Sessions = append([]evidence.SessionSummary(nil), input.Sessions...)
	output.Distractions = append([]evidence.DistractionSummary(nil), input.Distractions...)
	output.Reminders = append([]evidence.ReminderSummary(nil), input.Reminders...)
	output.Semantic = append([]evidence.SemanticSummary(nil), input.Semantic...)
	output.Warnings = append([]string(nil), input.Warnings...)
	output.ChatConversations = append([]CompactedConversation(nil), input.ChatConversations...)
	for conversationIndex := range output.ChatConversations {
		output.ChatConversations[conversationIndex].Turns = append([]CompactedTurn(nil), input.ChatConversations[conversationIndex].Turns...)
	}
	return output
}
