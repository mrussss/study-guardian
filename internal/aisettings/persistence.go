package aisettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"study-guardian/internal/config"
)

// persistedAIConfigV2 is the only shape written to the settings table. It is
// deliberately separate from config.AIConfig because that type also carries
// compatibility and runtime-only fields.
type persistedAIConfigV2 struct {
	SchemaVersion           int                 `json:"schema_version"`
	Enabled                 bool                `json:"enabled"`
	DeveloperMode           bool                `json:"developer_mode"`
	UseVisionOnlyWhenNeeded bool                `json:"use_vision_only_when_needed"`
	MinConfidence           float64             `json:"min_confidence"`
	Proxy                   persistedAIProxy    `json:"proxy"`
	Text                    persistedAIEndpoint `json:"text"`
	Vision                  persistedAIEndpoint `json:"vision"`
}

type persistedAIProxy struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

type persistedAIEndpoint struct {
	Enabled        bool     `json:"enabled"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	FallbackModels []string `json:"fallback_models"`
	BaseURL        string   `json:"base_url"`
	APIKeyEnv      string   `json:"api_key_env"`
	APIKeyFile     string   `json:"api_key_file"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	JSONMode       string   `json:"json_mode"`
	Temperature    *float64 `json:"temperature"`
}

// PersistedLoad is the result of decoding either the current canonical shape
// or an older database record. LegacyAPIKey is intentionally kept out of the
// config and is only used for one-time migration into the secret directory.
type PersistedLoad struct {
	Config       config.AIConfig
	LegacyAPIKey string
	NeedsRewrite bool
}

// EncodePersistedAIConfig returns canonical schema-v2 JSON. It never includes
// API keys, migration warnings, or legacy flat fields.
func EncodePersistedAIConfig(value config.AIConfig) ([]byte, error) {
	normalizeEndpoint := func(endpoint config.AIEndpointConfig) persistedAIEndpoint {
		fallbacks := append([]string{}, endpoint.FallbackModels...)
		if fallbacks == nil {
			fallbacks = []string{}
		}
		return persistedAIEndpoint{
			Enabled:        endpoint.Enabled,
			Provider:       endpoint.Provider,
			Model:          endpoint.Model,
			FallbackModels: fallbacks,
			BaseURL:        endpoint.BaseURL,
			APIKeyEnv:      endpoint.APIKeyEnv,
			APIKeyFile:     endpoint.APIKeyFile,
			TimeoutSeconds: endpoint.TimeoutSeconds,
			JSONMode:       endpoint.JSONMode,
			Temperature:    endpoint.Temperature,
		}
	}

	return json.Marshal(persistedAIConfigV2{
		SchemaVersion:           2,
		Enabled:                 value.Enabled,
		DeveloperMode:           value.DeveloperMode,
		UseVisionOnlyWhenNeeded: value.UseVisionOnlyWhenNeeded,
		MinConfidence:           value.MinConfidence,
		Proxy:                   persistedAIProxy{Mode: value.Proxy.Mode, URL: value.Proxy.URL},
		Text:                    normalizeEndpoint(value.Text),
		Vision:                  normalizeEndpoint(value.Vision),
	})
}

// DecodePersisted accepts canonical v2 JSON and the older direct
// json.Marshal(config.AIConfig) shape. When both shapes are present, v2 wins.
// Runtime warnings are never restored from the database.
func DecodePersisted(raw string, base config.AIConfig) (PersistedLoad, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return PersistedLoad{}, fmt.Errorf("decode AI settings: %w", err)
	}

	textRaw, hasText := rawField(fields, "text")
	visionRaw, hasVision := rawField(fields, "vision")
	if hasText && hasVision {
		value := base
		value.SchemaVersion = 2
		value.Enabled = boolField(fields, "enabled", value.Enabled)
		value.DeveloperMode = boolField(fields, "developer_mode", value.DeveloperMode)
		value.UseVisionOnlyWhenNeeded = boolField(fields, "use_vision_only_when_needed", value.UseVisionOnlyWhenNeeded)
		value.MinConfidence = floatField(fields, "min_confidence", value.MinConfidence)
		proxyRaw, hasProxy := rawField(fields, "proxy")
		if hasProxy {
			if proxyFields, err := rawObject(proxyRaw); err == nil {
				value.Proxy = config.AIProxyConfig{
					Mode: stringField(proxyFields, "mode", ""),
					URL:  stringField(proxyFields, "url", ""),
				}
			}
		}
		value.Text = decodeEndpoint(textRaw, config.AIEndpointConfig{})
		value.Vision = decodeEndpoint(visionRaw, config.AIEndpointConfig{})
		value.Provider, value.Model, value.APIKey, value.Endpoint, value.MigrationWarning = "", "", "", "", ""

		legacyKey := stringField(fields, "api_key", "")
		if legacyKey == "" {
			legacyKey = endpointLegacyKey(textRaw)
		}
		canonical := hasAll(fields, "schema_version", "enabled", "developer_mode", "use_vision_only_when_needed", "min_confidence", "proxy", "text", "vision")
		return PersistedLoad{
			Config:       value,
			LegacyAPIKey: legacyKey,
			NeedsRewrite: !canonical || hasAny(fields, "provider", "model", "api_key", "endpoint", "migration_warning"),
		}, nil
	}

	// Legacy flat records are mapped to Text in memory and immediately marked
	// for canonical rewrite. Do not copy their API key into the runtime config.
	value := base
	value.SchemaVersion = 2
	value.Enabled = boolField(fields, "enabled", value.Enabled)
	value.DeveloperMode = boolField(fields, "developer_mode", value.DeveloperMode)
	value.UseVisionOnlyWhenNeeded = boolField(fields, "use_vision_only_when_needed", value.UseVisionOnlyWhenNeeded)
	value.MinConfidence = floatField(fields, "min_confidence", value.MinConfidence)
	legacyProvider := stringField(fields, "provider", "")
	legacyModel := stringField(fields, "model", "")
	legacyEndpoint := stringField(fields, "endpoint", "")
	value.APIKey = ""
	value.MigrationWarning = ""
	if legacyProvider != "" || legacyModel != "" || legacyEndpoint != "" {
		value.Text.Provider = legacyProvider
		value.Text.Model = legacyModel
		value.Text.BaseURL = legacyEndpoint
	}
	value.Provider, value.Model, value.Endpoint = "", "", ""
	return PersistedLoad{
		Config:       value,
		LegacyAPIKey: stringField(fields, "api_key", ""),
		NeedsRewrite: true,
	}, nil
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func rawField(fields map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	wanted := normalizedKey(name)
	for key, value := range fields {
		if normalizedKey(key) == wanted {
			return value, true
		}
	}
	return nil, false
}

func hasAll(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		if _, ok := rawField(fields, name); !ok {
			return false
		}
	}
	return true
}

func hasAny(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		if _, ok := rawField(fields, name); ok {
			return true
		}
	}
	return false
}

func normalizedKey(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func stringField(fields map[string]json.RawMessage, name, fallback string) string {
	raw, ok := rawField(fields, name)
	if !ok {
		return fallback
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}

func boolField(fields map[string]json.RawMessage, name string, fallback bool) bool {
	raw, ok := rawField(fields, name)
	if !ok {
		return fallback
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}

func floatField(fields map[string]json.RawMessage, name string, fallback float64) float64 {
	raw, ok := rawField(fields, name)
	if !ok {
		return fallback
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}

func intField(fields map[string]json.RawMessage, name string, fallback int) int {
	raw, ok := rawField(fields, name)
	if !ok {
		return fallback
	}
	var value int
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}

func stringsField(fields map[string]json.RawMessage, name string, fallback []string) []string {
	raw, ok := rawField(fields, name)
	if !ok {
		return append([]string{}, fallback...)
	}
	var value []string
	if json.Unmarshal(raw, &value) != nil {
		return append([]string{}, fallback...)
	}
	if value == nil {
		return []string{}
	}
	return value
}

func decodeEndpoint(raw json.RawMessage, fallback config.AIEndpointConfig) config.AIEndpointConfig {
	fields, err := rawObject(raw)
	if err != nil {
		return fallback
	}
	value := fallback
	value.Enabled = boolField(fields, "enabled", value.Enabled)
	value.Provider = stringField(fields, "provider", value.Provider)
	value.Model = stringField(fields, "model", value.Model)
	value.FallbackModels = stringsField(fields, "fallback_models", value.FallbackModels)
	value.BaseURL = stringField(fields, "base_url", value.BaseURL)
	value.APIKeyEnv = stringField(fields, "api_key_env", value.APIKeyEnv)
	value.APIKeyFile = stringField(fields, "api_key_file", value.APIKeyFile)
	value.TimeoutSeconds = intField(fields, "timeout_seconds", value.TimeoutSeconds)
	value.JSONMode = stringField(fields, "json_mode", value.JSONMode)
	if rawTemperature, ok := rawField(fields, "temperature"); ok {
		var temperature *float64
		if json.Unmarshal(rawTemperature, &temperature) == nil {
			value.Temperature = temperature
		}
	}
	return value
}

func endpointLegacyKey(raw json.RawMessage) string {
	fields, err := rawObject(raw)
	if err != nil {
		return ""
	}
	return stringField(fields, "api_key", "")
}

// MigrateLegacySecret writes a legacy key to a private, separate secret file.
// The returned path is safe to persist; the secret itself never is.
func MigrateLegacySecret(secretDir, target, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" || len(secret) > 8192 {
		return "", fmt.Errorf("legacy secret is empty or too large")
	}
	if target != "text" && target != "vision" {
		return "", fmt.Errorf("invalid secret target")
	}
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		return "", fmt.Errorf("secret directory unavailable")
	}
	targetPath := filepath.Join(secretDir, target+".key")
	tmp, err := os.CreateTemp(secretDir, ".legacy-"+target+"-*")
	if err != nil {
		return "", fmt.Errorf("secret staging failed")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("secret permissions failed")
	}
	if _, err := tmp.WriteString(secret + "\n"); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("secret staging failed")
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("secret staging failed")
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return "", fmt.Errorf("secret replace failed")
	}
	_ = os.Chmod(targetPath, 0600)
	return filepath.Clean(targetPath), nil
}
