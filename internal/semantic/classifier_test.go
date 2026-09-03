package semantic

import (
	"testing"
	"time"

	"study-guardian/internal/state"
)

func TestClassifyDeterministicActivityMapping(t *testing.T) {
	base := Candidate{
		ObservedAt:  time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC),
		Fresh:       true,
		UserMode:    state.UserModeStudy,
		Task:        "Go project",
		Interaction: state.InteractionActive,
		Relation:    state.RelationFocused,
		Privacy:     state.PrivacyNormal,
	}
	tests := []struct {
		name      string
		candidate Candidate
		want      Activity
	}{
		{"coding", Candidate{App: "Code.exe", Title: "main.go", Domain: ""}, ActivityCoding},
		{"algorithm", Candidate{App: "chrome.exe", Title: "Two Sum - LeetCode", Domain: "leetcode.com"}, ActivityAlgorithm},
		{"reading", Candidate{App: "Acrobat.exe", Title: "chapter.pdf"}, ActivityReading},
		{"writing", Candidate{App: "WINWORD.EXE", Title: "study notes"}, ActivityWriting},
		{"watching", Candidate{App: "chrome.exe", Title: "lesson - YouTube", Domain: "youtube.com"}, ActivityWatching},
		{"ai assisted", Candidate{App: "chrome.exe", Title: "ChatGPT", Domain: "chat.openai.com"}, ActivityAIAssisted},
		{"browsing", Candidate{App: "chrome.exe", Title: "Go documentation", Domain: "example.org"}, ActivityReading},
		{"general study", Candidate{App: "notes.exe", Title: "lecture notes"}, ActivityGeneralStudy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.App, candidate.Title, candidate.Domain = test.candidate.App, test.candidate.Title, test.candidate.Domain
			got, confidence, reason := Classify(candidate)
			if got != test.want || confidence <= 0 || reason == "" {
				t.Fatalf("activity=%q confidence=%v reason=%q, want %q", got, confidence, reason, test.want)
			}
		})
	}
	// The generic browsing row above intentionally exercises the stronger
	// documentation signal; a genuinely generic web page maps to BROWSING.
	got, _, _ := Classify(Candidate{Fresh: true, UserMode: state.UserModeStudy, Task: "Go project", Interaction: state.InteractionActive, Relation: state.RelationFocused, Privacy: state.PrivacyNormal, App: "chrome.exe", Domain: "example.org"})
	if got != ActivityBrowsing {
		t.Fatalf("generic browser activity=%q, want %q", got, ActivityBrowsing)
	}
}

func TestClassifyPreservesActivityRelationOrthogonality(t *testing.T) {
	candidate := Candidate{Fresh: true, UserMode: state.UserModeStudy, Interaction: state.InteractionActive, Relation: state.RelationDistracted, Privacy: state.PrivacyNormal, App: "Code.exe", Title: "main.go"}
	activity, _, _ := Classify(candidate)
	if activity != ActivityCoding {
		t.Fatalf("distracted coding activity=%q, want %q", activity, ActivityCoding)
	}
	if candidate.Relation != state.RelationDistracted {
		t.Fatal("classifier must not rewrite the existing relation")
	}
}

func TestClassifyFallbacksForSafetyAndUnavailableInputs(t *testing.T) {
	base := Candidate{Fresh: true, UserMode: state.UserModeStudy, Interaction: state.InteractionActive, Relation: state.RelationFocused, Privacy: state.PrivacyNormal, App: "Code.exe"}
	for name, candidate := range map[string]Candidate{
		"break":               func() Candidate { c := base; c.UserMode = state.UserModeBreak; return c }(),
		"stale":               func() Candidate { c := base; c.Fresh = false; return c }(),
		"sensitive":           func() Candidate { c := base; c.Privacy = state.PrivacySensitive; return c }(),
		"unknown interaction": func() Candidate { c := base; c.Interaction = state.InteractionUnknown; return c }(),
	} {
		t.Run(name, func(t *testing.T) {
			activity, confidence, _ := Classify(candidate)
			if activity != ActivityUnknown || confidence != 0 {
				t.Fatalf("activity=%q confidence=%v, want UNKNOWN/0", activity, confidence)
			}
		})
	}
}
