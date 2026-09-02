package activitywatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ActivitySnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	App       string    `json:"app"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Domain    string    `json:"domain"`
	IsAFK     bool      `json:"is_afk"`
	Duration  float64   `json:"duration"`
}

type ActivitySource interface {
	GetLatestActivity(ctx context.Context) (*ActivitySnapshot, error)
	Health(ctx context.Context) bool
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex

	windowBucket string
	afkBucket    string
	webBucket    string
	lastDiscover time.Time
}

type BucketInfo struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Client   string `json:"client"`
	Hostname string `json:"hostname"`
}

type Event struct {
	ID        int64                  `json:"id"`
	Timestamp string                 `json:"timestamp"`
	Duration  float64                `json:"duration"`
	Data      map[string]interface{} `json:"data"`
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:5600"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (c *Client) Health(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/0/info", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *Client) DiscoverBuckets(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/0/buckets/", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach ActivityWatch at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ActivityWatch buckets returned status %d", resp.StatusCode)
	}

	var buckets map[string]BucketInfo
	if err := json.NewDecoder(resp.Body).Decode(&buckets); err != nil {
		return fmt.Errorf("failed to decode buckets json: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.windowBucket = ""
	c.afkBucket = ""
	c.webBucket = ""

	for id, b := range buckets {
		if b.Type == "currentwindow" || strings.HasPrefix(id, "aw-watcher-window") {
			c.windowBucket = id
		} else if b.Type == "afkstatus" || strings.HasPrefix(id, "aw-watcher-afk") {
			c.afkBucket = id
		} else if b.Type == "web.tab.current" || strings.HasPrefix(id, "aw-watcher-web") {
			c.webBucket = id
		}
	}
	c.lastDiscover = time.Now()

	if c.windowBucket == "" && c.afkBucket == "" {
		return fmt.Errorf("neither window nor afk bucket discovered in ActivityWatch")
	}

	return nil
}

func (c *Client) GetLatestActivity(ctx context.Context) (*ActivitySnapshot, error) {
	c.mu.RLock()
	needDiscover := (c.windowBucket == "" && c.afkBucket == "") || time.Since(c.lastDiscover) > 5*time.Minute
	c.mu.RUnlock()

	if needDiscover {
		if err := c.DiscoverBuckets(ctx); err != nil {
			return nil, err
		}
	}

	c.mu.RLock()
	winB := c.windowBucket
	afkB := c.afkBucket
	webB := c.webBucket
	c.mu.RUnlock()

	snapshot := &ActivitySnapshot{
		Timestamp: time.Now(),
		IsAFK:     false,
	}

	// 1. Fetch latest AFK event
	if afkB != "" {
		events, err := c.fetchLatestEvents(ctx, afkB, 1)
		if err == nil && len(events) > 0 {
			ev := events[0]
			if status, ok := ev.Data["status"].(string); ok {
				snapshot.IsAFK = (status == "afk")
			}
		}
	}

	// 2. Fetch latest Window event
	if winB != "" {
		events, err := c.fetchLatestEvents(ctx, winB, 1)
		if err == nil && len(events) > 0 {
			ev := events[0]
			snapshot.Duration = ev.Duration
			if app, ok := ev.Data["app"].(string); ok {
				snapshot.App = app
			}
			if title, ok := ev.Data["title"].(string); ok {
				snapshot.Title = title
			}
			if u, ok := ev.Data["url"].(string); ok {
				snapshot.URL = u
				snapshot.Domain = extractDomain(u)
			}
		}
	}

	// 3. Fetch latest Web event if URL not already present from window
	if snapshot.URL == "" && webB != "" {
		events, err := c.fetchLatestEvents(ctx, webB, 1)
		if err == nil && len(events) > 0 {
			ev := events[0]
			if u, ok := ev.Data["url"].(string); ok && u != "" {
				snapshot.URL = u
				snapshot.Domain = extractDomain(u)
			}
			if snapshot.Title == "" {
				if title, ok := ev.Data["title"].(string); ok {
					snapshot.Title = title
				}
			}
		}
	}

	return snapshot, nil
}

func (c *Client) fetchLatestEvents(ctx context.Context, bucketID string, limit int) ([]Event, error) {
	urlStr := fmt.Sprintf("%s/api/0/buckets/%s/events?limit=%d", c.baseURL, bucketID, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events endpoint returned %d", resp.StatusCode)
	}

	var events []Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	return events, nil
}

func extractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	return strings.TrimPrefix(host, "www.")
}
