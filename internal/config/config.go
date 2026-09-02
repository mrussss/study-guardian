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
	Standby  StandbyConfig  `yaml:"standby"`
	Study    StudyConfig    `yaml:"study"`
	Break    BreakConfig    `yaml:"break"`
	Reminder ReminderConfig `yaml:"reminder"`
	Screen   ScreenConfig   `yaml:"screen"`
	IPC      IPCConfig      `yaml:"ipc"`
	Privacy  PrivacyConfig  `yaml:"privacy"`
	AI       AIConfig       `yaml:"ai"`
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
	CooldownMinutes int `yaml:"cooldown_minutes"`
}

type ScreenConfig struct {
	Enabled              bool `yaml:"enabled"`
	StoreRaw             bool `yaml:"store_raw"`
	ActiveSampleSeconds  int  `yaml:"active_sample_seconds"`
	UnknownSampleSeconds int  `yaml:"unknown_sample_seconds"`
	BreakSampleSeconds   int  `yaml:"break_sample_seconds"`
}

type IPCConfig struct {
	SupervisorHost string `yaml:"supervisor_host"`
	SupervisorPort int    `yaml:"supervisor_port"`
	SensorHost     string `yaml:"sensor_host"`
	SensorPort     int    `yaml:"sensor_port"`
	AuthToken      string `yaml:"auth_token"`
}

type PrivacyConfig struct {
	SensitiveApps    []string `yaml:"sensitive_apps"`
	SensitiveDomains []string `yaml:"sensitive_domains"`
}

type AIConfig struct {
	Enabled                 bool    `yaml:"enabled"`
	UseVisionOnlyWhenNeeded bool    `yaml:"use_vision_only_when_needed"`
	MinConfidence           float64 `yaml:"min_confidence"`
	Provider                string  `yaml:"provider"`
	Model                   string  `yaml:"model"`
	APIKey                  string  `yaml:"api_key"`
	Endpoint                string  `yaml:"endpoint"`
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
		},
		Screen: ScreenConfig{
			Enabled:              true,
			StoreRaw:             false,
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
			Enabled:                 true,
			UseVisionOnlyWhenNeeded: true,
			MinConfidence:           0.75,
			Provider:                "fake",
		},
	}
}

func LoadConfig(configPath string, tokenPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config yaml: %w", err)
			}
		}
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

func generateRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
