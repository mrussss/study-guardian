package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"study-guardian/internal/evidence"
	"study-guardian/internal/storage"
)

const (
	StatusPending = "PENDING"
	StatusReady   = "READY"
	StatusStale   = "STALE"
	StatusFailed  = "FAILED"
)

type Service struct {
	store      *storage.Storage
	aggregator *evidence.Aggregator
	outputDir  string
	provider   Provider
	limits     ReviewLimits
	mu         sync.Mutex
}

func NewService(store *storage.Storage, timezone *time.Location, outputDir string) *Service {
	return &Service{store: store, aggregator: evidence.NewAggregator(store, timezone), outputDir: outputDir, limits: normalizeReviewLimits(ReviewLimits{})}
}

func (s *Service) SetProvider(provider Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = provider
}

func (s *Service) SetLimits(limits ReviewLimits) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = normalizeReviewLimits(limits)
}

func (s *Service) Evidence(ctx context.Context, date string) (evidence.DailyEvidenceBundle, error) {
	return s.aggregator.Build(ctx, date)
}

// BuildReviewInput is the only service entry point that prepares evidence for
// an AI provider. Aggregation reads the canonical store and Compact then
// creates a bounded copy; neither stage mutates the stored evidence.
func (s *Service) BuildReviewInput(ctx context.Context, date string, limits ReviewLimits) (ReviewInput, error) {
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return ReviewInput{}, err
	}
	return Compact(bundle, limits)
}

// MarkStaleIfChanged compares the current bounded canonical input with the
// saved review hash. Equal input is a no-op, so frequent callers do not write
// STALE on every supervisor tick.
func (s *Service) MarkStaleIfChanged(ctx context.Context, date string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return false, err
	}
	input, err := Compact(bundle, s.limits)
	if err != nil {
		return false, err
	}
	hash, err := hashReviewInput(input)
	if err != nil {
		return false, err
	}
	previous, err := s.store.LoadDailyReview(ctx, date)
	if err != nil {
		if storage.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if previous.Status != StatusReady || previous.InputHash == hash {
		return false, nil
	}
	if err := s.store.MarkDailyReviewStale(ctx, date, time.Now()); err != nil {
		return false, err
	}
	return true, nil
}

// BackfillPreviousDay is safe to call during startup. It only generates when
// the previous day has evidence and no current READY review.
func (s *Service) BackfillPreviousDay(ctx context.Context, now time.Time, enabled bool) error {
	if !enabled {
		return nil
	}
	date := now.AddDate(0, 0, -1).Format("2006-01-02")
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return err
	}
	if !hasReviewEvidence(bundle) {
		return nil
	}
	if previous, loadErr := s.store.LoadDailyReview(ctx, date); loadErr == nil && previous.Status == StatusReady {
		return nil
	}
	_, err = s.Generate(ctx, date)
	return err
}

func hasReviewEvidence(bundle evidence.DailyEvidenceBundle) bool {
	return bundle.Quality.StudyStatePresent || len(bundle.Sessions) > 0 || len(bundle.Distractions) > 0 || len(bundle.Reminders) > 0 || len(bundle.ChatTurns) > 0 || len(bundle.Semantic) > 0
}

// Generate executes the complete guarded AI path. Any provider, sanitization
// or validation failure is persisted as a deterministic fallback so the API
// remains useful while the failure is observable in ErrorCode.
func (s *Service) Generate(ctx context.Context, date string) (storage.DailyReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	input, err := Compact(bundle, s.limits)
	if err != nil {
		return s.generateFallbackLocked(ctx, bundle, "compaction_failed")
	}
	inputHash, err := hashReviewInput(input)
	if err != nil {
		return s.generateFallbackLocked(ctx, bundle, "input_hash_failed")
	}
	if previous, loadErr := s.store.LoadDailyReview(ctx, date); loadErr == nil {
		if previous.Status == StatusReady && previous.InputHash == inputHash {
			return previous, nil
		}
		if previous.Status == StatusReady && previous.InputHash != inputHash {
			_ = s.store.MarkDailyReviewStale(ctx, date, time.Now())
		}
	}
	if s.provider == nil {
		return s.generateFallbackLockedWithHash(ctx, bundle, inputHash, "provider_not_configured")
	}
	sanitized, sanitizerReport, err := Sanitize(input, s.limits.MaxFinalInputChars)
	if err != nil {
		return s.generateFallbackLockedWithHash(ctx, bundle, inputHash, "sanitizer_failed")
	}
	document, metadata, err := s.provider.Generate(ctx, sanitized)
	if err != nil {
		return s.generateFallbackLockedWithHash(ctx, bundle, inputHash, providerErrorCode(err))
	}
	validated, validationReport, err := ValidateDocument(sanitized, document)
	if err != nil {
		return s.generateFallbackLockedWithHash(ctx, bundle, inputHash, "validation_failed")
	}
	validated.EvidenceQuality = input.Quality
	validated.Warnings = appendReviewWarnings(input.Warnings, sanitizerReport.Warnings, validationReport.Warnings)
	return s.persistLocked(ctx, bundle, validated, inputHash, "AI", metadata.Provider, metadata.Model, metadata.PromptVersion, "")
}

// GenerateFallback is deterministic and offline-safe. The mutex is the single
// in-process generation gate; future AI generation must use the same gate.
func (s *Service) GenerateFallback(ctx context.Context, date string) (storage.DailyReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	return s.generateFallbackLocked(ctx, bundle, "")
}

func (s *Service) generateFallbackLocked(ctx context.Context, bundle evidence.DailyEvidenceBundle, errorCode string) (storage.DailyReviewRecord, error) {
	input, err := Compact(bundle, s.limits)
	if err != nil {
		return s.persistFallbackLocked(ctx, bundle, fallbackDocument(bundle), "", "", "", errorCode)
	}
	hash, err := hashReviewInput(input)
	if err != nil {
		return s.persistFallbackLocked(ctx, bundle, fallbackDocument(bundle), "", "", "", errorCode)
	}
	doc := BuildFallback(bundle)
	doc.EvidenceQuality = input.Quality
	doc.Warnings = appendReviewWarnings(input.Warnings)
	return s.persistFallbackLocked(ctx, bundle, doc, hash, "", "", errorCode)
}

func (s *Service) generateFallbackLockedWithHash(ctx context.Context, bundle evidence.DailyEvidenceBundle, inputHash, errorCode string) (storage.DailyReviewRecord, error) {
	return s.persistFallbackLocked(ctx, bundle, fallbackDocument(bundle), inputHash, "", "", errorCode)
}

func fallbackDocument(bundle evidence.DailyEvidenceBundle) Document {
	doc := BuildFallback(bundle)
	doc.EvidenceQuality = bundle.Quality
	doc.Warnings = appendReviewWarnings(bundle.Warnings)
	return doc
}

func (s *Service) persistFallbackLocked(ctx context.Context, bundle evidence.DailyEvidenceBundle, doc Document, inputHash, providerName, model, errorCode string) (storage.DailyReviewRecord, error) {
	return s.persistLocked(ctx, bundle, doc, inputHash, "FALLBACK", providerName, model, "fallback-v1", errorCode)
}

func (s *Service) persistLocked(ctx context.Context, bundle evidence.DailyEvidenceBundle, doc Document, inputHash, generationMode, providerName, model, promptVersion, errorCode string) (storage.DailyReviewRecord, error) {
	reviewJSON, err := json.Marshal(doc)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	if inputHash == "" {
		inputHash, err = hashBundle(bundle)
		if err != nil {
			return storage.DailyReviewRecord{}, err
		}
	}
	now := time.Now()
	revision := 1
	attemptCount := 1
	if previous, loadErr := s.store.LoadDailyReview(ctx, bundle.Date); loadErr == nil {
		revision = previous.Revision + 1
		attemptCount = previous.AttemptCount + 1
	}
	markdown := RenderMarkdown(doc, bundle)
	record := storage.DailyReviewRecord{
		Date: bundle.Date, Status: StatusPending, GenerationMode: generationMode, Revision: revision,
		InputHash: inputHash, SchemaVersion: 1, PromptVersion: promptVersion,
		Provider: providerName, Model: model,
		ReviewJSON: string(reviewJSON), Markdown: markdown, AttemptCount: attemptCount,
		StartedAt: &now, UpdatedAt: now, ErrorCode: errorCode,
	}
	if err := s.store.SaveDailyReview(ctx, record); err != nil {
		return storage.DailyReviewRecord{}, err
	}
	if s.outputDir != "" {
		if err := writeMarkdownAtomic(s.outputDir, bundle.Date, markdown); err != nil {
			return storage.DailyReviewRecord{}, err
		}
	}
	if err := s.store.MarkDailyReviewReady(ctx, bundle.Date, revision, now, now, errorCode); err != nil {
		return storage.DailyReviewRecord{}, err
	}
	record.Status = StatusReady
	record.GeneratedAt = &now
	if _, _, err := s.store.RecordDailyReviewReady(ctx, bundle.Date, record.Revision, generationMode, now); err != nil {
		return storage.DailyReviewRecord{}, err
	}
	return record, nil
}

func hashReviewInput(input ReviewInput) (string, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func hashBundle(bundle evidence.DailyEvidenceBundle) (string, error) {
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func providerErrorCode(err error) string {
	var providerErr ProviderError
	if errors.As(err, &providerErr) {
		return string(providerErr.Kind)
	}
	return "provider_failed"
}

func appendReviewWarnings(groups ...[]string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, warning := range group {
			if warning == "" {
				continue
			}
			if _, exists := seen[warning]; exists {
				continue
			}
			seen[warning] = struct{}{}
			out = append(out, warning)
		}
	}
	return out
}

func (s *Service) Get(ctx context.Context, date string) (storage.DailyReviewRecord, error) {
	return s.store.LoadDailyReview(ctx, date)
}

func (s *Service) Exclude(ctx context.Context, date, sourceType, sourceID string) error {
	if sourceType != "chat_turn" && sourceType != "chat_conversation" {
		return fmt.Errorf("unsupported review exclusion type: %s", sourceType)
	}
	if err := s.store.AddReviewExclusion(ctx, storage.ReviewExclusionRecord{Date: date, SourceType: sourceType, SourceID: sourceID, CreatedAt: time.Now()}); err != nil {
		return err
	}
	return s.store.MarkDailyReviewStale(ctx, date, time.Now())
}

func (s *Service) Delete(ctx context.Context, date string) error {
	if err := s.store.DeleteDailyReview(ctx, date); err != nil {
		return err
	}
	if s.outputDir != "" {
		if err := os.Remove(filepath.Join(s.outputDir, date+".md")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeMarkdownAtomic(outputDir, date, markdown string) error {
	if date == "" {
		return fmt.Errorf("review date is required")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(outputDir, "."+date+".md.*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.WriteString(markdown); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	targetName := filepath.Join(outputDir, date+".md")
	if err := os.Rename(tempName, targetName); err != nil {
		// Windows does not replace an existing file with Rename. Keep a
		// recoverable sibling while swapping the fully flushed temp file.
		backupName := targetName + ".replace-backup"
		_ = os.Remove(backupName)
		if moveErr := os.Rename(targetName, backupName); moveErr != nil {
			return err
		}
		if moveErr := os.Rename(tempName, targetName); moveErr != nil {
			_ = os.Rename(backupName, targetName)
			return moveErr
		}
		_ = os.Remove(backupName)
	}
	committed = true
	return nil
}
