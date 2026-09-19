package automation

import "testing"

func TestNormalizePersistedSettingsDefaultsOnlyMissingAutoStartFields(t *testing.T) {
	legacy := Settings{}
	legacy.AutoStart.Enabled = true
	legacy.AutoStart.MinConfidence = .8
	legacy.AutoStart.AllowUnclassified = true
	legacy.AutoPause.IdleStaticSeconds = 300
	legacy.AutoPause.IdleDynamicSeconds = 900
	legacy.AutoPause.LockedSeconds = 15
	legacy.AutoResume.FocusedStableSeconds = 45
	got := NormalizePersistedSettings(legacy, "{\"auto_start\":{\"enabled\":true,\"min_confidence\":0.8,\"allow_unclassified\":true}}")
	if got.AutoStart.FocusedStableSeconds != 90 || got.AutoStart.UnclassifiedStableSeconds != 180 || got.AutoStart.EvidenceGraceSeconds != 20 {
		t.Fatalf("legacy defaults not applied: %+v", got.AutoStart)
	}

	current := NormalizePersistedSettings(legacy, "{\"auto_start\":{\"focused_stable_seconds\":90,\"unclassified_stable_seconds\":180,\"evidence_grace_seconds\":0}}")
	if current.AutoStart.EvidenceGraceSeconds != 0 {
		t.Fatalf("explicit zero grace was changed: %d", current.AutoStart.EvidenceGraceSeconds)
	}
}
