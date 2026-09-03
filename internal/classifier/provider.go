package classifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/ai"
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
	Temperature      *float64
}
type OpenAICompatibleProvider struct {
	Endpoint            string
	APIKey              string
	Model               string
	JSONMode            string
	SupportsJSONMode    bool
	Temperature         *float64
	mu                  sync.Mutex
	cooldownUntil       time.Time
	consecutiveFailures int
	lastError           string
	lastSuccessAt       time.Time
	client              *ai.Client
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
	return &OpenAICompatibleProvider{
		Endpoint: strings.TrimRight(o.Endpoint, "/"), APIKey: o.APIKey, Model: o.Model,
		JSONMode: o.JSONMode, SupportsJSONMode: o.SupportsJSONMode, Temperature: o.Temperature,
		client: ai.NewClient(ai.Options{Endpoint: o.Endpoint, APIKey: o.APIKey, Model: o.Model, JSONMode: o.JSONMode, SupportsJSONMode: o.SupportsJSONMode, Timeout: o.Timeout, Temperature: o.Temperature}),
	}
}
func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible (" + p.Model + ")" }
func (p *OpenAICompatibleProvider) CooldownUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cooldownUntil
}
func (p *OpenAICompatibleProvider) LastError() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastError
}
func (p *OpenAICompatibleProvider) LastSuccessAt() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSuccessAt
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
	messages := []ai.Message{{Role: "system", Content: system}}
	if req.AnalysisImageBase64 != "" {
		messages = append(messages, ai.Message{Role: "user", Content: []map[string]interface{}{{"type": "text", "text": user}, {"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + req.AnalysisImageBase64, "detail": "low"}}}})
	} else {
		messages = append(messages, ai.Message{Role: "user", Content: user})
	}
	raw, err := p.client.CompleteJSONMessages(ctx, messages)
	if err != nil {
		p.recordFailure(err)
		return nil, err
	}
	p.recordSuccess()
	return parseClassification(raw)
}

type providerHTTPError = ai.HTTPError

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
		if e.Status == 401 || e.Status == 403 {
			delay = 5 * time.Minute
		}
		if e.Status == 429 {
			delay = 30 * time.Second
			if n, _ := strconv.Atoi(e.RetryAfter); n > 0 {
				delay = time.Duration(n) * time.Second
			}
		}
	}
	p.cooldownUntil = time.Now().Add(delay)
	if errors.As(err, &e) {
		p.lastError = fmt.Sprintf("HTTP %d", e.Status)
	} else {
		p.lastError = "provider request failed"
	}
}
func (p *OpenAICompatibleProvider) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures = 0
	p.cooldownUntil = time.Time{}
	p.lastError = ""
	p.lastSuccessAt = time.Now()
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
