package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

const settingKey = "automation.config.v1"

type StartSettings struct {
	Enabled              bool    `json:"enabled"`
	FocusedStableSeconds int     `json:"focused_stable_seconds"`
	MinConfidence        float64 `json:"min_confidence"`
	AllowUnclassified    bool    `json:"allow_unclassified"`
	Confirm              bool    `json:"confirm"`
}

type PauseSettings struct {
	Enabled            bool `json:"enabled"`
	IdleStaticSeconds  int  `json:"idle_static_seconds"`
	IdleDynamicSeconds int  `json:"idle_dynamic_seconds"`
	LockedSeconds      int  `json:"locked_seconds"`
	Confirm            bool `json:"confirm"`
}

type ResumeSettings struct {
	Enabled              bool `json:"enabled"`
	FocusedStableSeconds int  `json:"focused_stable_seconds"`
}

type Settings struct {
	Enabled                   bool           `json:"enabled"`
	AutoStart                 StartSettings  `json:"auto_start"`
	AutoPause                 PauseSettings  `json:"auto_pause"`
	AutoResume                ResumeSettings `json:"auto_resume"`
	TransitionCooldownSeconds int            `json:"transition_cooldown_seconds"`
	ManualOverrideMinutes     int            `json:"manual_override_minutes"`
}

type SettingsService struct {
	mu         sync.RWMutex
	cfg        *config.Config
	store      *storage.Storage
	controller *Controller
}

func NewSettingsService(cfg *config.Config, store *storage.Storage, controller *Controller) *SettingsService {
	return &SettingsService{cfg: cfg, store: store, controller: controller}
}

func (s *SettingsService) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fromConfig(s.cfg.Automation)
}

func (s *SettingsService) Save(ctx context.Context, input Settings) (Settings, error) {
	if err := validate(input); err != nil {
		return Settings{}, err
	}
	next := toConfig(input)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Persist first. The in-memory config and controller must not move ahead
	// of durable settings when SQLite rejects the write.
	if s.store != nil {
		if err := s.store.SetSetting(ctx, settingKey, marshalSettings(next), time.Now()); err != nil {
			return Settings{}, err
		}
	}
	s.cfg.Automation = next
	if s.controller != nil {
		s.controller.UpdateConfig(next)
	}
	return fromConfig(next), nil
}

func fromConfig(value config.AutomationConfig) Settings {
	return Settings{
		Enabled:                   value.Enabled,
		AutoStart:                 StartSettings{Enabled: value.AutoStart.Enabled, FocusedStableSeconds: value.AutoStart.FocusedStableSeconds, MinConfidence: value.AutoStart.MinConfidence, AllowUnclassified: value.AutoStart.AllowUnclassified, Confirm: value.AutoStart.Confirm},
		AutoPause:                 PauseSettings{Enabled: value.AutoPause.Enabled, IdleStaticSeconds: value.AutoPause.IdleStaticSeconds, IdleDynamicSeconds: value.AutoPause.IdleDynamicSeconds, LockedSeconds: value.AutoPause.LockedSeconds, Confirm: value.AutoPause.Confirm},
		AutoResume:                ResumeSettings{Enabled: value.AutoResume.Enabled, FocusedStableSeconds: value.AutoResume.FocusedStableSeconds},
		TransitionCooldownSeconds: value.TransitionCooldownSeconds,
		ManualOverrideMinutes:     value.ManualOverrideMinutes,
	}
}

func toConfig(value Settings) config.AutomationConfig {
	return config.AutomationConfig{Enabled: value.Enabled,
		AutoStart:                 config.AutomationStartConfig{Enabled: value.AutoStart.Enabled, FocusedStableSeconds: value.AutoStart.FocusedStableSeconds, MinConfidence: value.AutoStart.MinConfidence, AllowUnclassified: value.AutoStart.AllowUnclassified, Confirm: value.AutoStart.Confirm},
		AutoPause:                 config.AutomationPauseConfig{Enabled: value.AutoPause.Enabled, IdleStaticSeconds: value.AutoPause.IdleStaticSeconds, IdleDynamicSeconds: value.AutoPause.IdleDynamicSeconds, LockedSeconds: value.AutoPause.LockedSeconds, Confirm: value.AutoPause.Confirm},
		AutoResume:                config.AutomationResumeConfig{Enabled: value.AutoResume.Enabled, FocusedStableSeconds: value.AutoResume.FocusedStableSeconds},
		TransitionCooldownSeconds: value.TransitionCooldownSeconds, ManualOverrideMinutes: value.ManualOverrideMinutes}
}

func ConfigFromSettings(value Settings) config.AutomationConfig { return toConfig(value) }

func validate(value Settings) error {
	if value.AutoStart.FocusedStableSeconds < 1 || value.AutoStart.FocusedStableSeconds > 3600 {
		return fmt.Errorf("auto_start focused_stable_seconds must be 1-3600")
	}
	if value.AutoStart.MinConfidence < 0 || value.AutoStart.MinConfidence > 1 {
		return fmt.Errorf("auto_start min_confidence must be 0-1")
	}
	if value.AutoPause.IdleStaticSeconds < 1 || value.AutoPause.IdleStaticSeconds > 86400 || value.AutoPause.IdleDynamicSeconds < 1 || value.AutoPause.IdleDynamicSeconds > 86400 || value.AutoPause.LockedSeconds < 1 || value.AutoPause.LockedSeconds > 3600 {
		return fmt.Errorf("auto_pause thresholds are out of range")
	}
	if value.AutoResume.FocusedStableSeconds < 1 || value.AutoResume.FocusedStableSeconds > 3600 {
		return fmt.Errorf("auto_resume focused_stable_seconds must be 1-3600")
	}
	if value.TransitionCooldownSeconds < 1 || value.TransitionCooldownSeconds > 3600 || value.ManualOverrideMinutes < 0 || value.ManualOverrideMinutes > 1440 {
		return fmt.Errorf("automation cooldown or manual override is out of range")
	}
	return nil
}

func ValidateSettings(value Settings) error { return validate(value) }

func marshalSettings(value config.AutomationConfig) string {
	raw, _ := json.Marshal(fromConfig(value))
	return string(raw)
}
