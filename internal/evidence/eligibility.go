package evidence

import (
	"strings"

	"study-guardian/internal/state"
)

// IsLearningSemantic reports whether a semantic snapshot is strong enough to
// support a learning topic. Behavior-only or unknown snapshots remain useful
// for behavior context but must not be presented as learning evidence.
func IsLearningSemantic(item SemanticSummary) bool {
	if strings.ToUpper(strings.TrimSpace(item.Relation)) != string(state.RelationFocused) || item.Confidence < 0.6 {
		return false
	}
	if item.Privacy != "" && strings.ToUpper(strings.TrimSpace(item.Privacy)) != string(state.PrivacyNormal) {
		return false
	}
	return state.IsExplicitStudyActivity(item.Activity)
}
