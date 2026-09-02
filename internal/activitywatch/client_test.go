package activitywatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
