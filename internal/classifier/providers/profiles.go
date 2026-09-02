package providers

import (
	"net/url"
	"os"
	"strings"
	"time"

	"study-guardian/internal/classifier"
	"study-guardian/internal/config"
)

type Profile struct {
	ID               string
	DefaultBaseURL   string
	DefaultAPIKeyEnv string
	SupportsJSONMode bool
	SupportsVision   bool
}

var profiles = map[string]Profile{
	"none":              {ID: "none"},
	"openai":            {ID: "openai", DefaultBaseURL: "https://api.openai.com/v1", DefaultAPIKeyEnv: "OPENAI_API_KEY", SupportsJSONMode: true},
	"openai-compatible": {ID: "openai-compatible", SupportsJSONMode: false},
	"deepseek":          {ID: "deepseek", DefaultBaseURL: "https://api.deepseek.com", DefaultAPIKeyEnv: "DEEPSEEK_API_KEY", SupportsJSONMode: true},
	"qwen":              {ID: "qwen", DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", DefaultAPIKeyEnv: "DASHSCOPE_API_KEY", SupportsJSONMode: true},
	"kimi":              {ID: "kimi", DefaultBaseURL: "https://api.moonshot.cn/v1", DefaultAPIKeyEnv: "MOONSHOT_API_KEY", SupportsJSONMode: true},
	"zhipu":             {ID: "zhipu", DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultAPIKeyEnv: "ZAI_API_KEY", SupportsJSONMode: true},
	"siliconflow":       {ID: "siliconflow", DefaultBaseURL: "https://api.siliconflow.cn/v1", DefaultAPIKeyEnv: "SILICONFLOW_API_KEY", SupportsJSONMode: false},
	"doubao":            {ID: "doubao", DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", DefaultAPIKeyEnv: "ARK_API_KEY", SupportsJSONMode: true},
	"ollama":            {ID: "ollama", DefaultBaseURL: "http://127.0.0.1:11434/v1", SupportsJSONMode: true},
	"fake":              {ID: "fake"},
}

func ProfileFor(id string) (Profile, bool) {
	p, ok := profiles[strings.ToLower(strings.TrimSpace(id))]
	return p, ok
}
func AllProfiles() []Profile {
	out := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p)
	}
	return out
}

type Status struct {
	Enabled        bool       `json:"enabled"`
	TextProvider   string     `json:"text_provider"`
	TextModel      string     `json:"text_model"`
	TextConfigured bool       `json:"text_configured"`
	VisionEnabled  bool       `json:"vision_enabled"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	CooldownUntil  *time.Time `json:"cooldown_until,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	Warning        string     `json:"warning,omitempty"`
}

type Registry struct {
	cfg           *config.Config
	provider      classifier.TaskRelationProvider
	vision        classifier.TaskRelationProvider
	status        Status
	lastSuccess   *time.Time
	cooldown      func() time.Time
	lastError     func() string
	lastSuccessAt func() time.Time
}

func New(cfg *config.Config) *Registry {
	text := cfg.AI.Text
	st := Status{Enabled: cfg.AI.Enabled, TextProvider: text.Provider, TextModel: text.Model, VisionEnabled: cfg.AI.Vision.Enabled, Warning: cfg.AI.MigrationWarning}
	r := &Registry{cfg: cfg, status: st}
	if !cfg.AI.Enabled {
		return r
	}

	if p, ok := ProfileFor(text.Provider); ok && text.Provider != "none" {
		if text.Provider == "fake" {
			if cfg.AI.DeveloperMode {
				st.TextConfigured = true
				r.provider = classifier.NewFakeProvider()
			} else {
				st.Enabled = false
				st.Warning = "fake provider disabled outside developer_mode; rules only"
			}
		} else {
			key := resolveKey(text, p.DefaultAPIKeyEnv)
			// Legacy api_key is read only for compatibility; migration scripts never
			// create this field and new configs should use env/file references.
			if key == "" {
				key = strings.TrimSpace(cfg.AI.APIKey)
			}
			endpoint := text.BaseURL
			if endpoint == "" {
				endpoint = p.DefaultBaseURL
			}
			if endpoint == "" && text.Provider != "ollama" {
				st.Warning = "provider base_url is required"
			} else {
				st.TextConfigured = key != "" || text.Provider == "ollama" || localEndpoint(endpoint)
				if st.TextConfigured {
					r.provider = classifier.NewOpenAICompatibleProviderWithOptions(classifier.ProviderOptions{Endpoint: endpoint, APIKey: key, Model: text.Model, JSONMode: text.JSONMode, SupportsJSONMode: p.SupportsJSONMode, Timeout: time.Duration(text.TimeoutSeconds) * time.Second, Temperature: text.Temperature})
				} else {
					st.Warning = "API key is not configured; rules only"
				}
			}
		}
	} else if text.Provider != "" && text.Provider != "none" {
		st.Warning = "unknown AI provider; rules only"
	}
	if provider, ok := r.provider.(*classifier.OpenAICompatibleProvider); ok {
		r.cooldown = provider.CooldownUntil
		r.lastError = provider.LastError
		r.lastSuccessAt = provider.LastSuccessAt
	}

	vision := cfg.AI.Vision
	if cfg.AI.Vision.Enabled && vision.Provider != "" && vision.Provider != "none" {
		if vp, exists := ProfileFor(vision.Provider); exists {
			visionEndpoint := vision.BaseURL
			if visionEndpoint == "" {
				visionEndpoint = vp.DefaultBaseURL
			}
			visionKey := resolveKey(vision, vp.DefaultAPIKeyEnv)
			if visionKey != "" || vision.Provider == "ollama" || localEndpoint(visionEndpoint) {
				r.vision = classifier.NewOpenAICompatibleProviderWithOptions(classifier.ProviderOptions{Endpoint: visionEndpoint, APIKey: visionKey, Model: vision.Model, JSONMode: vision.JSONMode, SupportsJSONMode: vp.SupportsJSONMode, Timeout: time.Duration(vision.TimeoutSeconds) * time.Second, Temperature: vision.Temperature})
			}
		}
	}
	r.status = st
	return r
}

func localEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func resolveKey(c config.AIEndpointConfig, fallback string) string {
	if c.APIKeyEnv != "" {
		if v := os.Getenv(c.APIKeyEnv); v != "" {
			return strings.TrimSpace(v)
		}
	}
	if fallback != "" {
		if v := os.Getenv(fallback); v != "" {
			return strings.TrimSpace(v)
		}
	}
	if c.APIKeyFile != "" {
		if b, err := os.ReadFile(c.APIKeyFile); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
func (r *Registry) Provider() classifier.TaskRelationProvider       { return r.provider }
func (r *Registry) VisionProvider() classifier.TaskRelationProvider { return r.vision }
func (r *Registry) Status() Status {
	st := r.status
	if r.provider != nil {
		if u := r.cooldown(); !u.IsZero() {
			st.CooldownUntil = &u
		}
	}
	if r.lastSuccess != nil {
		t := *r.lastSuccess
		st.LastSuccessAt = &t
	}
	if r.lastError != nil {
		st.LastError = r.lastError()
	}
	if r.lastSuccessAt != nil {
		if t := r.lastSuccessAt(); !t.IsZero() {
			st.LastSuccessAt = &t
		}
	}
	return st
}
