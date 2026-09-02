package rules

import (
	"strings"

	"study-guardian/internal/state"
)

type RuleEngine struct {
	distractionApps    []string
	distractionDomains []string
	devApps            []string
	devDomains         []string
}

func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		distractionApps: []string{
			"steam", "epicgames", "genshinimpact", "honkaistarrail", "leagueclient", "riotclient",
			"valorant", "csgo", "dota2", "minecraft", "overwatch",
		},
		distractionDomains: []string{
			"store.steampowered.com", "steamcommunity.com", "epicgames.com", "qidian.com",
			"jjwxc.net", "biquge", "novel", "mangadex.org", "douyu.com", "huya.com", "twitch.tv",
		},
		devApps: []string{
			"code", "goland", "pycharm", "clion", "idea", "webstorm", "cursor", "devenv",
			"windowsterminal", "powershell", "cmd", "bash", "git", "postman", "datagrip",
		},
		devDomains: []string{
			"github.com", "gitlab.com", "stackoverflow.com", "pkg.go.dev", "go.dev",
			"golang.org", "developer.mozilla.org", "docs.python.org", "learn.microsoft.com",
			"chatgpt.com", "claude.ai", "deepseek.com", "gemini.google.com", "arxiv.org",
		},
	}
}

func (re *RuleEngine) Classify(app, title, domain, task string) state.ClassificationResult {
	appLower := strings.ToLower(app)
	titleLower := strings.ToLower(title)
	domainLower := strings.ToLower(domain)
	taskLower := strings.ToLower(task)

	// 1. Check definite distractions
	for _, dApp := range re.distractionApps {
		if strings.Contains(appLower, dApp) {
			return state.ClassificationResult{
				Relation:   state.RelationDistracted,
				Confidence: 0.95,
				Reason:     "App matches known distraction blacklist: " + dApp,
				IsFromRule: true,
			}
		}
	}

	for _, dDomain := range re.distractionDomains {
		if strings.Contains(domainLower, dDomain) {
			return state.ClassificationResult{
				Relation:   state.RelationDistracted,
				Confidence: 0.90,
				Reason:     "Domain matches known distraction blacklist: " + dDomain,
				IsFromRule: true,
			}
		}
	}

	// 2. Check task keyword matching
	if taskLower != "" {
		keywords := extractKeywords(taskLower)
		matchedCount := 0
		for _, kw := range keywords {
			if len(kw) >= 2 && (strings.Contains(titleLower, kw) || strings.Contains(domainLower, kw)) {
				matchedCount++
			}
		}
		if matchedCount > 0 {
			return state.ClassificationResult{
				Relation:   state.RelationFocused,
				Confidence: 0.85 + float64(matchedCount)*0.05,
				Reason:     "Window title/domain matches current task keywords",
				IsFromRule: true,
			}
		}
	}

	// 3. Check dev environment apps
	for _, devApp := range re.devApps {
		if strings.Contains(appLower, devApp) {
			return state.ClassificationResult{
				Relation:   state.RelationFocused,
				Confidence: 0.85,
				Reason:     "App is a recognized development tool: " + devApp,
				IsFromRule: true,
			}
		}
	}

	// 4. Check dev domains
	for _, devDomain := range re.devDomains {
		if strings.Contains(domainLower, devDomain) {
			return state.ClassificationResult{
				Relation:   state.RelationFocused,
				Confidence: 0.80,
				Reason:     "Domain is a recognized learning/developer resource: " + devDomain,
				IsFromRule: true,
			}
		}
	}

	// 5. Default unknown
	return state.ClassificationResult{
		Relation:   state.RelationUnknown,
		Confidence: 0.50,
		Reason:     "No deterministic rule matched; candidate for AI/OCR analysis",
		IsFromRule: true,
	}
}

func extractKeywords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == ':' || r == '/' || r == '.' || r == ',' || r == '，' || r == '。'
	})
	res := make([]string, 0, len(fields))
	for _, f := range fields {
		fTrim := strings.TrimSpace(f)
		if len(fTrim) >= 2 {
			res = append(res, fTrim)
		}
	}
	return res
}
