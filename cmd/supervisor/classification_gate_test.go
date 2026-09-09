package main

import (
	"study-guardian/internal/state"
	"testing"
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
