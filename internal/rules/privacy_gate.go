package rules

import (
	"strings"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

type PrivacyGate struct {
	sensitiveApps    []string
	sensitiveDomains []string
}

func NewPrivacyGate(cfg *config.Config) *PrivacyGate {
	apps := make([]string, 0)
	domains := make([]string, 0)
	if cfg != nil {
		for _, a := range cfg.Privacy.SensitiveApps {
			apps = append(apps, strings.ToLower(strings.TrimSpace(a)))
		}
		for _, d := range cfg.Privacy.SensitiveDomains {
			domains = append(domains, strings.ToLower(strings.TrimSpace(d)))
		}
	}
	return &PrivacyGate{
		sensitiveApps:    apps,
		sensitiveDomains: domains,
	}
}

func (pg *PrivacyGate) Evaluate(app, title, domain string) state.PrivacyState {
	appLower := strings.ToLower(app)
	titleLower := strings.ToLower(title)
	domainLower := strings.ToLower(domain)

	// Check apps
	for _, sApp := range pg.sensitiveApps {
		if sApp != "" && (strings.Contains(appLower, sApp) || strings.Contains(titleLower, sApp)) {
			return state.PrivacySensitive
		}
	}

	// Check domains
	for _, sDomain := range pg.sensitiveDomains {
		if sDomain != "" && (strings.Contains(domainLower, sDomain) || strings.Contains(titleLower, sDomain)) {
			return state.PrivacySensitive
		}
	}

	return state.PrivacyNormal
}
