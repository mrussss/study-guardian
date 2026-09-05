package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Standby    StandbyConfig    `yaml:"standby"`
	Study      StudyConfig      `yaml:"study"`
	Break      BreakConfig      `yaml:"break"`
	Reminder   ReminderConfig   `yaml:"reminder"`
	Screen     ScreenConfig     `yaml:"screen"`
	IPC        IPCConfig        `yaml:"ipc"`
	Privacy    PrivacyConfig    `yaml:"privacy"`
	AI         AIConfig         `yaml:"ai"`
	Motivation MotivationConfig `yaml:"motivation"`
	Review     ReviewConfig     `yaml:"review"`
}

type StandbyConfig struct {
	FirstStudyActiveMinutes int `yaml:"first_study_active_minutes"`
	RepeatReminderMinutes   int `yaml:"repeat_reminder_minutes"`
}

type StudyConfig struct {
	DistractionWarnMinutes   int `yaml:"distraction_warn_minutes"`
	DistractionStrongMinutes int `yaml:"distraction_strong_minutes"`
	IdleStaticWarnMinutes    int `yaml:"idle_static_warn_minutes"`
	IdleStaticStrongMinutes  int `yaml:"idle_static_strong_minutes"`
}

type BreakConfig struct {
	WarnMinutes   int `yaml:"warn_minutes"`
	StrongMinutes int `yaml:"strong_minutes"`
	RepeatMinutes int `yaml:"repeat_minutes"`
}

type ReminderConfig struct {
	CooldownMinutes int                 `yaml:"cooldown_minutes" json:"cooldown_minutes"`
	QuietPeriods    []QuietPeriodConfig `yaml:"quiet_periods" json:"quiet_periods"`
}

type ScreenConfig struct {
	Enabled  bool `yaml:"enabled"`
	StoreRaw bool `yaml:"store_raw"`
	// Monitor 0 means the virtual desktop (all monitors); 1..N selects a
	// physical monitor.
	Monitor              int `yaml:"monitor"`
	ActiveSampleSeconds  int `yaml:"active_sample_seconds"`
	UnknownSampleSeconds int `yaml:"unknown_sample_seconds"`
	BreakSampleSeconds   int `yaml:"break_sample_seconds"`
}

type IPCConfig struct {
	SupervisorHost string `yaml:"supervisor_host"`
	SupervisorPort int    `yaml:"supervisor_port"`
	SensorHost     string `yaml:"sensor_host"`
	SensorPort     int    `yaml:"sensor_port"`
	AuthToken      string `yaml:"auth_token"`
	CollectorToken string `yaml:"-"`
}

type PrivacyConfig struct {
	SensitiveApps    []string `yaml:"sensitive_apps"`
	SensitiveDomains []string `yaml:"sensitive_domains"`
}

type AIConfig struct {
	SchemaVersion           int              `yaml:"schema_version"`
	Enabled                 bool             `yaml:"enabled"`
	DeveloperMode           bool             `yaml:"developer_mode"`
	UseVisionOnlyWhenNeeded bool             `yaml:"use_vision_only_when_needed"`
	MinConfidence           float64          `yaml:"min_confidence"`
	Text                    AIEndpointConfig `yaml:"text"`
	Vision                  AIEndpointConfig `yaml:"vision"`
	// Legacy fields are retained for backwards-compatible loading. New code
	// should use Text/Vision; LoadConfig maps the old flat shape into Text.
	Provider         string `yaml:"provider"`
	Model            string `yaml:"model"`
	APIKey           string `yaml:"api_key"`
	Endpoint         string `yaml:"endpoint"`
	MigrationWarning string `yaml:"-"`
}

type AIEndpointConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Provider       string   `yaml:"provider"`
	Model          string   `yaml:"model"`
	BaseURL        string   `yaml:"base_url"`
	APIKeyEnv      string   `yaml:"api_key_env"`
	APIKeyFile     string   `yaml:"api_key_file"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	JSONMode       string   `yaml:"json_mode"`
	Temperature    *float64 `yaml:"temperature"`
}

type MotivationConfig struct {
	Enabled                      bool  `yaml:"enabled"`
	DefaultDailyTargetMinutes    int   `yaml:"default_daily_target_minutes"`
	DailyTargetMinutes           int   `yaml:"daily_target_minutes"` // legacy alias
	CheckinThresholdMinutes      int   `yaml:"checkin_threshold_minutes"`
	IdleStaticCreditGraceSeconds int   `yaml:"idle_static_credit_grace_seconds"`
	APPerFocusHourMilli          int64 `yaml:"ap_per_focus_hour_milli"`
}

type ReviewConfig struct {
	Enabled   bool                  `yaml:"enabled"`
	Timezone  string                `yaml:"timezone"`
	Provider  ReviewProviderConfig  `yaml:"provider"`
	Trigger   ReviewTriggerConfig   `yaml:"trigger"`
	Retention ReviewRetentionConfig `yaml:"retention"`
	Limits    ReviewLimitsConfig    `yaml:"limits"`
}

type ReviewProviderConfig struct {
	InheritTextProfile bool     `yaml:"inherit_text_profile"`
	Provider           string   `yaml:"provider"`
	Model              string   `yaml:"model"`
	BaseURL            string   `yaml:"base_url"`
	APIKeyEnv          string   `yaml:"api_key_env"`
	APIKeyFile         string   `yaml:"api_key_file"`
	TimeoutSeconds     int      `yaml:"timeout_seconds"`
	JSONMode           string   `yaml:"json_mode"`
	Temperature        *float64 `yaml:"temperature"`
}

type ReviewTriggerConfig struct {
	OffDebounceMinutes  int  `yaml:"off_debounce_minutes"`
	BackfillPreviousDay bool `yaml:"backfill_previous_day"`
}

type ReviewRetentionConfig struct {
	RawChatDays  int `yaml:"raw_chat_days"`
	SemanticDays int `yaml:"semantic_days"`
}

type ReviewLimitsConfig struct {
	MaxTurnChars         int `yaml:"max_turn_chars"`
	MaxConversationChars int `yaml:"max_conversation_chars"`
	MaxFinalInputChars   int `yaml:"max_final_input_chars"`
}

func DefaultConfig() *Config {
	return &Config{
		Standby: StandbyConfig{
			FirstStudyActiveMinutes: 60,
			RepeatReminderMinutes:   30,
		},
		Study: StudyConfig{
			DistractionWarnMinutes:   8,
			DistractionStrongMinutes: 15,
			IdleStaticWarnMinutes:    20,
			IdleStaticStrongMinutes:  30,
		},
		Break: BreakConfig{
			WarnMinutes:   20,
			StrongMinutes: 30,
			RepeatMinutes: 15,
		},
		Reminder: ReminderConfig{
			CooldownMinutes: 10,
			QuietPeriods: []QuietPeriodConfig{
				{Start: "12:00", End: "14:00"},
				{Start: "17:30", End: "19:00"},
				{Start: "21:00", End: "24:00"},
			},
		},
		Screen: ScreenConfig{
			Enabled:              true,
			StoreRaw:             false,
			Monitor:              0,
			ActiveSampleSeconds:  15,
			UnknownSampleSeconds: 5,
			BreakSampleSeconds:   60,
		},
		IPC: IPCConfig{
			SupervisorHost: "127.0.0.1",
			SupervisorPort: 17321,
			SensorHost:     "127.0.0.1",
			SensorPort:     17322,
			AuthToken:      "",
		},
		Privacy: PrivacyConfig{
			SensitiveApps: []string{
				"keepass", "1password", "bitwarden", "credential", "authenticator", "wechatpay", "alipay",
			},
			SensitiveDomains: []string{
				"bank", "login", "auth", "checkout", "accounts.google.com",
			},
		},
		AI: AIConfig{
			SchemaVersion:           2,
			Enabled:                 false,
			UseVisionOnlyWhenNeeded: true,
			MinConfidence:           0.75,
			Provider:                "none",
		},
		Motivation: MotivationConfig{Enabled: true, DefaultDailyTargetMinutes: 120, CheckinThresholdMinutes: 30, IdleStaticCreditGraceSeconds: 300, APPerFocusHourMilli: 1000},
		Review: ReviewConfig{
			Enabled: true, Timezone: "local",
			Provider:  ReviewProviderConfig{InheritTextProfile: true, TimeoutSeconds: 30, JSONMode: "auto"},
			Trigger:   ReviewTriggerConfig{OffDebounceMinutes: 5, BackfillPreviousDay: true},
			Retention: ReviewRetentionConfig{RawChatDays: 30, SemanticDays: 180},
			Limits:    ReviewLimitsConfig{MaxTurnChars: 12000, MaxConversationChars: 40000, MaxFinalInputChars: 60000},
		},
	}
}

func LoadConfig(configPath string, tokenPath string) (*Config, error) {
	cfg := DefaultConfig()

	legacyAI := false
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config yaml: %w", err)
			}
			legacyAI = isLegacyAIBlock(data) && (cfg.AI.Provider != "" || cfg.AI.Endpoint != "" || cfg.AI.APIKey != "")
		}
	}
	NormalizeAIConfig(cfg, legacyAI)
	NormalizeMotivationConfig(cfg)
	if err := ValidateReminderConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid reminder config: %w", err)
	}

	// Ensure token exists or load from tokenPath
	if tokenPath != "" {
		token, err := ensureAuthToken(tokenPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve auth token: %w", err)
		}
		cfg.IPC.AuthToken = token
	} else if cfg.IPC.AuthToken == "" {
		cfg.IPC.AuthToken = generateRandomToken()
	}

	return cfg, nil
}

func NormalizeMotivationConfig(cfg *Config) {
	if cfg.Motivation.DefaultDailyTargetMinutes <= 0 {
		cfg.Motivation.DefaultDailyTargetMinutes = cfg.Motivation.DailyTargetMinutes
	}
	if cfg.Motivation.DefaultDailyTargetMinutes <= 0 {
		cfg.Motivation.DefaultDailyTargetMinutes = 120
	}
	if cfg.Motivation.CheckinThresholdMinutes <= 0 {
		cfg.Motivation.CheckinThresholdMinutes = 30
	}
	if cfg.Motivation.IdleStaticCreditGraceSeconds < 0 {
		cfg.Motivation.IdleStaticCreditGraceSeconds = 0
	}
	if cfg.Motivation.APPerFocusHourMilli <= 0 {
		cfg.Motivation.APPerFocusHourMilli = 1000
	}
}

// NormalizeAIConfig upgrades the legacy flat ai block in memory. It never
// rewrites the user's config file; migrate-config.ps1 is the explicit writer.
func NormalizeAIConfig(cfg *Config, legacy bool) {
	if legacy {
		cfg.AI.Text.Provider = cfg.AI.Provider
		cfg.AI.Text.Model = cfg.AI.Model
		cfg.AI.Text.BaseURL = cfg.AI.Endpoint
	}
	if cfg.AI.Text.Provider == "" {
		cfg.AI.Text.Provider = "none"
	}
	if cfg.AI.Text.TimeoutSeconds <= 0 {
		cfg.AI.Text.TimeoutSeconds = 6
	}
	if cfg.AI.Text.JSONMode == "" {
		cfg.AI.Text.JSONMode = "auto"
	}
	if cfg.AI.Vision.TimeoutSeconds <= 0 {
		cfg.AI.Vision.TimeoutSeconds = 8
	}
	if cfg.AI.Vision.JSONMode == "" {
		cfg.AI.Vision.JSONMode = "auto"
	}
	if cfg.AI.Vision.Provider == "" {
		cfg.AI.Vision.Provider = "none"
	}
	if cfg.AI.Text.Provider == "fake" && !cfg.AI.DeveloperMode {
		cfg.AI.Enabled = false
		cfg.AI.MigrationWarning = "provider fake is disabled outside developer_mode; rules continue"
	}
	if legacy {
		cfg.AI.MigrationWarning = "legacy flat AI config loaded in memory; run migrate-config.ps1 to persist schema v2"
	}
}

func isLegacyAIBlock(data []byte) bool {
	var root map[string]yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false
	}
	ai, ok := root["ai"]
	if !ok || ai.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(ai.Content); i += 2 {
		if ai.Content[i].Value == "text" || ai.Content[i].Value == "vision" {
			return false
		}
	}
	return true
}

func ensureAuthToken(tokenPath string) (string, error) {
	if data, err := os.ReadFile(tokenPath); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
	}

	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	token := generateRandomToken()
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0600); err != nil {
		return "", err
	}

	return token, nil
}

// EnsureToken creates a token file with restrictive permissions if needed.
// It is used for the collector's scoped credential and remains separate from
// the main Supervisor token.
func EnsureToken(tokenPath string) (string, error) { return ensureAuthToken(tokenPath) }

func generateRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
