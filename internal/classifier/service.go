package classifier

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/rules"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type Service struct {
	mu          sync.RWMutex
	cfg         *config.Config
	aiConfig    config.AIConfig
	ruleEngine  *rules.RuleEngine
	privacyGate *rules.PrivacyGate
	provider    TaskRelationProvider
	vision      TaskRelationProvider
	storage     *storage.Storage
}

func NewServiceWithProviders(cfg *config.Config, ruleEngine *rules.RuleEngine, privacyGate *rules.PrivacyGate, textProvider, visionProvider TaskRelationProvider, store *storage.Storage) *Service {
	s := NewService(cfg, ruleEngine, privacyGate, textProvider, store)
	s.vision = visionProvider
	return s
}

func NewService(
	cfg *config.Config,
	ruleEngine *rules.RuleEngine,
	privacyGate *rules.PrivacyGate,
	provider TaskRelationProvider,
	store *storage.Storage,
) *Service {
	return &Service{
		cfg:         cfg,
		aiConfig:    cfg.AI,
		ruleEngine:  ruleEngine,
		privacyGate: privacyGate,
		provider:    provider,
		storage:     store,
	}
}

func (s *Service) UpdateAI(aiConfig config.AIConfig, textProvider, visionProvider TaskRelationProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aiConfig = aiConfig
	s.provider = textProvider
	s.vision = visionProvider
}

func (s *Service) Classify(
	ctx context.Context,
	app, title, domain, task, screenHash string,
	userMode string,
	imageBase64 string,
) state.ClassificationResult {
	// 1. Local deterministic rules first!
	ruleRes := s.ruleEngine.Classify(app, title, domain, task)
	s.mu.RLock()
	aiConfig := s.aiConfig
	textProvider := s.provider
	visionProvider := s.vision
	s.mu.RUnlock()
	minConf := aiConfig.MinConfidence
	if minConf <= 0 {
		minConf = 0.75
	}

	// If rule gave definitive answer (FOCUSED or DISTRACTED) with high confidence, use it immediately
	if ruleRes.Relation != state.RelationUnknown && ruleRes.Confidence >= minConf {
		return ruleRes
	}

	// 2. Check Privacy Gate before consulting AI
	privacy := s.privacyGate.Evaluate(app, title, domain)
	if privacy == state.PrivacySensitive {
		// Sensitive window: NEVER query AI with image or sensitive metadata
		return state.ClassificationResult{
			Relation:   ruleRes.Relation,
			Confidence: ruleRes.Confidence,
			Reason:     "Privacy Gate flagged sensitive window; AI analysis bypassed",
			IsFromRule: true,
		}
	}

	// The caller decides when a screenshot is worth collecting. Supplying an
	// image here explicitly selects the configured vision fallback provider.
	requestedVision := imageBase64 != "" && visionProvider != nil
	provider := textProvider
	if requestedVision {
		provider = visionProvider
	}
	providerName := "none"
	if provider != nil {
		providerName = provider.Name()
	}

	// 3. Check Classification Cache
	cacheKey := computeCacheKey(providerName, app, title, domain, task, screenHash, requestedVision)
	now := time.Now()
	if s.storage != nil {
		if rel, conf, reason, found := s.storage.GetClassificationCache(ctx, cacheKey, now); found {
			return state.ClassificationResult{
				Relation:   state.TaskRelation(rel),
				Confidence: conf,
				Reason:     reason + " (cached)",
				IsFromRule: false,
			}
		}
	}

	// 4. If AI is not enabled or no provider configured, return rule fallback
	if !aiConfig.Enabled || provider == nil {
		return ruleRes
	}

	// 5. Query AI Provider
	aiReq := ClassificationRequest{
		Task:     task,
		App:      app,
		Title:    title,
		Domain:   domain,
		UserMode: userMode,
	}
	if requestedVision {
		aiReq.AnalysisImageBase64 = imageBase64
	}

	timeoutSeconds := aiConfig.Text.TimeoutSeconds
	if requestedVision {
		timeoutSeconds = aiConfig.Vision.TimeoutSeconds
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 6
		if requestedVision {
			timeoutSeconds = 8
		}
	}
	aiCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	resp, err := provider.Classify(aiCtx, aiReq)
	if err != nil {
		log.Printf("[Classifier] AI Provider (%s) failed or timed out: %v; falling back to rules", provider.Name(), err)
		return ruleRes
	}
	if err := validateClassificationResponse(resp); err != nil {
		log.Printf("[Classifier] AI Provider (%s) returned invalid data: %v; falling back to rules", provider.Name(), err)
		return ruleRes
	}

	// 6. Save in cache
	if s.storage != nil && resp.Confidence >= 0.70 {
		_ = s.storage.SetClassificationCache(ctx, cacheKey, string(resp.Relation), resp.Confidence, resp.ReasonShort, now, now.Add(10*time.Minute))
	}

	return state.ClassificationResult{
		Relation:   resp.Relation,
		Confidence: resp.Confidence,
		Reason:     fmt.Sprintf("[AI %s] %s: %s", provider.Name(), resp.Activity, resp.ReasonShort),
		IsFromRule: false,
	}
}

func computeCacheKey(provider, app, title, domain, task, screenHash string, vision bool) string {
	// Text classification is independent of screen pixels; vision is not.
	if !vision {
		screenHash = ""
	}
	kind := "text"
	if vision {
		kind = "vision"
	}
	normalize := func(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", kind, normalize(provider), normalize(app), normalize(title), normalize(domain), normalize(task), normalize(screenHash))
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}
