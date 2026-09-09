package aisettings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type PersistedAIStore interface {
	GetSetting(context.Context, string) (string, bool, error)
	SetSetting(context.Context, string, string, time.Time) error
}

type MigrationResult struct {
	Config        config.AIConfig
	Found         bool
	Rewritten     bool
	SecretReused  bool
	SecretCreated bool
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

	if schemaRaw, present := rawField(fields, "schema_version"); present {
		version, err := requiredInt(schemaRaw, "schema_version")
		if err != nil || version != 2 {
			if err != nil {
				return PersistedLoad{}, err
			}
			return PersistedLoad{}, fmt.Errorf("unsupported AI settings schema_version %d", version)
		}
		return decodeStrictV2(fields, base)
	}

	if !hasAny(fields, "provider", "model", "endpoint", "api_key") {
		return PersistedLoad{}, fmt.Errorf("AI settings have neither schema_version nor legacy flat fields")
	}
	return decodeLegacyFlat(fields, base)
}

func decodeStrictV2(fields map[string]json.RawMessage, base config.AIConfig) (PersistedLoad, error) {
	for _, name := range []string{"enabled", "developer_mode", "use_vision_only_when_needed", "min_confidence", "proxy", "text", "vision"} {
		if _, ok := rawField(fields, name); !ok {
			return PersistedLoad{}, fmt.Errorf("schema v2 missing %s", name)
		}
	}
	enabled, err := requiredBoolField(fields, "enabled")
	if err != nil {
		return PersistedLoad{}, err
	}
	developerMode, err := requiredBoolField(fields, "developer_mode")
	if err != nil {
		return PersistedLoad{}, err
	}
	visionOnly, err := requiredBoolField(fields, "use_vision_only_when_needed")
	if err != nil {
		return PersistedLoad{}, err
	}
	minConfidence, err := requiredFloatField(fields, "min_confidence")
	if err != nil {
		return PersistedLoad{}, err
	}
	if minConfidence < 0 || minConfidence > 1 || math.IsNaN(minConfidence) || math.IsInf(minConfidence, 0) {
		return PersistedLoad{}, fmt.Errorf("min_confidence must be between 0 and 1")
	}

	proxyRaw, _ := rawField(fields, "proxy")
	proxyFields, err := rawObject(proxyRaw)
	if err != nil {
		return PersistedLoad{}, fmt.Errorf("schema v2 proxy must be an object")
	}
	proxyMode, err := requiredStringField(proxyFields, "mode")
	if err != nil {
		return PersistedLoad{}, err
	}
	proxyURL, err := requiredStringField(proxyFields, "url")
	if err != nil {
		return PersistedLoad{}, err
	}
	proxy := config.AIProxyConfig{Mode: proxyMode, URL: proxyURL}
	if err := config.ValidateAIProxyConfig(proxy); err != nil {
		return PersistedLoad{}, fmt.Errorf("invalid AI proxy config: %w", err)
	}

	textRaw, _ := rawField(fields, "text")
	visionRaw, _ := rawField(fields, "vision")
	text, err := decodeStrictEndpoint(textRaw, "text")
	if err != nil {
		return PersistedLoad{}, err
	}
	vision, err := decodeStrictEndpoint(visionRaw, "vision")
	if err != nil {
		return PersistedLoad{}, err
	}
	value := base
	value.SchemaVersion = 2
	value.Enabled = enabled
	value.DeveloperMode = developerMode
	value.UseVisionOnlyWhenNeeded = visionOnly
	value.MinConfidence = minConfidence
	value.Proxy = proxy
	value.Text = text
	value.Vision = vision
	value.Provider, value.Model, value.APIKey, value.Endpoint, value.MigrationWarning = "", "", "", "", ""
	legacyKey := stringField(fields, "api_key", "")
	if legacyKey == "" {
		legacyKey = endpointLegacyKey(textRaw)
	}
	return PersistedLoad{
		Config:       value,
		LegacyAPIKey: legacyKey,
		NeedsRewrite: hasAny(fields, "provider", "model", "api_key", "endpoint", "migration_warning") || !hasAllEndpointFields(textRaw) || !hasAllEndpointFields(visionRaw),
	}, nil
}

func decodeLegacyFlat(fields map[string]json.RawMessage, base config.AIConfig) (PersistedLoad, error) {
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
	return PersistedLoad{Config: value, LegacyAPIKey: stringField(fields, "api_key", ""), NeedsRewrite: true}, nil
}

func decodeStrictEndpoint(raw json.RawMessage, label string) (config.AIEndpointConfig, error) {
	fields, err := rawObject(raw)
	if err != nil {
		return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s must be an object", label)
	}
	for _, name := range []string{"enabled", "provider", "model", "base_url", "api_key_env", "api_key_file", "timeout_seconds", "json_mode", "temperature"} {
		if _, ok := rawField(fields, name); !ok {
			return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s missing %s", label, name)
		}
	}
	enabled, err := requiredBoolField(fields, "enabled")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	provider, err := requiredStringField(fields, "provider")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	model, err := requiredStringField(fields, "model")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	baseURL, err := requiredStringField(fields, "base_url")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	apiKeyEnv, err := requiredStringField(fields, "api_key_env")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	apiKeyFile, err := requiredStringField(fields, "api_key_file")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	timeoutSeconds, err := requiredIntField(fields, "timeout_seconds")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	if timeoutSeconds < 1 || timeoutSeconds > 120 {
		return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s timeout_seconds must be 1-120", label)
	}
	jsonMode, err := requiredStringField(fields, "json_mode")
	if err != nil {
		return config.AIEndpointConfig{}, err
	}
	if jsonMode != "auto" && jsonMode != "json_object" && jsonMode != "off" {
		return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s has invalid json_mode", label)
	}
	fallbackModels := []string{}
	if fallbackRaw, ok := rawField(fields, "fallback_models"); ok {
		if bytes.Equal(bytes.TrimSpace(fallbackRaw), []byte("null")) || json.Unmarshal(fallbackRaw, &fallbackModels) != nil {
			return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s fallback_models must be an array", label)
		}
	}
	if len(fallbackModels) > 3 {
		return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s fallback_models supports at most 3 models", label)
	}
	seen := map[string]struct{}{model: {}}
	for _, fallback := range fallbackModels {
		if strings.TrimSpace(fallback) == "" || len(fallback) > 128 {
			return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s has invalid fallback model", label)
		}
		if _, exists := seen[fallback]; exists {
			return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s fallback model duplicates primary", label)
		}
		seen[fallback] = struct{}{}
	}
	temperatureRaw, _ := rawField(fields, "temperature")
	var temperature *float64
	if !bytes.Equal(bytes.TrimSpace(temperatureRaw), []byte("null")) {
		var value float64
		if json.Unmarshal(temperatureRaw, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return config.AIEndpointConfig{}, fmt.Errorf("schema v2 %s temperature must be a number or null", label)
		}
		temperature = &value
	}
	return config.AIEndpointConfig{Enabled: enabled, Provider: provider, Model: model, FallbackModels: fallbackModels, BaseURL: baseURL, APIKeyEnv: apiKeyEnv, APIKeyFile: apiKeyFile, TimeoutSeconds: timeoutSeconds, JSONMode: jsonMode, Temperature: temperature}, nil
}

func hasAllEndpointFields(raw json.RawMessage) bool {
	fields, err := rawObject(raw)
	if err != nil {
		return false
	}
	for _, name := range []string{"enabled", "provider", "model", "fallback_models", "base_url", "api_key_env", "api_key_file", "timeout_seconds", "json_mode", "temperature"} {
		if _, ok := rawField(fields, name); !ok {
			return false
		}
	}
	return true
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

func requiredStringField(fields map[string]json.RawMessage, name string) (string, error) {
	raw, ok := rawField(fields, name)
	if !ok {
		return "", fmt.Errorf("schema v2 missing %s", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("schema v2 %s must be a string", name)
	}
	return value, nil
}

func requiredBool(raw json.RawMessage, name string) (bool, error) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("schema v2 %s must be a boolean", name)
	}
	return value, nil
}

func requiredBoolField(fields map[string]json.RawMessage, name string) (bool, error) {
	raw, ok := rawField(fields, name)
	if !ok {
		return false, fmt.Errorf("schema v2 missing %s", name)
	}
	return requiredBool(raw, name)
}

func requiredInt(raw json.RawMessage, name string) (int, error) {
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("schema v2 %s must be an integer", name)
	}
	return value, nil
}

func requiredIntField(fields map[string]json.RawMessage, name string) (int, error) {
	raw, ok := rawField(fields, name)
	if !ok {
		return 0, fmt.Errorf("schema v2 missing %s", name)
	}
	return requiredInt(raw, name)
}

func requiredFloatField(fields map[string]json.RawMessage, name string) (float64, error) {
	raw, ok := rawField(fields, name)
	if !ok {
		return 0, fmt.Errorf("schema v2 missing %s", name)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("schema v2 %s must be a number", name)
	}
	return value, nil
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

// LoadAndMigratePersistedAI implements the recoverable migration order:
// decode, make the secret reference safe, then rewrite canonical JSON. A
// failed secret step or database write leaves the original setting untouched.
func LoadAndMigratePersistedAI(ctx context.Context, store PersistedAIStore, secretDir string, base config.AIConfig) (MigrationResult, error) {
	raw, found, err := store.GetSetting(ctx, SettingKey)
	if err != nil {
		return MigrationResult{Config: base}, fmt.Errorf("read persisted AI settings: %w", err)
	}
	if !found {
		return MigrationResult{Config: base}, nil
	}
	loaded, err := DecodePersisted(raw, base)
	if err != nil {
		return MigrationResult{Config: base, Found: true}, err
	}
	result := MigrationResult{Config: loaded.Config, Found: true}
	if !loaded.NeedsRewrite {
		return result, nil
	}

	if loaded.LegacyAPIKey != "" {
		if existing := configuredSecretPath(loaded.Config, "text"); existing != "" {
			result.Config.Text.APIKeyFile = existing
			result.Config.Text.APIKeyEnv = ""
			result.SecretReused = true
		} else {
			secretPath, migrateErr := MigrateLegacySecret(secretDir, "text", loaded.LegacyAPIKey)
			if migrateErr != nil {
				return MigrationResult{Config: base, Found: true}, fmt.Errorf("legacy secret migration: %w", migrateErr)
			}
			if !secretFileValid(secretPath) {
				return MigrationResult{Config: base, Found: true}, fmt.Errorf("legacy secret migration: target verification failed")
			}
			result.Config.Text.APIKeyFile = secretPath
			result.Config.Text.APIKeyEnv = ""
			result.SecretCreated = true
		}
	}
	encoded, err := EncodePersistedAIConfig(result.Config)
	if err != nil {
		return MigrationResult{Config: base, Found: true}, fmt.Errorf("canonical AI settings encoding: %w", err)
	}
	if err := store.SetSetting(ctx, SettingKey, string(encoded), time.Now()); err != nil {
		return MigrationResult{Config: base, Found: true}, fmt.Errorf("canonical AI settings persistence: %w", err)
	}
	result.Rewritten = true
	return result, nil
}

func configuredSecretPath(value config.AIConfig, target string) string {
	var path string
	if target == "text" {
		path = value.Text.APIKeyFile
	} else {
		path = value.Vision.APIKeyFile
	}
	if secretFileValid(path) {
		return filepath.Clean(path)
	}
	return ""
}

func secretFileValid(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) != ""
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
	if secretFileValid(targetPath) {
		return filepath.Clean(targetPath), nil
	}
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
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("secret staging failed")
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("secret staging failed")
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		// Windows does not replace an existing file with Rename. Never remove a
		// non-empty target; it is a newer valid secret and must win.
		if secretFileValid(targetPath) {
			return filepath.Clean(targetPath), nil
		}
		if info, statErr := os.Stat(targetPath); statErr == nil {
			if info.IsDir() {
				return "", fmt.Errorf("secret replace failed")
			}
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return "", fmt.Errorf("secret replace failed")
			}
			if retryErr := os.Rename(tmpName, targetPath); retryErr != nil {
				return "", fmt.Errorf("secret replace failed")
			}
		} else {
			return "", fmt.Errorf("secret replace failed")
		}
	}
	_ = os.Chmod(targetPath, 0600)
	if !secretFileValid(targetPath) {
		return "", fmt.Errorf("secret verification failed")
	}
	return filepath.Clean(targetPath), nil
}
