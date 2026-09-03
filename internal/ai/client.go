package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// JSONClient is the business-neutral AI contract shared by classifier and
// Daily Review. It deliberately knows nothing about either domain's schema.
type JSONClient interface {
	CompleteJSON(context.Context, string, any, any) error
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Options struct {
	Endpoint         string
	APIKey           string
	Model            string
	JSONMode         string
	SupportsJSONMode bool
	Timeout          time.Duration
	Temperature      *float64
	HTTPClient       *http.Client
}

type Client struct {
	endpoint         string
	apiKey           string
	model            string
	jsonMode         string
	supportsJSONMode bool
	temperature      *float64
	httpClient       *http.Client
}

func NewClient(o Options) *Client {
	if o.Endpoint == "" {
		o.Endpoint = "https://api.openai.com/v1"
	}
	if o.JSONMode == "" {
		o.JSONMode = "auto"
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: o.Timeout}
	}
	return &Client{
		endpoint:         strings.TrimRight(o.Endpoint, "/"),
		apiKey:           o.APIKey,
		model:            o.Model,
		jsonMode:         o.JSONMode,
		supportsJSONMode: o.SupportsJSONMode,
		temperature:      o.Temperature,
		httpClient:       o.HTTPClient,
	}
}

func (c *Client) CompleteJSON(ctx context.Context, systemPrompt string, userPayload any, out any) error {
	userData, err := json.Marshal(userPayload)
	if err != nil {
		return fmt.Errorf("marshal AI user payload: %w", err)
	}
	content, err := c.complete(ctx, []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(userData)},
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, out); err != nil {
		return fmt.Errorf("decode AI JSON response: %w", err)
	}
	return nil
}

// CompleteJSONMessages is useful for an existing provider that needs a
// multimodal message. The transport still owns HTTP, auth, timeout and JSON
// mode fallback; the classifier remains responsible for its own schema.
func (c *Client) CompleteJSONMessages(ctx context.Context, messages []Message) ([]byte, error) {
	return c.complete(ctx, messages)
}

type request struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	ResponseFormat *format   `json:"response_format,omitempty"`
	Temperature    *float64  `json:"temperature,omitempty"`
}

type format struct {
	Type string `json:"type"`
}

type response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) complete(ctx context.Context, messages []Message) ([]byte, error) {
	if strings.TrimSpace(c.model) == "" {
		return nil, errors.New("AI model is required")
	}
	withFormat := c.jsonMode == "json_object" || (c.jsonMode == "auto" && c.supportsJSONMode)
	raw, err := c.doRequest(ctx, messages, withFormat)
	if err != nil && withFormat && isUnsupportedJSONMode(err) {
		raw, err = c.doRequest(ctx, messages, false)
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

func (c *Client) doRequest(ctx context.Context, messages []Message, withFormat bool) (string, error) {
	payload := request{Model: c.model, Messages: messages, Temperature: c.temperature}
	if withFormat {
		payload.ResponseFormat = &format{Type: "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI provider HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	var decoded response
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", HTTPError{Status: resp.StatusCode, Message: "invalid provider response"}
		}
		return "", fmt.Errorf("failed to decode AI response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		message := "provider request failed"
		if decoded.Error != nil && decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
		return "", HTTPError{Status: resp.StatusCode, Message: message, RetryAfter: resp.Header.Get("Retry-After")}
	}
	if decoded.Error != nil {
		return "", errors.New("AI provider API error")
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return "", errors.New("empty response content from AI provider")
	}
	return decoded.Choices[0].Message.Content, nil
}

type HTTPError struct {
	Status     int
	Message    string
	RetryAfter string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("AI provider returned HTTP status %d: %s", e.Status, e.Message)
}

func isUnsupportedJSONMode(err error) bool {
	var e HTTPError
	if !errors.As(err, &e) {
		return false
	}
	message := strings.ToLower(e.Message)
	return (e.Status == 400 || e.Status == 422) &&
		(strings.Contains(message, "response_format") || strings.Contains(message, "unsupported") ||
			strings.Contains(message, "json_object") || strings.Contains(message, "json mode"))
}
