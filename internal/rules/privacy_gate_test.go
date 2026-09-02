package rules

import (
	"testing"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

func TestPrivacyGate(t *testing.T) {
	cfg := config.DefaultConfig()
	gate := NewPrivacyGate(cfg)

	// Normal developer apps
	if gate.Evaluate("Code.exe", "main.go - study-guardian", "") != state.PrivacyNormal {
		t.Fatalf("expected NORMAL for VS Code")
	}

	if gate.Evaluate("chrome.exe", "Go Documentation - pkg.go.dev", "pkg.go.dev") != state.PrivacyNormal {
		t.Fatalf("expected NORMAL for pkg.go.dev")
	}

	// Sensitive password managers
	if gate.Evaluate("Bitwarden.exe", "Bitwarden Vault", "") != state.PrivacySensitive {
		t.Fatalf("expected SENSITIVE for Bitwarden")
	}

	if gate.Evaluate("chrome.exe", "1Password Login", "1password.com") != state.PrivacySensitive {
		t.Fatalf("expected SENSITIVE for 1Password")
	}

	// Sensitive banking/auth
	if gate.Evaluate("chrome.exe", "Google Accounts Login", "accounts.google.com") != state.PrivacySensitive {
		t.Fatalf("expected SENSITIVE for accounts.google.com")
	}
}
