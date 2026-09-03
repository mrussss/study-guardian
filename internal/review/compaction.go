package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"study-guardian/internal/evidence"
)

// ReviewLimits are the single set of review budgets. They mirror the existing
// review.limits config and are deliberately reused by compaction and the
// later provider/sanitizer stages.
type ReviewLimits struct {
	MaxTurnChars         int
	MaxConversationChars int
	MaxFinalInputChars   int
}

const (
	defaultMaxTurnChars         = 12000
	defaultMaxConversationChars = 40000
	defaultMaxFinalInputChars   = 60000
)

type CompactedTurn struct {
	Ref              string    `json:"ref"`
	ObservedAt       time.Time `json:"observed_at"`
	TaskAtStart      string    `json:"task_at_start"`
	Finalized        bool      `json:"finalized"`
	UserContent      string    `json:"user_content"`
	AssistantContent string    `json:"assistant_content"`
	Truncated        bool      `json:"truncated"`
}

type CompactedConversation struct {
	ConversationID string          `json:"conversation_id"`
	Title          string          `json:"title"`
	Turns          []CompactedTurn `json:"turns"`
}

// ReviewInput is the deterministic, bounded projection consumed by review
// providers. It contains existing evidence summaries, not a second database
// or a second source of truth.
type ReviewInput struct {
	SchemaVersion        int                           `json:"schema_version"`
	Date                 string                        `json:"date"`
	Timezone             string                        `json:"timezone"`
	DailyState           evidence.DailyStateSummary    `json:"daily_state"`
	Sessions             []evidence.SessionSummary     `json:"sessions"`
	Distractions         []evidence.DistractionSummary `json:"distractions"`
	Reminders            []evidence.ReminderSummary    `json:"reminders"`
	Motivation           evidence.MotivationSummary    `json:"motivation"`
	Semantic             []evidence.SemanticSummary    `json:"semantic"`
	ChatConversations    []CompactedConversation       `json:"chat_conversations"`
	Quality              evidence.EvidenceQuality      `json:"quality"`
	Warnings             []string                      `json:"warnings"`
	OmittedTurnCount     int                           `json:"omitted_turn_count"`
	OmittedEvidenceCount int                           `json:"omitted_evidence_count"`
	Truncated            bool                          `json:"truncated"`
}

func normalizeReviewLimits(limits ReviewLimits) ReviewLimits {
	if limits.MaxTurnChars <= 0 {
		limits.MaxTurnChars = defaultMaxTurnChars
	}
	if limits.MaxConversationChars <= 0 {
		limits.MaxConversationChars = defaultMaxConversationChars
	}
	if limits.MaxFinalInputChars <= 0 {
		limits.MaxFinalInputChars = defaultMaxFinalInputChars
	}
	return limits
}

// Compact projects the already-aggregated daily evidence without querying the
// database. Its output is deterministic for byte-equivalent input.
func Compact(bundle evidence.DailyEvidenceBundle, limits ReviewLimits) (ReviewInput, error) {
	limits = normalizeReviewLimits(limits)
	conversations, omittedTurns, truncated := compactConversations(bundle.ChatTurns, limits)
	input := ReviewInput{
		SchemaVersion:        1,
		Date:                 bundle.Date,
		Timezone:             bundle.Timezone,
		DailyState:           bundle.DailyState,
		Sessions:             append([]evidence.SessionSummary(nil), bundle.Sessions...),
		Distractions:         append([]evidence.DistractionSummary(nil), bundle.Distractions...),
		Reminders:            append([]evidence.ReminderSummary(nil), bundle.Reminders...),
		Motivation:           bundle.Motivation,
		Semantic:             append([]evidence.SemanticSummary(nil), bundle.Semantic...),
		ChatConversations:    conversations,
		Quality:              bundle.Quality,
		Warnings:             append([]string(nil), bundle.Warnings...),
		OmittedTurnCount:     omittedTurns,
		OmittedEvidenceCount: 0,
		Truncated:            truncated,
	}
	boundFixedEvidence(&input)
	if err := enforceFinalBudget(&input, limits.MaxFinalInputChars); err != nil {
		return ReviewInput{}, err
	}
	return input, nil
}

func compactConversations(turns []evidence.ChatTurnSummary, limits ReviewLimits) ([]CompactedConversation, int, bool) {
	groups := make(map[string][]evidence.ChatTurnSummary)
	for _, turn := range turns {
		conversationID := strings.TrimSpace(turn.ExternalConversationID)
		if conversationID == "" {
			conversationID = "unassigned:" + turn.TurnKey
		}
		groups[conversationID] = append(groups[conversationID], turn)
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	conversations := make([]CompactedConversation, 0, len(ids))
	omitted := 0
	truncated := false
	for _, id := range ids {
		group := groups[id]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].ObservedAt.Equal(group[j].ObservedAt) {
				return group[i].TurnKey < group[j].TurnKey
			}
			return group[i].ObservedAt.Before(group[j].ObservedAt)
		})
		conversation := CompactedConversation{ConversationID: id}
		if len(group) > 0 {
			conversation.Title = group[0].ConversationTitle
		}
		for _, turn := range group {
			compacted := compactTurn(turn, limits.MaxTurnChars)
			conversation.Turns = append(conversation.Turns, compacted)
			truncated = truncated || compacted.Truncated
		}
		var conversationOmitted int
		var conversationTruncated bool
		conversation.Turns, conversationOmitted, conversationTruncated = fitConversation(conversation.Turns, limits.MaxConversationChars)
		omitted += conversationOmitted
		truncated = truncated || conversationTruncated
		if len(conversation.Turns) > 0 {
			conversations = append(conversations, conversation)
		}
	}
	return conversations, omitted, truncated
}

func compactTurn(turn evidence.ChatTurnSummary, maxChars int) CompactedTurn {
	compacted := CompactedTurn{
		Ref:              turn.Ref,
		ObservedAt:       turn.ObservedAt,
		TaskAtStart:      turn.TaskAtStart,
		Finalized:        turn.Finalized,
		UserContent:      turn.UserContent,
		AssistantContent: turn.AssistantContent,
	}
	return fitTurn(compacted, maxChars)
}

func fitConversation(turns []CompactedTurn, maxChars int) ([]CompactedTurn, int, bool) {
	if len(turns) == 0 {
		return nil, 0, false
	}
	if maxChars <= 0 {
		return nil, len(turns), true
	}
	total := 0
	for _, turn := range turns {
		total += runeLen(turn.UserContent) + runeLen(turn.AssistantContent)
	}
	if total <= maxChars {
		return turns, 0, false
	}

	// Take head and tail turns first, then fill inward. This retains the
	// opening problem and the latest progress instead of truncating from one
	// side only.
	selected := make(map[int]CompactedTurn)
	left, right, remaining := 0, len(turns)-1, maxChars
	for left <= right && remaining > 0 {
		if left == right {
			selected[left] = fitTurn(turns[left], remaining)
			remaining -= runeLen(selected[left].UserContent) + runeLen(selected[left].AssistantContent)
			left++
			continue
		}
		head := fitTurn(turns[left], remaining)
		selected[left] = head
		remaining -= runeLen(head.UserContent) + runeLen(head.AssistantContent)
		left++
		if remaining <= 0 {
			break
		}
		tail := fitTurn(turns[right], remaining)
		selected[right] = tail
		remaining -= runeLen(tail.UserContent) + runeLen(tail.AssistantContent)
		right--
	}
	indices := make([]int, 0, len(selected))
	for index := range selected {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	out := make([]CompactedTurn, 0, len(indices))
	for _, index := range indices {
		out = append(out, selected[index])
	}
	return out, len(turns) - len(out), true
}

func fitTurn(turn CompactedTurn, maxChars int) CompactedTurn {
	if maxChars <= 0 {
		turn.UserContent = ""
		turn.AssistantContent = ""
		turn.Truncated = true
		return turn
	}
	originalUser, originalAssistant := turn.UserContent, turn.AssistantContent
	userChars := runeLen(originalUser)
	if userChars >= maxChars {
		turn.UserContent = headTail(originalUser, maxChars)
		turn.AssistantContent = ""
	} else {
		turn.UserContent = originalUser
		turn.AssistantContent = headTail(originalAssistant, maxChars-userChars)
	}
	turn.Truncated = turn.Truncated || turn.UserContent != originalUser || turn.AssistantContent != originalAssistant
	return turn
}

func headTail(value string, maxChars int) string {
	runes := []rune(value)
	if maxChars <= 0 {
		return ""
	}
	if len(runes) <= maxChars {
		return value
	}
	if maxChars == 1 {
		return "…"
	}
	head := (maxChars - 1 + 1) / 2
	tail := maxChars - 1 - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

func runeLen(value string) int { return len([]rune(value)) }

func boundFixedEvidence(input *ReviewInput) {
	for index := range input.Sessions {
		input.Sessions[index].Task = headTail(input.Sessions[index].Task, 1024)
	}
	for index := range input.Distractions {
		input.Distractions[index].App = headTail(input.Distractions[index].App, 256)
		input.Distractions[index].Title = headTail(input.Distractions[index].Title, 1024)
		input.Distractions[index].Domain = headTail(input.Distractions[index].Domain, 512)
		input.Distractions[index].Task = headTail(input.Distractions[index].Task, 1024)
	}
	for index := range input.Reminders {
		input.Reminders[index].Message = headTail(input.Reminders[index].Message, 1024)
	}
	for index := range input.Semantic {
		input.Semantic[index].Task = headTail(input.Semantic[index].Task, 1024)
		input.Semantic[index].App = headTail(input.Semantic[index].App, 256)
		input.Semantic[index].Title = headTail(input.Semantic[index].Title, 1024)
		input.Semantic[index].Domain = headTail(input.Semantic[index].Domain, 512)
		input.Semantic[index].SourceKind = headTail(input.Semantic[index].SourceKind, 128)
	}
	for index := range input.Warnings {
		input.Warnings[index] = headTail(input.Warnings[index], 1024)
	}
}

func encodedRuneLen(input ReviewInput) (int, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	return runeLen(string(encoded)), nil
}

func enforceFinalBudget(input *ReviewInput, maxChars int) error {
	encoded, err := encodedRuneLen(*input)
	if err != nil {
		return err
	}
	if encoded <= maxChars {
		return nil
	}

	// Chat is the first variable budget. Binary search over content characters
	// keeps the result deterministic while accounting for JSON field overhead.
	full := append([]CompactedConversation(nil), input.ChatConversations...)
	totalChatChars := 0
	for _, conversation := range full {
		for _, turn := range conversation.Turns {
			totalChatChars += runeLen(turn.UserContent) + runeLen(turn.AssistantContent)
		}
	}
	best := *input
	best.ChatConversations = nil
	best.OmittedTurnCount += countConversationTurns(full)
	best.Truncated = true
	low, high := 0, totalChatChars
	for low <= high {
		middle := low + (high-low)/2
		candidate := *input
		candidate.ChatConversations, candidate.OmittedTurnCount = fitConversationsGlobal(full, middle, input.OmittedTurnCount)
		candidate.Truncated = true
		candidateSize, sizeErr := encodedRuneLen(candidate)
		if sizeErr != nil {
			return sizeErr
		}
		if candidateSize <= maxChars {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	*input = best
	encoded, err = encodedRuneLen(*input)
	if err != nil {
		return err
	}
	if encoded <= maxChars {
		return nil
	}

	// Fixed evidence arrays are also bounded projections. Preserve their
	// chronological head/tail while removing the middle until the final JSON
	// budget is met. This path is deterministic and reports every omission.
	for encoded > maxChars {
		if !dropOneFixedEvidence(input) {
			break
		}
		encoded, err = encodedRuneLen(*input)
		if err != nil {
			return err
		}
	}
	if encoded > maxChars {
		return fmt.Errorf("review input exceeds max_final_input_chars=%d after bounded projection", maxChars)
	}
	input.Truncated = input.Truncated || input.OmittedEvidenceCount > 0
	return nil
}

func fitConversationsGlobal(full []CompactedConversation, maxChars, omittedBase int) ([]CompactedConversation, int) {
	if maxChars <= 0 {
		return nil, omittedBase + countConversationTurns(full)
	}
	remaining := maxChars
	out := make([]CompactedConversation, 0, len(full))
	omitted := omittedBase
	for conversationIndex, conversation := range full {
		turns, dropped, _ := fitConversation(conversation.Turns, remaining)
		if len(turns) == 0 {
			omitted += len(conversation.Turns)
			continue
		}
		copyConversation := conversation
		copyConversation.Turns = turns
		out = append(out, copyConversation)
		omitted += dropped
		for _, turn := range turns {
			remaining -= runeLen(turn.UserContent) + runeLen(turn.AssistantContent)
		}
		if remaining <= 0 {
			for _, rest := range full[conversationIndex+1:] {
				omitted += len(rest.Turns)
			}
			break
		}
	}
	return out, omitted
}

func countConversationTurns(conversations []CompactedConversation) int {
	count := 0
	for _, conversation := range conversations {
		count += len(conversation.Turns)
	}
	return count
}

func dropOneFixedEvidence(input *ReviewInput) bool {
	type collection struct {
		length int
		drop   func(int)
	}
	collections := []collection{
		{len(input.Semantic), func(index int) { input.Semantic = append(input.Semantic[:index], input.Semantic[index+1:]...) }},
		{len(input.Sessions), func(index int) { input.Sessions = append(input.Sessions[:index], input.Sessions[index+1:]...) }},
		{len(input.Distractions), func(index int) {
			input.Distractions = append(input.Distractions[:index], input.Distractions[index+1:]...)
		}},
		{len(input.Reminders), func(index int) { input.Reminders = append(input.Reminders[:index], input.Reminders[index+1:]...) }},
	}
	for _, item := range collections {
		if item.length > 0 {
			item.drop(item.length / 2)
			input.OmittedEvidenceCount++
			return true
		}
	}
	if len(input.Warnings) > 0 {
		index := len(input.Warnings) / 2
		input.Warnings = append(input.Warnings[:index], input.Warnings[index+1:]...)
		input.OmittedEvidenceCount++
		return true
	}
	return false
}
