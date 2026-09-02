package sensor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CaptureRequest struct {
	Monitor              int  `json:"monitor"`
	IncludeAnalysisImage bool `json:"include_analysis_image"`
	MaxWidth             int  `json:"max_width"`
}

type CaptureResponse struct {
	Timestamp     string  `json:"timestamp"`
	Monitor       int     `json:"monitor"`
	Changed       bool    `json:"changed"`
	Hash          string  `json:"hash"`
	IsStub        bool    `json:"is_stub"`
	AnalysisImage *string `json:"analysis_image,omitempty"`
	Error         *string `json:"error,omitempty"`
}

type HealthResponse struct {
	Status       string `json:"status"`
	Service      string `json:"service"`
	MSSAvailable bool   `json:"mss_available"`
	Timestamp    string `json:"timestamp"`
}

type Client interface {
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error)
	Health(ctx context.Context) (*HealthResponse, error)
}

type HTTPClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

func NewHTTPClient(host string, port int, authToken string) *HTTPClient {
	return &HTTPClient{
		baseURL:   fmt.Sprintf("http://%s:%d", host, port),
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (c *HTTPClient) Health(ctx context.Context) (*HealthResponse, error) {
	url := fmt.Sprintf("%s/healthz", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensor health request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sensor health returned status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode sensor health response: %w", err)
	}
	return &health, nil
}

func (c *HTTPClient) Capture(ctx context.Context, captureReq CaptureRequest) (*CaptureResponse, error) {
	url := fmt.Sprintf("%s/v1/capture", c.baseURL)
	body, err := json.Marshal(captureReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensor capture request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sensor capture returned status %d", resp.StatusCode)
	}

	var captureResp CaptureResponse
	if err := json.NewDecoder(resp.Body).Decode(&captureResp); err != nil {
		return nil, fmt.Errorf("failed to decode sensor capture response: %w", err)
	}
	return &captureResp, nil
}
