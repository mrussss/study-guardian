package rules

import (
	"testing"

	"study-guardian/internal/state"
)

func TestRuleEngine(t *testing.T) {
	engine := NewRuleEngine()

	// 1. Definite distraction
	res := engine.Classify("Steam.exe", "Steam Store - Winter Sale", "store.steampowered.com", "Go Lab")
	if res.Relation != state.RelationDistracted {
		t.Fatalf("expected DISTRACTED for Steam, got %s", res.Relation)
	}

	// 2. Dev app
	res = engine.Classify("Code.exe", "server.go - study-guardian", "", "Go Lab")
	if res.Relation != state.RelationFocused {
		t.Fatalf("expected FOCUSED for VS Code, got %s", res.Relation)
	}

	// 3. Task matching keyword
	res = engine.Classify("chrome.exe", "Understanding Go Interfaces - Blog", "medium.com", "Go Lab - interface")
	if res.Relation != state.RelationFocused {
		t.Fatalf("expected FOCUSED for keyword match on interface, got %s", res.Relation)
	}

	// 4. Ambiguous / Unknown
	res = engine.Classify("chrome.exe", "Random Video Title", "bilibili.com", "Go Lab")
	if res.Relation != state.RelationUnknown {
		t.Fatalf("expected UNKNOWN for ambiguous bilibili video without keyword match, got %s", res.Relation)
	}
}
