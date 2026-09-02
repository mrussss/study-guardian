package classifier

import (
	"context"
	"testing"

	"study-guardian/internal/config"
	"study-guardian/internal/rules"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func TestClassificationService(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "fake"
	ruleEngine := rules.NewRuleEngine()
	privacyGate := rules.NewPrivacyGate(cfg)
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()

	fakeProvider := NewFakeProvider()
	service := NewService(cfg, ruleEngine, privacyGate, fakeProvider, store)

	// 1. Rules first: Steam is DISTRACTED without calling AI
	res := service.Classify(ctx, "Steam.exe", "Steam Store", "store.steampowered.com", "Math Homework", "hash1", "STUDY", "")
	if res.Relation != state.RelationDistracted || !res.IsFromRule {
		t.Fatalf("expected rule-based DISTRACTED, got %+v", res)
	}

	// 2. Ambiguous content (no task keyword in title): calls Fake AI provider
	res = service.Classify(ctx, "chrome.exe", "Video Tutorial #42", "bilibili.com", "Go Lab", "hash2", "STUDY", "")
	if res.Relation != state.RelationFocused || res.IsFromRule {
		t.Fatalf("expected AI-classified FOCUSED, got %+v", res)
	}

	// 3. Cache verification: second call should be cached
	resCached := service.Classify(ctx, "chrome.exe", "Video Tutorial #42", "bilibili.com", "Go Lab", "hash2", "STUDY", "")
	if resCached.Relation != state.RelationFocused {
		t.Fatalf("expected cached FOCUSED, got %+v", resCached)
	}

	// 4. Privacy Gate: Sensitive password manager window
	res = service.Classify(ctx, "Bitwarden.exe", "Bitwarden Vault", "", "Go Lab", "hash3", "STUDY", "")
	if res.Reason != "Privacy Gate flagged sensitive window; AI analysis bypassed" {
		t.Fatalf("expected privacy gate bypass, got reason: %s", res.Reason)
	}

	// 5. Fail-soft when AI provider fails
	fakeProvider.ShouldError = true
	res = service.Classify(ctx, "chrome.exe", "Unknown Unmatched Page", "example.com", "Go Lab", "hash4", "STUDY", "")
	if res.Relation != state.RelationUnknown {
		t.Fatalf("expected fallback UNKNOWN when AI fails, got %s", res.Relation)
	}
}

func TestClassificationCacheKeySeparatesProviderAndVision(t *testing.T) {
	textA := computeCacheKey("deepseek (chat)", "chrome.exe", "Lecture", "example.com", "Go", "screen-a", false)
	textB := computeCacheKey("deepseek (chat)", "chrome.exe", "Lecture", "example.com", "Go", "screen-b", false)
	if textA != textB {
		t.Fatal("text cache key must ignore screen hash")
	}
	visionA := computeCacheKey("deepseek (chat)", "chrome.exe", "Lecture", "example.com", "Go", "screen-a", true)
	visionB := computeCacheKey("deepseek (chat)", "chrome.exe", "Lecture", "example.com", "Go", "screen-b", true)
	if visionA == visionB {
		t.Fatal("vision cache key must include screen hash")
	}
	otherProvider := computeCacheKey("qwen (qwen-plus)", "chrome.exe", "Lecture", "example.com", "Go", "screen-a", false)
	if textA == otherProvider {
		t.Fatal("cache key must include provider/model identity")
	}
}

func TestClassificationUsesIndependentVisionProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.UseVisionOnlyWhenNeeded = true
	ruleEngine := rules.NewRuleEngine()
	privacyGate := rules.NewPrivacyGate(cfg)
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := NewServiceWithProviders(cfg, ruleEngine, privacyGate, nil, NewFakeProvider(), store)
	res := service.Classify(context.Background(), "chrome.exe", "Unmatched page", "example.com", "Go Lab", "screen", "STUDY", "base64-image")
	if res.Relation != state.RelationFocused || res.IsFromRule {
		t.Fatalf("expected independent vision provider result, got %+v", res)
	}
}
