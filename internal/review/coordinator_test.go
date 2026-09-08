package review

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"study-guardian/internal/storage"
)

type blockingReviewProvider struct {
	mu    sync.Mutex
	calls int
}

type timedBlockingReviewProvider struct {
	started  chan struct{}
	duration chan time.Duration
}

func (p *blockingReviewProvider) Generate(ctx context.Context, input ReviewInput) (Document, ProviderMetadata, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	<-ctx.Done()
	return Document{}, ProviderMetadata{Provider: "test", Model: "blocking", PromptVersion: ReviewPromptVersion}, ctx.Err()
}

func (p *blockingReviewProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *timedBlockingReviewProvider) Generate(ctx context.Context, input ReviewInput) (Document, ProviderMetadata, error) {
	startedAt := time.Now()
	select {
	case p.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	p.duration <- time.Since(startedAt)
	return Document{}, ProviderMetadata{Provider: "test", Model: "blocking", PromptVersion: ReviewPromptVersion}, ctx.Err()
}

func waitForProviderCall(t *testing.T, provider *blockingReviewProvider) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for provider.Calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if provider.Calls() == 0 {
		t.Fatal("provider did not start")
	}
}

func newCoordinatorTestService(t *testing.T, provider Provider) (*Service, *storage.Storage) {
	t.Helper()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, time.UTC, t.TempDir())
	service.SetProvider(provider)
	return service, store
}

func TestCoordinatorSingleFlightsManualAndAutoByDate(t *testing.T) {
	provider := &blockingReviewProvider{}
	service, store := newCoordinatorTestService(t, provider)
	defer store.Close()
	coordinator := NewCoordinator(service, time.Second)
	first, err := coordinator.Start("2026-09-07")
	if err != nil || first.State != GenerationPending || first.GenerationID == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := coordinator.Start("2026-09-07")
	if err != nil || !second.AlreadyRunning || second.GenerationID != first.GenerationID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	waitForProviderCall(t, provider)
	if provider.Calls() != 1 {
		t.Fatalf("provider calls=%d, want one", provider.Calls())
	}
	coordinator.Close()
}

func TestCoordinatorPersistsFallbackAfterGenerationDeadline(t *testing.T) {
	provider := &blockingReviewProvider{}
	service, store := newCoordinatorTestService(t, provider)
	defer store.Close()
	coordinator := NewCoordinator(service, 20*time.Millisecond)
	status, err := coordinator.Start("2026-09-07")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, statusErr := coordinator.Status("2026-09-07")
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if current.State == GenerationReady {
			if current.GenerationMode != "FALLBACK" || current.ErrorKind != "timeout" {
				t.Fatalf("fallback status=%+v", current)
			}
			record, loadErr := service.Get(context.Background(), "2026-09-07")
			if loadErr != nil || record.GenerationMode != "FALLBACK" {
				t.Fatalf("record=%+v err=%v", record, loadErr)
			}
			coordinator.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	coordinator.Close()
	t.Fatalf("generation %s did not settle", status.GenerationID)
}

func TestCoordinatorStartsFallbackTimeoutAfterAIDeadline(t *testing.T) {
	provider := &timedBlockingReviewProvider{started: make(chan struct{}, 1), duration: make(chan time.Duration, 1)}
	service, store := newCoordinatorTestService(t, provider)
	defer store.Close()
	service.fallbackPersistenceTimeout = 20 * time.Millisecond
	date := "2026-09-07"
	first, err := service.GenerateFallback(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordSemanticSnapshot(context.Background(), storage.SemanticSnapshotRecord{
		ObservedAt: time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC), LocalDate: date,
		Relation: "FOCUSED", Confidence: .9, Activity: "READING", SourceKind: "LOCAL_RULE",
	}); err != nil {
		t.Fatal(err)
	}

	coordinator := NewCoordinator(service, 60*time.Millisecond)
	defer coordinator.Close()
	if _, err := coordinator.Start(date); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	aiElapsed := <-provider.duration
	if aiElapsed <= service.fallbackPersistenceTimeout {
		t.Fatalf("AI path elapsed=%s, want longer than fallback timeout=%s", aiElapsed, service.fallbackPersistenceTimeout)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := coordinator.Status(date)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status.State == GenerationReady {
			if status.GenerationMode != "FALLBACK" || status.ErrorKind != "timeout" || status.Revision != first.Revision+1 {
				t.Fatalf("generation status=%+v first=%+v", status, first)
			}
			record, loadErr := service.Get(context.Background(), date)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if record.Status != StatusReady || record.GenerationMode != "FALLBACK" || record.Revision != first.Revision+1 || record.ErrorCode != "timeout" {
				t.Fatalf("fallback record=%+v first=%+v", record, first)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("fallback did not settle after the AI deadline")
}

func TestCoordinatorCloseCancelsRunningGeneration(t *testing.T) {
	provider := &blockingReviewProvider{}
	service, store := newCoordinatorTestService(t, provider)
	defer store.Close()
	coordinator := NewCoordinator(service, time.Minute)
	if _, err := coordinator.Start("2026-09-07"); err != nil {
		t.Fatal(err)
	}
	waitForProviderCall(t, provider)
	coordinator.Close()
	if provider.Calls() != 1 {
		t.Fatalf("provider calls=%d", provider.Calls())
	}
	status, err := coordinator.Status("2026-09-07")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != GenerationFailed || status.ErrorKind != "canceled" {
		t.Fatalf("shutdown status=%+v", status)
	}
}

func TestNormalizeDateRejectsAmbiguousValues(t *testing.T) {
	for _, value := range []string{"2026/09/07", "2026-2-07", "2026-02-30", "tomorrow"} {
		if _, err := NormalizeDate(value); err == nil {
			t.Fatalf("accepted invalid date %q", value)
		}
	}
	if value, err := NormalizeDate("2026-09-07"); err != nil || value != "2026-09-07" {
		t.Fatalf("normalized=%q err=%v", value, err)
	}
}

func TestCoordinatorErrorClassificationDoesNotExposeProviderDetails(t *testing.T) {
	if got := generationErrorKind(errors.New("provider response secret")); got != "generation_failed" {
		t.Fatalf("kind=%q", got)
	}
}
