package classifier

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/rules"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type Service struct {
	cfg         *config.Config
	ruleEngine  *rules.RuleEngine
	privacyGate *rules.PrivacyGate
	provider    TaskRelationProvider
	storage     *storage.Storage
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
		ruleEngine:  ruleEngine,
		privacyGate: privacyGate,
		provider:    provider,
		storage:     store,
	}
}

func (s *Service) Classify(
	ctx context.Context,
	app, title, domain, task, screenHash string,
	userMode string,
	imageBase64 string,
) state.ClassificationResult {
	// 1. Local deterministic rules first!
	ruleRes := s.ruleEngine.Classify(app, title, domain, task)
	minConf := s.cfg.AI.MinConfidence
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

	// 3. Check Classification Cache
	cacheKey := computeCacheKey(app, title, domain, task, screenHash)
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
	if !s.cfg.AI.Enabled || s.provider == nil {
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
	if s.cfg.AI.UseVisionOnlyWhenNeeded && imageBase64 != "" {
		aiReq.AnalysisImageBase64 = imageBase64
	}

	aiCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := s.provider.Classify(aiCtx, aiReq)
	if err != nil {
		log.Printf("[Classifier] AI Provider (%s) failed or timed out: %v; falling back to rules", s.provider.Name(), err)
		return ruleRes
	}

	// 6. Save in cache
	if s.storage != nil && resp.Confidence >= 0.70 {
		_ = s.storage.SetClassificationCache(ctx, cacheKey, string(resp.Relation), resp.Confidence, resp.ReasonShort, now, now.Add(10*time.Minute))
	}

	return state.ClassificationResult{
		Relation:   resp.Relation,
		Confidence: resp.Confidence,
		Reason:     fmt.Sprintf("[AI %s] %s: %s", s.provider.Name(), resp.Activity, resp.ReasonShort),
		IsFromRule: false,
	}
}

func computeCacheKey(app, title, domain, task, screenHash string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s", app, title, domain, task, screenHash)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}
