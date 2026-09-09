package main

import "study-guardian/internal/state"

func shouldSkipAI(mode state.UserMode, isAFK, isLocked, activityWatchOK bool, privacy state.PrivacyState) bool {
	return isAFK || isLocked || mode == state.UserModeOff || mode == state.UserModeBreak ||
		privacy == state.PrivacySensitive || !activityWatchOK
}
