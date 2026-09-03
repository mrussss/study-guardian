package review

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrInvalidReviewDocument = errors.New("invalid daily review document")

type ValidationReport struct {
	Warnings                    []string `json:"warnings"`
	StrippedTopicCount          int      `json:"stripped_topic_count"`
	StrippedAccomplishmentCount int      `json:"stripped_accomplishment_count"`
	InvalidReferenceCount       int      `json:"invalid_reference_count"`
}

type evidenceRef struct {
	kind              string
	completionCapable bool
}

type evidenceIndex map[string]evidenceRef

// ValidateDocument applies deterministic evidence rules to an AI-produced
// document. It never invents replacement refs and never mutates the input or
// the provider's document.
func ValidateDocument(input ReviewInput, document Document) (Document, ValidationReport, error) {
	report := ValidationReport{}
	if document.SchemaVersion != 1 {
		return Document{}, report, fmt.Errorf("%w: schema_version=%d", ErrInvalidReviewDocument, document.SchemaVersion)
	}
	index := buildEvidenceIndex(input)
	validated := cloneDocument(document)
	if validated.Date != input.Date {
		validated.Date = input.Date
		report.Warnings = append(report.Warnings, "document date did not match ReviewInput and was normalized")
	}
	validated.Topics = validateTopics(validated.Topics, index, &report)
	validated.Accomplishments = validateAccomplishments(validated.Accomplishments, index, &report)
	validated.Behavior = behaviorFromEvidence(input, validated.Behavior, &report)
	return validated, report, nil
}

func buildEvidenceIndex(input ReviewInput) evidenceIndex {
	index := make(evidenceIndex)
	add := func(ref, kind string, completionCapable bool) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		if _, exists := index[ref]; !exists {
			index[ref] = evidenceRef{kind: kind, completionCapable: completionCapable}
		}
	}
	for _, item := range input.Sessions {
		add(item.Ref, "session", false)
	}
	for _, item := range input.Distractions {
		add(item.Ref, "distraction", false)
	}
	for _, item := range input.Reminders {
		add(item.Ref, "reminder", false)
	}
	for _, item := range input.Semantic {
		add(item.Ref, "semantic", false)
	}
	for _, conversation := range input.ChatConversations {
		for _, turn := range conversation.Turns {
			add(turn.Ref, "chat_turn", false)
		}
	}
	return index
}

func validateTopics(topics []Topic, index evidenceIndex, report *ValidationReport) []Topic {
	out := make([]Topic, 0, len(topics))
	for _, topic := range topics {
		if !validConfidence(topic.Confidence) {
			report.StrippedTopicCount++
			report.Warnings = append(report.Warnings, "a topic with invalid confidence was removed")
			continue
		}
		refs := validRefs(topic.EvidenceRefs, index, report)
		if len(refs) == 0 {
			report.StrippedTopicCount++
			report.Warnings = append(report.Warnings, "a topic without valid evidence refs was removed")
			continue
		}
		topic.EvidenceRefs = refs
		out = append(out, topic)
	}
	return out
}

func validateAccomplishments(accomplishments []Accomplishment, index evidenceIndex, report *ValidationReport) []Accomplishment {
	out := make([]Accomplishment, 0, len(accomplishments))
	for _, accomplishment := range accomplishments {
		if !validConfidence(accomplishment.Confidence) {
			report.StrippedAccomplishmentCount++
			report.Warnings = append(report.Warnings, "an accomplishment with invalid confidence was removed")
			continue
		}
		refs := validRefs(accomplishment.EvidenceRefs, index, report)
		if len(refs) == 0 {
			report.StrippedAccomplishmentCount++
			report.Warnings = append(report.Warnings, "an accomplishment without valid evidence refs was removed")
			continue
		}
		strong := false
		for _, ref := range refs {
			if index[ref].completionCapable {
				strong = true
				break
			}
		}
		if !strong {
			report.StrippedAccomplishmentCount++
			report.Warnings = append(report.Warnings, "chat and semantic evidence cannot prove an accomplishment; claim removed")
			continue
		}
		accomplishment.EvidenceRefs = refs
		out = append(out, accomplishment)
	}
	return out
}

func validRefs(refs []string, index evidenceIndex, report *ValidationReport) []string {
	valid := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			report.InvalidReferenceCount++
			continue
		}
		if _, ok := index[ref]; !ok {
			report.InvalidReferenceCount++
			continue
		}
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		valid = append(valid, ref)
	}
	return valid
}

func validConfidence(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func behaviorFromEvidence(input ReviewInput, behavior Behavior, report *ValidationReport) Behavior {
	wantCount := len(input.Distractions)
	var wantLargest int64
	for _, distraction := range input.Distractions {
		if distraction.DurationSeconds > wantLargest {
			wantLargest = distraction.DurationSeconds
		}
	}
	if behavior.DistractionCount != wantCount || behavior.LargestDistractionSec != wantLargest {
		report.Warnings = append(report.Warnings, "behavior distraction metrics were normalized from evidence")
	}
	if behavior.AverageRecoverySec < 0 {
		behavior.AverageRecoverySec = 0
		report.Warnings = append(report.Warnings, "negative recovery metric was normalized to zero")
	}
	behavior.DistractionCount = wantCount
	behavior.LargestDistractionSec = wantLargest
	return behavior
}

func cloneDocument(document Document) Document {
	cloned := document
	cloned.Topics = append([]Topic(nil), document.Topics...)
	for index := range cloned.Topics {
		cloned.Topics[index].EvidenceRefs = append([]string(nil), document.Topics[index].EvidenceRefs...)
	}
	cloned.Accomplishments = append([]Accomplishment(nil), document.Accomplishments...)
	for index := range cloned.Accomplishments {
		cloned.Accomplishments[index].EvidenceRefs = append([]string(nil), document.Accomplishments[index].EvidenceRefs...)
	}
	cloned.Unfinished = append([]string(nil), document.Unfinished...)
	cloned.Difficulties = append([]string(nil), document.Difficulties...)
	return cloned
}
