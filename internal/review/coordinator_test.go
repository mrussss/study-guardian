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
	coordinator := NewCoordinator(service, time.Second, 100*time.Millisecond)
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
	coordinator := NewCoordinator(service, 20*time.Millisecond, 200*time.Millisecond)
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

func TestCoordinatorCloseCancelsRunningGeneration(t *testing.T) {
	provider := &blockingReviewProvider{}
	service, store := newCoordinatorTestService(t, provider)
	defer store.Close()
	coordinator := NewCoordinator(service, time.Minute, 100*time.Millisecond)
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
