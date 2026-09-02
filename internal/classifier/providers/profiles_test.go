package providers

import (
	"testing"

	"study-guardian/internal/config"
)

func TestRequiredProviderProfiles(t *testing.T) {
	cases := map[string]string{
		"openai":      "https://api.openai.com/v1",
		"deepseek":    "https://api.deepseek.com",
		"kimi":        "https://api.moonshot.cn/v1",
		"zhipu":       "https://open.bigmodel.cn/api/paas/v4",
		"siliconflow": "https://api.siliconflow.cn/v1",
		"doubao":      "https://ark.cn-beijing.volces.com/api/v3",
		"ollama":      "http://127.0.0.1:11434/v1",
	}
	for id, want := range cases {
		p, ok := ProfileFor(id)
		if !ok || p.DefaultBaseURL != want {
			t.Fatalf("profile %s = %#v, want base URL %s", id, p, want)
		}
	}
	if _, ok := ProfileFor("not-a-provider"); ok {
		t.Fatal("unknown provider must not be accepted")
	}
}

func TestVisionProviderCanBeConfiguredIndependently(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Text.Provider = "none"
	cfg.AI.Vision.Enabled = true
	cfg.AI.Vision.Provider = "ollama"
	cfg.AI.Vision.Model = "llava"
	r := New(cfg)
	if r.Provider() != nil {
		t.Fatal("text provider should remain disabled")
	}
	if r.VisionProvider() == nil {
		t.Fatal("vision provider should be configurable independently")
	}
}
