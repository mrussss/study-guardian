package aisettings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/ai"
	"study-guardian/internal/classifier"
	"study-guardian/internal/classifier/providers"
	"study-guardian/internal/config"
	"study-guardian/internal/review"
	"study-guardian/internal/storage"
)

const SettingKey = "ai.config.v1"

type EndpointDTO struct {
	Enabled          bool   `json:"enabled"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	BaseURL          string `json:"base_url"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	JSONMode         string `json:"json_mode"`
}

type SettingsDTO struct {
	Enabled       bool        `json:"enabled"`
	MinConfidence float64     `json:"min_confidence"`
	Text          EndpointDTO `json:"text"`
	Vision        EndpointDTO `json:"vision"`
}

type TestResult struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
	ErrorKind string `json:"error_kind,omitempty"`
}

type Service struct {
	mu         sync.RWMutex
	cfg        *config.Config
	store      *storage.Storage
	secretDir  string
	classifier *classifier.Service
	review     *review.Service
	registry   *providers.Registry
}

func New(cfg *config.Config, store *storage.Storage, secretDir string, classifierService *classifier.Service, reviewService *review.Service) *Service {
	config.NormalizeAIConfig(cfg, false)
	s := &Service{cfg: cfg, store: store, secretDir: secretDir, classifier: classifierService, review: reviewService}
	s.applyLocked(cfg.AI)
	return s
}

func (s *Service) Settings() SettingsDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sanitized(s.cfg.AI)
}

func sanitized(aiConfig config.AIConfig) SettingsDTO {
	configured := func(endpoint config.AIEndpointConfig) bool {
		if endpoint.Provider == "ollama" {
			return true
		}
		if endpoint.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(endpoint.APIKeyEnv)) != "" {
			return true
		}
		if endpoint.APIKeyFile != "" {
			if data, err := os.ReadFile(endpoint.APIKeyFile); err == nil && strings.TrimSpace(string(data)) != "" {
				return true
			}
		}
		return false
	}
	toDTO := func(endpoint config.AIEndpointConfig) EndpointDTO {
		return EndpointDTO{Enabled: endpoint.Enabled, Provider: endpoint.Provider, Model: endpoint.Model, BaseURL: endpoint.BaseURL, APIKeyConfigured: configured(endpoint), TimeoutSeconds: endpoint.TimeoutSeconds, JSONMode: endpoint.JSONMode}
	}
	return SettingsDTO{Enabled: aiConfig.Enabled, MinConfidence: aiConfig.MinConfidence, Text: toDTO(aiConfig.Text), Vision: toDTO(aiConfig.Vision)}
}

func (s *Service) Save(ctx context.Context, input SettingsDTO) (SettingsDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateDTO(input); err != nil {
		return SettingsDTO{}, err
	}
	next := s.cfg.AI
	next.Enabled = input.Enabled
	next.MinConfidence = input.MinConfidence
	applyEndpoint := func(current config.AIEndpointConfig, dto EndpointDTO) config.AIEndpointConfig {
		current.Enabled, current.Provider, current.Model, current.BaseURL = dto.Enabled, strings.ToLower(strings.TrimSpace(dto.Provider)), strings.TrimSpace(dto.Model), strings.TrimSpace(dto.BaseURL)
		current.TimeoutSeconds, current.JSONMode = dto.TimeoutSeconds, strings.TrimSpace(dto.JSONMode)
		return current
	}
	next.Text = applyEndpoint(next.Text, input.Text)
	next.Vision = applyEndpoint(next.Vision, input.Vision)
	next.UseVisionOnlyWhenNeeded = true
	if err := s.persistLocked(ctx, next); err != nil {
		return SettingsDTO{}, err
	}
	s.applyLocked(next)
	return sanitized(next), nil
}

func validateDTO(input SettingsDTO) error {
	if input.MinConfidence < 0 || input.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}
	for label, endpoint := range map[string]EndpointDTO{"text": input.Text, "vision": input.Vision} {
		profile, ok := providers.ProfileFor(endpoint.Provider)
		if !ok || endpoint.Provider == "fake" {
			return fmt.Errorf("unsupported %s provider", label)
		}
		if endpoint.TimeoutSeconds < 1 || endpoint.TimeoutSeconds > 120 {
			return fmt.Errorf("%s timeout_seconds must be 1-120", label)
		}
		if endpoint.JSONMode != "auto" && endpoint.JSONMode != "json_object" && endpoint.JSONMode != "off" {
			return fmt.Errorf("unsupported %s json_mode", label)
		}
		if endpoint.Provider != "none" && endpoint.Model == "" {
			return fmt.Errorf("%s model is required", label)
		}
		if endpoint.Provider == "openai-compatible" && endpoint.BaseURL == "" {
			return fmt.Errorf("%s base_url is required", label)
		}
		_ = profile
	}
	return nil
}

func (s *Service) PutSecret(ctx context.Context, target, secret string) (SettingsDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret = strings.TrimSpace(secret)
	if secret == "" || len(secret) > 8192 {
		return SettingsDTO{}, fmt.Errorf("API key must be non-empty and bounded")
	}
	endpoint, err := s.endpointLocked(target)
	if err != nil {
		return SettingsDTO{}, err
	}
	if err := os.MkdirAll(s.secretDir, 0700); err != nil {
		return SettingsDTO{}, fmt.Errorf("secret directory unavailable")
	}
	path := filepath.Join(s.secretDir, target+".key")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0600); err != nil {
		return SettingsDTO{}, fmt.Errorf("secret write failed")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return SettingsDTO{}, fmt.Errorf("secret replace failed")
	}
	_ = os.Chmod(path, 0600)
	endpoint.APIKeyFile, endpoint.APIKeyEnv = filepath.Clean(path), ""
	next := s.cfg.AI
	if target == "text" {
		next.Text = *endpoint
	} else {
		next.Vision = *endpoint
	}
	if err := s.persistLocked(ctx, next); err != nil {
		return SettingsDTO{}, err
	}
	s.applyLocked(next)
	return sanitized(next), nil
}

func (s *Service) DeleteSecret(ctx context.Context, target string) (SettingsDTO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint, err := s.endpointLocked(target)
	if err != nil {
		return SettingsDTO{}, err
	}
	managed := filepath.Join(s.secretDir, target+".key")
	if endpoint.APIKeyFile != "" && filepath.Clean(endpoint.APIKeyFile) == filepath.Clean(managed) {
		_ = os.Remove(managed)
	}
	endpoint.APIKeyFile, endpoint.APIKeyEnv = "", ""
	next := s.cfg.AI
	if target == "text" {
		next.Text = *endpoint
	} else {
		next.Vision = *endpoint
	}
	if err := s.persistLocked(ctx, next); err != nil {
		return SettingsDTO{}, err
	}
	s.applyLocked(next)
	return sanitized(next), nil
}

func (s *Service) endpointLocked(target string) (*config.AIEndpointConfig, error) {
	switch target {
	case "text":
		value := s.cfg.AI.Text
		return &value, nil
	case "vision":
		value := s.cfg.AI.Vision
		return &value, nil
	default:
		return nil, fmt.Errorf("target must be text or vision")
	}
}

func (s *Service) persistLocked(ctx context.Context, next config.AIConfig) error {
	raw, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("settings encoding failed")
	}
	if err := s.store.SetSetting(ctx, SettingKey, string(raw), time.Now()); err != nil {
		return fmt.Errorf("settings persistence failed")
	}
	return nil
}

func (s *Service) applyLocked(next config.AIConfig) {
	s.cfg.AI = next
	s.registry = providers.New(s.cfg)
	if s.classifier != nil {
		s.classifier.UpdateAI(next, s.registry.Provider(), s.registry.VisionProvider())
	}
	if s.review != nil {
		provider, _ := review.NewConfiguredProvider(s.cfg)
		s.review.SetProvider(provider)
	}
}

func (s *Service) Status() providers.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.registry == nil {
		return providers.Status{}
	}
	return s.registry.Status()
}

func (s *Service) Test(ctx context.Context, target string) TestResult {
	s.mu.RLock()
	registry := providers.New(s.cfg)
	settings := sanitized(s.cfg.AI)
	s.mu.RUnlock()
	var provider classifier.TaskRelationProvider
	var endpoint EndpointDTO
	request := classifier.ClassificationRequest{Task: "connection test", App: "StudyGuardian", Title: "Fixed non-private connection test", UserMode: "STUDY"}
	if target == "text" {
		provider, endpoint = registry.Provider(), settings.Text
	} else if target == "vision" {
		provider, endpoint = registry.VisionProvider(), settings.Vision
		request.AnalysisImageBase64 = fixedTestPNG()
	} else {
		return TestResult{ErrorKind: "invalid_response"}
	}
	result := TestResult{Provider: endpoint.Provider, Model: endpoint.Model}
	if provider == nil {
		result.ErrorKind = "provider_unavailable"
		return result
	}
	started := time.Now()
	_, err := provider.Classify(ctx, request)
	result.LatencyMS = time.Since(started).Milliseconds()
	if err == nil {
		result.OK = true
		return result
	}
	result.ErrorKind = classifyTestError(err)
	return result
}

func fixedTestPNG() string {
	raw := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0, 144, 119, 83, 222, 0, 0, 0, 12, 73, 68, 65, 84, 8, 215, 99, 248, 207, 192, 0, 0, 3, 1, 1, 0, 24, 221, 141, 24, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}
	return base64.StdEncoding.EncodeToString(raw)
}

func classifyTestError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var httpErr ai.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.Status == 401 || httpErr.Status == 403:
			return "authentication_failed"
		case httpErr.Status == 404:
			return "model_not_found"
		case httpErr.Status >= 500:
			return "provider_unavailable"
		default:
			return "invalid_response"
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "decode") || strings.Contains(message, "invalid") || strings.Contains(message, "empty response") {
		return "invalid_response"
	}
	return "network_unavailable"
}
