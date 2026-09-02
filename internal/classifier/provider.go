package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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
	Classify(ctx context.Context, req ClassificationRequest) (*ClassificationResponse, error)
	Name() string
}

// FakeProvider for offline testing and fallback
type FakeProvider struct {
	DefaultRelation   state.TaskRelation
	DefaultConfidence float64
	DefaultReason     string
	ShouldError       bool
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		DefaultRelation:   state.RelationFocused,
		DefaultConfidence: 0.88,
		DefaultReason:     "Fake provider classified as focused on task",
	}
}

func (f *FakeProvider) Name() string {
	return "fake"
}

func (f *FakeProvider) Classify(ctx context.Context, req ClassificationRequest) (*ClassificationResponse, error) {
	if f.ShouldError {
		return nil, errors.New("fake provider simulated error")
	}

	titleLower := strings.ToLower(req.Title)
	if strings.Contains(titleLower, "game") || strings.Contains(titleLower, "play") || strings.Contains(titleLower, "anime") {
		return &ClassificationResponse{
			Relation:    state.RelationDistracted,
			Confidence:  0.92,
			Activity:    "entertainment",
			TaskRelated: false,
			ReasonShort: "Entertainment content detected",
		}, nil
	}

	return &ClassificationResponse{
		Relation:    f.DefaultRelation,
		Confidence:  f.DefaultConfidence,
		Activity:    "studying/work",
		TaskRelated: f.DefaultRelation == state.RelationFocused,
		ReasonShort: f.DefaultReason,
	}, nil
}

// OpenAICompatibleProvider handles OpenAI / Ollama / DeepSeek / Gemini compatible endpoints
type OpenAICompatibleProvider struct {
	Endpoint   string
	APIKey     string
	Model      string
	httpClient *http.Client
}

func NewOpenAICompatibleProvider(endpoint, apiKey, model string) *OpenAICompatibleProvider {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAICompatibleProvider{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Model:    model,
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
		},
	}
}

func (p *OpenAICompatibleProvider) Name() string {
	return "openai-compatible (" + p.Model + ")"
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
	if p.APIKey == "" && !strings.Contains(p.Endpoint, "127.0.0.1") && !strings.Contains(p.Endpoint, "localhost") {
		return nil, errors.New("API key not configured for AI provider")
	}

	systemPrompt := `You are an AI study guardian assistant. Classify if the user's current computer activity is related to their declared study task.
Respond ONLY with a valid JSON object with the following schema:
{
  "relation": "FOCUSED" | "DISTRACTED" | "UNKNOWN",
  "confidence": number between 0.0 and 1.0,
  "activity": "short description of activity",
  "task_related": boolean,
  "reason_short": "brief justification"
}`

	userPrompt := fmt.Sprintf("Declared Task: %s\nActive Window: %s\nWindow Title: %s\nDomain: %s",
		req.Task, req.App, req.Title, req.Domain)

	var messages []chatMessage
	messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})

	if req.AnalysisImageBase64 != "" {
		// Multimodal vision message
		contentParts := []map[string]interface{}{
			{"type": "text", "text": userPrompt},
			{
				"type": "image_url",
				"image_url": map[string]string{
					"url":    "data:image/jpeg;base64," + req.AnalysisImageBase64,
					"detail": "low",
				},
			},
		}
		messages = append(messages, chatMessage{Role: "user", Content: contentParts})
	} else {
		messages = append(messages, chatMessage{Role: "user", Content: userPrompt})
	}

	payload := chatCompletionRequest{
		Model:          p.Model,
		Messages:       messages,
		ResponseFormat: &respFormat{Type: "json_object"},
		Temperature:    0.1,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/chat/completions", p.Endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("AI provider HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI provider returned HTTP status %d", resp.StatusCode)
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode AI response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("AI provider API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		return nil, errors.New("empty response content from AI provider")
	}

	var classResp ClassificationResponse
	contentStr := chatResp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(contentStr), &classResp); err != nil {
		return nil, fmt.Errorf("failed to parse structured JSON classification from AI: %w", err)
	}

	// Validate enum values
	switch classResp.Relation {
	case state.RelationFocused, state.RelationDistracted, state.RelationUnknown:
		// Valid
	default:
		classResp.Relation = state.RelationUnknown
	}

	return &classResp, nil
}
