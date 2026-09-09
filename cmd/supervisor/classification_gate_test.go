package main

import (
	"testing"

	"study-guardian/internal/state"
)

func TestShouldSkipAIForLocalSafetyStates(t *testing.T) {
	cases := []struct {
		name              string
		mode              state.UserMode
		afk, locked, awOK bool
		privacy           state.PrivacyState
		want              bool
	}{
		{"afk static", state.UserModeStudy, true, false, true, state.PrivacyNormal, true},
		{"afk dynamic", state.UserModeStudy, true, false, true, state.PrivacyNormal, true},
		{"locked", state.UserModeStudy, false, true, true, state.PrivacyNormal, true},
		{"break", state.UserModeBreak, false, false, true, state.PrivacyNormal, true},
		{"off", state.UserModeOff, false, false, true, state.PrivacyNormal, true},
		{"sensitive", state.UserModeStudy, false, false, true, state.PrivacySensitive, true},
		{"activitywatch unavailable", state.UserModeStudy, false, false, false, state.PrivacyNormal, true},
		{"active healthy", state.UserModeStudy, false, false, true, state.PrivacyNormal, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSkipAI(tc.mode, tc.afk, tc.locked, tc.awOK, tc.privacy); got != tc.want {
				t.Fatalf("skip=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestBreakUsesLocalRulesWithoutRemoteAI(t *testing.T) {
	localCalls, remoteCalls := 0, 0
	local := func() state.ClassificationResult {
		localCalls++
		return state.ClassificationResult{Relation: state.RelationFocused, Confidence: .9, SourceKind: state.SourceKindLocalRule, IsFromRule: true}
	}
	remote := func() state.ClassificationResult {
		remoteCalls++
		return state.ClassificationResult{Relation: state.RelationDistracted, Confidence: .9, SourceKind: state.SourceKindTextAI}
	}
	result := classifyObservation(state.UserModeBreak, false, false, true, state.PrivacyNormal, local, remote)
	if result.Relation != state.RelationFocused || localCalls != 1 || remoteCalls != 0 {
		t.Fatalf("result=%+v local=%d remote=%d", result, localCalls, remoteCalls)
	}
	if remoteAIAllowed(state.UserModeBreak, false, false, true, state.PrivacyNormal) {
		t.Fatal("BREAK must not allow remote AI")
	}
	if observationUnavailable(false, false, true, state.PrivacyNormal) {
		t.Fatal("healthy BREAK observations should remain locally usable")
	}
}

func TestUnavailableObservationNeverCallsEitherClassifier(t *testing.T) {
	for _, tc := range []struct {
		name              string
		afk, locked, awOK bool
		privacy           state.PrivacyState
	}{
		{"afk", true, false, true, state.PrivacyNormal},
		{"locked", false, true, true, state.PrivacyNormal},
		{"activitywatch", false, false, false, state.PrivacyNormal},
		{"privacy", false, false, true, state.PrivacySensitive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			result := classifyObservation(state.UserModeBreak, tc.afk, tc.locked, tc.awOK, tc.privacy,
				func() state.ClassificationResult {
					calls++
					return state.ClassificationResult{Relation: state.RelationFocused}
				},
				func() state.ClassificationResult {
					calls++
					return state.ClassificationResult{Relation: state.RelationFocused}
				})
			if calls != 0 || result.Relation != state.RelationUnknown {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
		})
	}
}
