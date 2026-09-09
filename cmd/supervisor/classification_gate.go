package main

import "study-guardian/internal/state"

// observationUnavailable means the local observation itself is not safe to
// interpret. These states must produce UNKNOWN and must never fall through to
// either the local rule engine or a remote provider.
func observationUnavailable(isAFK, isLocked, activityWatchOK bool, privacy state.PrivacyState) bool {
	return isAFK || isLocked || !activityWatchOK || privacy == state.PrivacySensitive
}

// remoteAIAllowed deliberately does not imply that local rules are allowed.
// BREAK is the important split: it cannot call Text/Vision, but it can use the
// local rule engine to accumulate an automatic-resume focus interval.
func remoteAIAllowed(mode state.UserMode, isAFK, isLocked, activityWatchOK bool, privacy state.PrivacyState) bool {
	return mode != state.UserModeOff && mode != state.UserModeBreak &&
		!observationUnavailable(isAFK, isLocked, activityWatchOK, privacy)
}

func shouldSkipAI(mode state.UserMode, isAFK, isLocked, activityWatchOK bool, privacy state.PrivacyState) bool {
	return !remoteAIAllowed(mode, isAFK, isLocked, activityWatchOK, privacy)
}

// classifyObservation is the supervisor's single classification decision. The
// callbacks make the ordering testable without a network provider or a real
// screenshot: BREAK invokes only local, while healthy STUDY invokes remote.
func classifyObservation(mode state.UserMode, isAFK, isLocked, activityWatchOK bool, privacy state.PrivacyState, local, remote func() state.ClassificationResult) state.ClassificationResult {
	if mode == state.UserModeOff {
		return unavailableClassification("System is OFF")
	}
	if observationUnavailable(isAFK, isLocked, activityWatchOK, privacy) {
		reason := "ActivityWatch unavailable or stale"
		switch {
		case isLocked:
			reason = "locked; AI skipped"
		case isAFK:
			reason = "AFK; AI skipped"
		case privacy == state.PrivacySensitive:
			reason = "sensitive privacy state"
		}
		return unavailableClassification(reason)
	}
	if mode == state.UserModeBreak {
		if local == nil {
			return unavailableClassification("BREAK mode; local rule unavailable")
		}
		return local()
	}
	if remote == nil {
		return unavailableClassification("remote classification unavailable")
	}
	return remote()
}

func unavailableClassification(reason string) state.ClassificationResult {
	return state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: reason, SourceKind: state.SourceKindLocalRule, IsFromRule: true}
}
