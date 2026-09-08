package review

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	GenerationIdle    = "IDLE"
	GenerationPending = "PENDING"
	GenerationReady   = "READY"
	GenerationFailed  = "FAILED"

	defaultGenerationTimeout = 110 * time.Second
	defaultFallbackTimeout   = 5 * time.Second
)

// GenerationStatus is deliberately a small, secret-free status DTO. It does
// not contain review content, evidence, prompts, provider response text or
// credentials.
type GenerationStatus struct {
	Accepted       bool   `json:"accepted,omitempty"`
	AlreadyRunning bool   `json:"already_running,omitempty"`
	GenerationID   string `json:"generation_id,omitempty"`
	Date           string `json:"date"`
	State          string `json:"state"`
	GenerationMode string `json:"generation_mode,omitempty"`
	ErrorKind      string `json:"error_kind,omitempty"`
	Revision       int    `json:"revision,omitempty"`
}

type generationJob struct {
	status GenerationStatus
}

// Coordinator owns the asynchronous lifecycle. There is at most one running
// generation for a date, regardless of whether it was started manually, by
// the OFF trigger, or by startup backfill.
type Coordinator struct {
	service         *Service
	totalTimeout    time.Duration
	fallbackTimeout time.Duration
	ctx             context.Context
	cancel          context.CancelFunc

	mu     sync.Mutex
	jobs   map[string]*generationJob
	closed bool
	wg     sync.WaitGroup
	seq    uint64
}

func NewCoordinator(service *Service, totalTimeout, fallbackTimeout time.Duration) *Coordinator {
	if totalTimeout <= 0 {
		totalTimeout = defaultGenerationTimeout
	}
	if fallbackTimeout <= 0 {
		fallbackTimeout = defaultFallbackTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Coordinator{
		service: service, totalTimeout: totalTimeout, fallbackTimeout: fallbackTimeout,
		ctx: ctx, cancel: cancel, jobs: make(map[string]*generationJob),
	}
}

// NormalizeDate accepts only an exact local YYYY-MM-DD date. An empty date
// means today, which keeps the API convenient without accepting ambiguous
// values.
func NormalizeDate(raw string) (string, error) {
	date := strings.TrimSpace(raw)
	if date == "" {
		return time.Now().Format("2006-01-02"), nil
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return "", fmt.Errorf("invalid review date")
	}
	return date, nil
}

func (c *Coordinator) Start(date string) (GenerationStatus, error) {
	if c == nil || c.service == nil {
		return GenerationStatus{}, errors.New("review coordinator is unavailable")
	}
	date, err := NormalizeDate(date)
	if err != nil {
		return GenerationStatus{}, err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return GenerationStatus{}, errors.New("review coordinator is closed")
	}
	if existing := c.jobs[date]; existing != nil && existing.status.State == GenerationPending {
		status := existing.status
		status.Accepted = true
		status.AlreadyRunning = true
		c.mu.Unlock()
		return status, nil
	}
	status := GenerationStatus{Accepted: true, Date: date, State: GenerationPending, GenerationID: c.nextID()}
	job := &generationJob{status: status}
	c.jobs[date] = job
	c.wg.Add(1)
	c.mu.Unlock()

	go c.run(date, job)
	return status, nil
}

func (c *Coordinator) nextID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%x-%x", time.Now().UnixNano(), atomic.AddUint64(&c.seq, 1))
}

func (c *Coordinator) Status(date string) (GenerationStatus, error) {
	if c == nil || c.service == nil {
		return GenerationStatus{}, errors.New("review coordinator is unavailable")
	}
	date, err := NormalizeDate(date)
	if err != nil {
		return GenerationStatus{}, err
	}
	c.mu.Lock()
	if job := c.jobs[date]; job != nil {
		status := job.status
		c.mu.Unlock()
		return status, nil
	}
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return GenerationStatus{Date: date, State: GenerationIdle}, nil
	}

	// A persisted result remains observable after a Supervisor restart. STALE
	// is content state rather than an active generation state, so it is exposed
	// as READY here; the daily review endpoint still carries the exact status.
	record, loadErr := c.service.Get(context.Background(), date)
	if loadErr == nil && (record.Status == StatusReady || record.Status == StatusStale) {
		return GenerationStatus{Date: date, State: GenerationReady, GenerationMode: record.GenerationMode, Revision: record.Revision}, nil
	}
	return GenerationStatus{Date: date, State: GenerationIdle}, nil
}

func (c *Coordinator) run(date string, job *generationJob) {
	defer c.wg.Done()
	ctx, cancel := context.WithTimeout(c.ctx, c.totalTimeout)
	defer cancel()
	fallbackBase := context.WithoutCancel(ctx)
	fallbackCtx, cancelFallback := context.WithTimeout(fallbackBase, c.fallbackTimeout)
	defer cancelFallback()

	record, err := c.service.GenerateWithFallbackContext(ctx, fallbackCtx, date)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.jobs[date] != job {
		return
	}
	if c.ctx.Err() != nil {
		job.status.State = GenerationFailed
		job.status.ErrorKind = "canceled"
		return
	}
	if err != nil {
		job.status.State = GenerationFailed
		job.status.ErrorKind = generationErrorKind(err)
		return
	}
	job.status.State = GenerationReady
	job.status.GenerationMode = normalizeGenerationMode(record.GenerationMode)
	job.status.Revision = record.Revision
	job.status.ErrorKind = normalizeErrorKind(record.ErrorCode)
}

// StartPreviousDayIfNeeded routes startup backfill through this coordinator,
// keeping it single-flight with manual and OFF-triggered generation.
func (c *Coordinator) StartPreviousDayIfNeeded(ctx context.Context, now time.Time, enabled bool) (GenerationStatus, error) {
	if !enabled {
		return GenerationStatus{}, nil
	}
	date := now.AddDate(0, 0, -1).Format("2006-01-02")
	bundle, err := c.service.Evidence(ctx, date)
	if err != nil {
		return GenerationStatus{}, err
	}
	if !hasReviewEvidence(bundle) {
		return GenerationStatus{}, nil
	}
	if previous, loadErr := c.service.Get(ctx, date); loadErr == nil && previous.Status == StatusReady {
		return GenerationStatus{Date: date, State: GenerationReady, GenerationMode: previous.GenerationMode, Revision: previous.Revision}, nil
	}
	return c.Start(date)
}

func (c *Coordinator) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.wg.Wait()
}

func generationErrorKind(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var providerErr ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.FailureKind != "" {
			return normalizeErrorKind(string(providerErr.FailureKind))
		}
		return normalizeErrorKind(string(providerErr.Kind))
	}
	return "generation_failed"
}

func normalizeGenerationMode(value string) string {
	if value == "AI" || value == "FALLBACK" {
		return value
	}
	return ""
}

func normalizeErrorKind(value string) string {
	switch value {
	case "timeout", "provider_timeout", "model_timeout":
		return "timeout"
	case "authentication":
		return "authentication_failed"
	case "canceled", "network_unreachable", "proxy_unreachable", "tls_failed", "provider_unavailable", "authentication_failed", "model_not_found", "model_unavailable", "model_rate_limited", "account_rate_limited", "invalid_output", "storage_unavailable", "provider_not_configured", "compaction_failed", "input_hash_failed", "sanitizer_failed", "validation_failed", "generation_failed":
		return value
	default:
		if value == "" {
			return ""
		}
		return "generation_failed"
	}
}
