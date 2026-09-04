package activitywatch

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestActivityWatchClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/0/info":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v0.13.2"})
		case "/api/0/buckets/":
			w.Header().Set("Content-Type", "application/json")
			buckets := map[string]BucketInfo{
				"aw-watcher-window_HOST": {
					ID:   "aw-watcher-window_HOST",
					Type: "currentwindow",
				},
				"aw-watcher-afk_HOST": {
					ID:   "aw-watcher-afk_HOST",
					Type: "afkstatus",
				},
				"aw-watcher-web_HOST": {
					ID:   "aw-watcher-web_HOST",
					Type: "web.tab.current",
				},
			}
			_ = json.NewEncoder(w).Encode(buckets)
		case "/api/0/buckets/aw-watcher-window_HOST/events":
			w.Header().Set("Content-Type", "application/json")
			events := []Event{
				{
					ID:        1,
					Timestamp: "2026-09-02T10:00:00Z",
					Duration:  10.5,
					Data: map[string]interface{}{
						"app":   "Code.exe",
						"title": "main.go - study-guardian",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(events)
		case "/api/0/buckets/aw-watcher-afk_HOST/events":
			w.Header().Set("Content-Type", "application/json")
			events := []Event{
				{
					ID:        2,
					Timestamp: "2026-09-02T10:00:00Z",
					Duration:  10.5,
					Data: map[string]interface{}{
						"status": "not-afk",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(events)
		case "/api/0/buckets/aw-watcher-web_HOST/events":
			w.Header().Set("Content-Type", "application/json")
			events := []Event{
				{
					ID:        3,
					Timestamp: "2026-09-02T10:00:00Z",
					Duration:  5.0,
					Data: map[string]interface{}{
						"url":   "https://pkg.go.dev/net/http",
						"title": "http package - net/http - Go Packages",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(events)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	// Health check
	if !client.Health(context.Background()) {
		t.Fatalf("expected Health to return true")
	}

	// Discover and get latest activity
	snap, err := client.GetLatestActivity(context.Background())
	if err != nil {
		t.Fatalf("failed to get latest activity: %v", err)
	}

	if snap.App != "Code.exe" || snap.Title != "main.go - study-guardian" {
		t.Fatalf("unexpected window data: %+v", snap)
	}
	if snap.IsAFK {
		t.Fatalf("expected not AFK, got AFK")
	}
	if snap.Domain != "pkg.go.dev" {
		t.Fatalf("expected domain pkg.go.dev from web watcher, got %s", snap.Domain)
	}
	if snap.Timestamp.IsZero() || snap.Timestamp.Format(time.RFC3339) != "2026-09-02T10:00:00Z" {
		t.Fatalf("expected timestamp from real ActivityWatch event, got %v", snap.Timestamp)
	}
	if snap.WindowEventTimestamp.IsZero() || snap.AFKEventTimestamp.IsZero() || snap.WebEventTimestamp.IsZero() {
		t.Fatalf("expected per-watcher timestamps, got %+v", snap)
	}
	if snap.IsFresh(time.Date(2026, 9, 2, 10, 1, 0, 0, time.UTC), 2*time.Minute) == false {
		t.Fatal("expected recent ActivityWatch event to be fresh")
	}
	if snap.IsFresh(time.Date(2026, 9, 2, 10, 5, 0, 0, time.UTC), 2*time.Minute) {
		t.Fatal("expected old ActivityWatch event to be stale")
	}
}

func TestActivityWatchOfflineFailSoft(t *testing.T) {
	client := NewClient("http://127.0.0.1:56999")
	if client.Health(context.Background()) {
		t.Fatalf("expected Health to return false for offline port")
	}
	_, err := client.GetLatestActivity(context.Background())
	if err == nil {
		t.Fatalf("expected error when getting activity from offline port, got nil")
	}
}

func TestActivitySnapshotHeartbeatFreshness(t *testing.T) {
	base := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	maxAge := 10 * time.Second
	tests := []struct {
		name     string
		duration float64
		now      time.Time
		want     bool
	}{
		{"heartbeat still active", 299, base.Add(5 * time.Minute), true},
		{"stopped event stale", 30, base.Add(5 * time.Minute), false},
		{"duration covers now", 60, base.Add(45 * time.Second), true},
		{"future timestamp fail closed", 1, base.Add(-time.Second), false},
		{"negative duration fail soft", -1, base.Add(11 * time.Second), false},
		{"nan duration fail soft", math.NaN(), base.Add(11 * time.Second), false},
		{"infinite duration fail soft", math.Inf(1), base.Add(11 * time.Second), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := &ActivitySnapshot{Timestamp: base, Duration: test.duration}
			if got := snapshot.IsFresh(test.now, maxAge); got != test.want {
				t.Fatalf("IsFresh()=%v, want %v (end=%v)", got, test.want, snapshot.EffectiveEnd())
			}
		})
	}
	if got := (*ActivitySnapshot)(nil).EffectiveEnd(); !got.IsZero() {
		t.Fatalf("nil EffectiveEnd=%v, want zero", got)
	}
}

func TestActivityWatchClientWorksWithoutWebWatcher(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/0/buckets/":
			_ = json.NewEncoder(w).Encode(map[string]BucketInfo{
				"aw-watcher-window_MATRIX": {ID: "aw-watcher-window_MATRIX", Type: "currentwindow"},
				"aw-watcher-afk_MATRIX":    {ID: "aw-watcher-afk_MATRIX", Type: "afkstatus"},
			})
		case "/api/0/buckets/aw-watcher-window_MATRIX/events":
			_ = json.NewEncoder(w).Encode([]Event{{Timestamp: now, Duration: 60, Data: map[string]interface{}{
				"app": "Code.exe", "title": "main.go", "url": "https://pkg.go.dev/net/http",
			}}})
		case "/api/0/buckets/aw-watcher-afk_MATRIX/events":
			_ = json.NewEncoder(w).Encode([]Event{{Timestamp: now, Duration: 60, Data: map[string]interface{}{"status": "not-afk"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	snapshot, err := NewClient(ts.URL).GetLatestActivity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.App != "Code.exe" || snapshot.Domain != "pkg.go.dev" || snapshot.IsAFK {
		t.Fatalf("window/afk-only snapshot was not usable: %+v", snapshot)
	}
	if !snapshot.WebEventTimestamp.IsZero() {
		t.Fatalf("web timestamp should remain empty when web watcher is absent: %+v", snapshot)
	}
}

func TestActivityWatchClientRecoversAndRediscoversAfterTemporaryStop(t *testing.T) {
	var mu sync.Mutex
	available := true
	bucketRequests := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isAvailable := available
		if r.URL.Path == "/api/0/buckets/" {
			bucketRequests++
		}
		mu.Unlock()
		if !isAvailable {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/api/0/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v0.13.2"})
		case "/api/0/buckets/":
			_ = json.NewEncoder(w).Encode(map[string]BucketInfo{
				"aw-watcher-window_RECOVERY": {ID: "aw-watcher-window_RECOVERY", Type: "currentwindow"},
				"aw-watcher-afk_RECOVERY":    {ID: "aw-watcher-afk_RECOVERY", Type: "afkstatus"},
			})
		case "/api/0/buckets/aw-watcher-window_RECOVERY/events":
			_ = json.NewEncoder(w).Encode([]Event{{Timestamp: now, Duration: 60, Data: map[string]interface{}{"app": "Code.exe", "title": "recovered.go"}}})
		case "/api/0/buckets/aw-watcher-afk_RECOVERY/events":
			_ = json.NewEncoder(w).Encode([]Event{{Timestamp: now, Duration: 60, Data: map[string]interface{}{"status": "not-afk"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	if _, err := client.GetLatestActivity(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	firstDiscoveries := bucketRequests
	available = false
	mu.Unlock()
	if client.Health(context.Background()) {
		t.Fatal("expected temporary ActivityWatch stop to be unhealthy")
	}
	mu.Lock()
	available = true
	client.lastDiscover = time.Now().Add(-6 * time.Minute)
	mu.Unlock()
	if !client.Health(context.Background()) {
		t.Fatal("expected ActivityWatch recovery to become healthy")
	}
	snapshot, err := client.GetLatestActivity(context.Background())
	if err != nil || snapshot.Title != "recovered.go" {
		t.Fatalf("recovery snapshot=%+v err=%v", snapshot, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if bucketRequests <= firstDiscoveries {
		t.Fatalf("expected bucket rediscovery after recovery, before=%d after=%d", firstDiscoveries, bucketRequests)
	}
}
