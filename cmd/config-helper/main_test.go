package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMigrateLegacyNodeKeepsUnrelatedConfig(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("standby:\n  first_study_active_minutes: 5\nai:\n  enabled: true\n  provider: deepseek\n  model: deepseek-chat\n"), &doc); err != nil {
		t.Fatal(err)
	}
	ai := ensureMap(doc.Content[0], "ai")
	migrateLegacy(ai)
	text := ensureMap(ai, "text")
	if value(text, "provider").Value != "deepseek" || value(text, "model").Value != "deepseek-chat" {
		t.Fatalf("legacy fields were not migrated: %#v", text)
	}
	if value(doc.Content[0], "standby") == nil || value(ai, "schema_version").Value != "2" {
		t.Fatal("unrelated config or schema version was lost")
	}
}
