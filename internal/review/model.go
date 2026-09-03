package review

import "study-guardian/internal/evidence"

type Document struct {
	SchemaVersion    int                      `json:"schema_version"`
	Date             string                   `json:"date"`
	Headline         string                   `json:"headline"`
	Topics           []Topic                  `json:"topics"`
	Accomplishments  []Accomplishment         `json:"accomplishments"`
	Unfinished       []string                 `json:"unfinished"`
	Difficulties     []string                 `json:"difficulties"`
	Behavior         Behavior                 `json:"behavior"`
	TomorrowPriority string                   `json:"tomorrow_priority"`
	EvidenceQuality  evidence.EvidenceQuality `json:"evidence_quality"`
	Warnings         []string                 `json:"warnings"`
}

type Topic struct {
	Name         string   `json:"name"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs"`
	Confidence   float64  `json:"confidence"`
}

type Accomplishment struct {
	Text         string   `json:"text"`
	EvidenceRefs []string `json:"evidence_refs"`
	Confidence   float64  `json:"confidence"`
}

type Behavior struct {
	DistractionCount      int   `json:"distraction_count"`
	LargestDistractionSec int64 `json:"largest_distraction_seconds"`
	AverageRecoverySec    int64 `json:"average_recovery_seconds"`
}
