package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/state"
)

type ClassificationRequest struct {
	Task                string `json:"task"`
	App                 string `json:"app"`
	Title               string `json:"title"`
	Domain              string `json:"domain"`
	AnalysisImageBase64 string `json:"analysis_image_base64,omitempty"`
	UserMode            string `json:"user_mode"`
}
type ClassificationResponse struct {
	Relation    state.TaskRelation `json:"relation"`
	Confidence  float64            `json:"confidence"`
	Activity    string             `json:"activity"`
	TaskRelated bool               `json:"task_related"`
	ReasonShort string             `json:"reason_short"`
}
type TaskRelationProvider interface {
	Classify(context.Context, ClassificationRequest) (*ClassificationResponse, error)
	Name() string
}

const (
	maxActivityLength = 200
	maxReasonLength   = 240
)

func ValidateClassificationResponse(resp *ClassificationResponse) error {
	if resp == nil {
		return errors.New("AI provider returned nil classification")
	}
	switch resp.Relation {
	case state.RelationFocused, state.RelationDistracted, state.RelationUnknown:
	default:
		return fmt.Errorf("invalid classification relation %q", resp.Relation)
	}
	if math.IsNaN(resp.Confidence) || math.IsInf(resp.Confidence, 0) || resp.Confidence < 0 || resp.Confidence > 1 {
		return fmt.Errorf("classification confidence must be between 0 and 1, got %v", resp.Confidence)
	}
	if strings.TrimSpace(resp.Activity) == "" || len([]rune(resp.Activity)) > maxActivityLength {
		return fmt.Errorf("classification activity must be non-empty and at most %d characters", maxActivityLength)
	}
	if strings.TrimSpace(resp.ReasonShort) == "" || len([]rune(resp.ReasonShort)) > maxReasonLength {
		return fmt.Errorf("classification reason_short must be non-empty and at most %d characters", maxReasonLength)
	}
	return nil
}

// Kept as a package-local alias for existing classifier service/tests.
func validateClassificationResponse(resp *ClassificationResponse) error {
	return ValidateClassificationResponse(resp)
}

type FakeProvider struct {
	DefaultRelation   state.TaskRelation
	DefaultConfidence float64
	DefaultReason     string
	ShouldError       bool
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{DefaultRelation: state.RelationFocused, DefaultConfidence: .88, DefaultReason: "Fake provider classified as focused on task"}
}
func (f *FakeProvider) Name() string { return "fake" }
func (f *FakeProvider) Classify(ctx context.Context, req ClassificationRequest) (*ClassificationResponse, error) {
	if f.ShouldError {
		return nil, errors.New("fake provider simulated error")
	}
	if strings.Contains(strings.ToLower(req.Title), "game") || strings.Contains(strings.ToLower(req.Title), "play") || strings.Contains(strings.ToLower(req.Title), "anime") {
		return &ClassificationResponse{Relation: state.RelationDistracted, Confidence: .92, Activity: "entertainment", TaskRelated: false, ReasonShort: "Entertainment content detected"}, nil
	}
	return &ClassificationResponse{Relation: f.DefaultRelation, Confidence: f.DefaultConfidence, Activity: "studying/work", TaskRelated: f.DefaultRelation == state.RelationFocused, ReasonShort: f.DefaultReason}, nil
}

type ProviderOptions struct {
	Endpoint         string
	APIKey           string
	Model            string
	JSONMode         string
	SupportsJSONMode bool
	Timeout          time.Duration
}
type OpenAICompatibleProvider struct {
	Endpoint            string
	APIKey              string
	Model               string
	JSONMode            string
	SupportsJSONMode    bool
	httpClient          *http.Client
	mu                  sync.Mutex
	cooldownUntil       time.Time
	consecutiveFailures int
}

func NewOpenAICompatibleProvider(endpoint, apiKey, model string) *OpenAICompatibleProvider {
	return NewOpenAICompatibleProviderWithOptions(ProviderOptions{Endpoint: endpoint, APIKey: apiKey, Model: model, JSONMode: "json_object", SupportsJSONMode: true, Timeout: 4 * time.Second})
}
func NewOpenAICompatibleProviderWithOptions(o ProviderOptions) *OpenAICompatibleProvider {
	if o.Endpoint == "" {
		o.Endpoint = "https://api.openai.com/v1"
	}
	if o.Model == "" {
		o.Model = "gpt-4o-mini"
	}
	if o.JSONMode == "" {
		o.JSONMode = "auto"
	}
	if o.Timeout <= 0 {
		o.Timeout = 6 * time.Second
	}
	return &OpenAICompatibleProvider{Endpoint: strings.TrimRight(o.Endpoint, "/"), APIKey: o.APIKey, Model: o.Model, JSONMode: o.JSONMode, SupportsJSONMode: o.SupportsJSONMode, httpClient: &http.Client{Timeout: o.Timeout}}
}
func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible (" + p.Model + ")" }
func (p *OpenAICompatibleProvider) CooldownUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cooldownUntil
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}
type chatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
	Temperature    float64       `json:"temperature"`
}
type respFormat struct {
	Type string `json:"type"`
}
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAICompatibleProvider) Classify(ctx context.Context, req ClassificationRequest) (*ClassificationResponse, error) {
	if p.APIKey == "" && !isLocalEndpoint(p.Endpoint) {
		return nil, errors.New("API key not configured for AI provider")
	}
	p.mu.Lock()
	if time.Now().Before(p.cooldownUntil) {
		u := p.cooldownUntil
		p.mu.Unlock()
		return nil, fmt.Errorf("provider cooldown until %s", u.UTC().Format(time.RFC3339))
	}
	p.mu.Unlock()
	system := `You are an AI study guardian assistant. Classify if the user's current computer activity is related to their declared study task. Respond ONLY with a valid JSON object with the following schema: {"relation":"FOCUSED|DISTRACTED|UNKNOWN","confidence":number,"activity":"short description","task_related":boolean,"reason_short":"brief justification"}`
	user := fmt.Sprintf("Declared Task: %s\nActive Window: %s\nWindow Title: %s\nDomain: %s", req.Task, req.App, req.Title, req.Domain)
	messages := []chatMessage{{Role: "system", Content: system}}
	if req.AnalysisImageBase64 != "" {
		messages = append(messages, chatMessage{Role: "user", Content: []map[string]interface{}{{"type": "text", "text": user}, {"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + req.AnalysisImageBase64, "detail": "low"}}}})
	} else {
		messages = append(messages, chatMessage{Role: "user", Content: user})
	}
	withFormat := p.JSONMode == "json_object" || (p.JSONMode == "auto" && p.SupportsJSONMode)
	raw, err := p.doRequest(ctx, messages, withFormat)
	if err != nil && withFormat && isUnsupportedJSONMode(err) {
		raw, err = p.doRequest(ctx, messages, false)
	}
	if err != nil {
		p.recordFailure(err)
		return nil, err
	}
	p.recordSuccess()
	return parseClassification(raw)
}
func (p *OpenAICompatibleProvider) doRequest(ctx context.Context, messages []chatMessage, withFormat bool) ([]byte, error) {
	payload := chatCompletionRequest{Model: p.Model, Messages: messages, Temperature: .1}
	if withFormat {
		payload.ResponseFormat = &respFormat{Type: "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.httpClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("AI provider HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	var cr chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, providerHTTPError{status: resp.StatusCode, message: "invalid provider response"}
		}
		return nil, fmt.Errorf("failed to decode AI response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := "provider request failed"
		if cr.Error != nil && cr.Error.Message != "" {
			msg = cr.Error.Message
		}
		return nil, providerHTTPError{status: resp.StatusCode, message: msg, retryAfter: resp.Header.Get("Retry-After")}
	}
	if cr.Error != nil {
		return nil, errors.New("AI provider API error")
	}
	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		return nil, errors.New("empty response content from AI provider")
	}
	return []byte(cr.Choices[0].Message.Content), nil
}

type providerHTTPError struct {
	status     int
	message    string
	retryAfter string
}

func (e providerHTTPError) Error() string {
	return fmt.Sprintf("AI provider returned HTTP status %d: %s", e.status, e.message)
}
func isUnsupportedJSONMode(err error) bool {
	var e providerHTTPError
	if !errors.As(err, &e) {
		return false
	}
	message := strings.ToLower(e.message)
	return (e.status == 400 || e.status == 422) && (strings.Contains(message, "response_format") || strings.Contains(message, "unsupported") || strings.Contains(message, "json_object") || strings.Contains(message, "json mode"))
}

func isLocalEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
func (p *OpenAICompatibleProvider) recordFailure(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures++
	delay := 2 * time.Second
	if p.consecutiveFailures > 1 {
		delay = time.Duration(1<<minInt(p.consecutiveFailures-1, 4)) * time.Second
	}
	var e providerHTTPError
	if errors.As(err, &e) {
		if e.status == 401 || e.status == 403 {
			delay = 5 * time.Minute
		}
		if e.status == 429 {
			delay = 30 * time.Second
			if n, _ := strconv.Atoi(e.retryAfter); n > 0 {
				delay = time.Duration(n) * time.Second
			}
		}
	}
	p.cooldownUntil = time.Now().Add(delay)
}
func (p *OpenAICompatibleProvider) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures = 0
	p.cooldownUntil = time.Time{}
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func parseClassification(content []byte) (*ClassificationResponse, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse structured JSON classification from AI: %w", err)
	}
	for _, f := range []string{"relation", "confidence", "activity", "task_related", "reason_short"} {
		if _, ok := fields[f]; !ok {
			return nil, fmt.Errorf("AI classification is missing required field %q", f)
		}
	}
	normalized, _ := json.Marshal(fields)
	var result ClassificationResponse
	if err := json.Unmarshal(normalized, &result); err != nil {
		return nil, fmt.Errorf("invalid AI classification field type: %w", err)
	}
	if err := ValidateClassificationResponse(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
