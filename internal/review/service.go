package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	mu         sync.Mutex
}

func NewService(store *storage.Storage, timezone *time.Location, outputDir string) *Service {
	return &Service{store: store, aggregator: evidence.NewAggregator(store, timezone), outputDir: outputDir}
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

// GenerateFallback is deterministic and offline-safe. The mutex is the single
// in-process generation gate; future AI generation must use the same gate.
func (s *Service) GenerateFallback(ctx context.Context, date string) (storage.DailyReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, err := s.aggregator.Build(ctx, date)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	doc := BuildFallback(bundle)
	reviewJSON, err := json.Marshal(doc)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	input, err := json.Marshal(bundle)
	if err != nil {
		return storage.DailyReviewRecord{}, err
	}
	hash := sha256.Sum256(input)
	now := time.Now()
	revision := 1
	if previous, loadErr := s.store.LoadDailyReview(ctx, date); loadErr == nil {
		revision = previous.Revision + 1
	}
	markdown := RenderMarkdown(doc, bundle)
	record := storage.DailyReviewRecord{
		Date: date, Status: StatusReady, GenerationMode: "FALLBACK", Revision: revision,
		InputHash: hex.EncodeToString(hash[:]), SchemaVersion: 1, PromptVersion: "fallback-v1",
		ReviewJSON: string(reviewJSON), Markdown: markdown, AttemptCount: 1,
		GeneratedAt: &now, UpdatedAt: now,
	}
	if err := s.store.SaveDailyReview(ctx, record); err != nil {
		return storage.DailyReviewRecord{}, err
	}
	if s.outputDir != "" {
		if err := writeMarkdownAtomic(s.outputDir, date, markdown); err != nil {
			return storage.DailyReviewRecord{}, err
		}
	}
	return record, nil
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
