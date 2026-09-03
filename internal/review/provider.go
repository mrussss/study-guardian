package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"study-guardian/internal/ai"
	"study-guardian/internal/classifier/providers"
	"study-guardian/internal/config"
)

const ReviewPromptVersion = "review-provider-v1"

// Provider is deliberately independent from classifier providers. Daily
// Review has a different input, output and evidence-safety contract.
type Provider interface {
	Generate(context.Context, ReviewInput) (Document, ProviderMetadata, error)
}

type ProviderMetadata struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
}

type ProviderOptions struct {
	Name             string
	Endpoint         string
	APIKey           string
	Model            string
	JSONMode         string
	SupportsJSONMode bool
	Timeout          time.Duration
	Temperature      *float64
	HTTPClient       *http.Client
}

// ProviderErrorKind is intentionally a bounded, secret-free operational
// classification. Callers can decide whether to retry or use fallback without
// persisting provider response text.
type ProviderErrorKind string

const (
	ProviderErrorTimeout       ProviderErrorKind = "timeout"
	ProviderErrorUnavailable   ProviderErrorKind = "unavailable"
	ProviderErrorNetwork       ProviderErrorKind = "network"
	ProviderErrorHTTP          ProviderErrorKind = "http"
	ProviderErrorInvalidJSON   ProviderErrorKind = "invalid_json"
	ProviderErrorSchemaInvalid ProviderErrorKind = "schema_invalid"
	ProviderErrorUnsupported   ProviderErrorKind = "unsupported_version"
	ProviderErrorNotConfigured ProviderErrorKind = "not_configured"
)

type ProviderError struct {
	Kind  ProviderErrorKind
	Cause error
}

func (e ProviderError) Error() string {
	return fmt.Sprintf("daily review provider error: %s", e.Kind)
}

func (e ProviderError) Unwrap() error { return e.Cause }

type HTTPReviewProvider struct {
	name     string
	model    string
	apiKey   string
	endpoint string
	client   *ai.Client
}

func NewProvider(o ProviderOptions) (*HTTPReviewProvider, error) {
	if strings.TrimSpace(o.Model) == "" {
		return nil, ProviderError{Kind: ProviderErrorNotConfigured, Cause: errors.New("review provider model is required")}
	}
	if strings.TrimSpace(o.Name) == "" {
		o.Name = "openai-compatible"
	}
	if strings.TrimSpace(o.Endpoint) == "" {
		o.Endpoint = "https://api.openai.com/v1"
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	return &HTTPReviewProvider{
		name: strings.TrimSpace(o.Name), model: strings.TrimSpace(o.Model),
		apiKey: o.APIKey, endpoint: strings.TrimRight(o.Endpoint, "/"),
		client: ai.NewClient(ai.Options{Endpoint: o.Endpoint, APIKey: o.APIKey, Model: o.Model, JSONMode: o.JSONMode, SupportsJSONMode: o.SupportsJSONMode, Timeout: o.Timeout, Temperature: o.Temperature, HTTPClient: o.HTTPClient}),
	}, nil
}

func (p *HTTPReviewProvider) Generate(ctx context.Context, input ReviewInput) (Document, ProviderMetadata, error) {
	metadata := ProviderMetadata{Provider: p.name, Model: p.model, PromptVersion: ReviewPromptVersion}
	if strings.TrimSpace(p.model) == "" || (p.apiKey == "" && !isLocalReviewEndpoint(p.endpoint)) {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorNotConfigured, Cause: errors.New("review provider is not configured")}
	}
	var raw json.RawMessage
	if err := p.client.CompleteJSON(ctx, reviewSystemPrompt, input, &raw); err != nil {
		return Document{}, metadata, classifyProviderError(err)
	}
	var document Document
	if err := json.Unmarshal(raw, &document); err != nil {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorSchemaInvalid, Cause: err}
	}
	if document.SchemaVersion == 0 {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorSchemaInvalid, Cause: errors.New("review document schema_version is required")}
	}
	if document.SchemaVersion != 1 {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorUnsupported, Cause: fmt.Errorf("review document schema version %d", document.SchemaVersion)}
	}
	return document, metadata, nil
}

const reviewSystemPrompt = `You are the StudyGuardian Daily Review assistant.
Return ONLY one JSON object matching the existing canonical Daily Review schema:
{"schema_version":1,"date":"YYYY-MM-DD","headline":"...","topics":[{"name":"...","summary":"...","evidence_refs":["..."],"confidence":0.0}],"accomplishments":[{"text":"...","evidence_refs":["..."],"confidence":0.0}],"unfinished":["..."],"difficulties":["..."],"behavior":{"distraction_count":0,"largest_distraction_seconds":0,"average_recovery_seconds":0},"tomorrow_priority":"..."}
Use only the supplied ReviewInput evidence. Discussion is not mastery. An AI suggestion is not a user accomplishment. An assistant saying “done” is not proof that the user completed the work. A partial assistant turn is not a final conclusion. Every topic and accomplishment must cite existing evidence_refs from the input. Accomplishments require strong completion evidence; chat and semantic snapshots alone cannot prove completion, so prefer an empty accomplishments array and explain uncertainty in unfinished or difficulties. Never invent, alter or replace evidence references. When evidence conflicts or is weak, state uncertainty instead of guessing.`

func classifyProviderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProviderError{Kind: ProviderErrorTimeout, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ProviderError{Kind: ProviderErrorTimeout, Cause: err}
	}
	var httpErr ai.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.Status == 401 || httpErr.Status == 403:
			return ProviderError{Kind: ProviderErrorHTTP, Cause: err}
		case httpErr.Status == 404 || httpErr.Status == 408 || httpErr.Status >= 500:
			return ProviderError{Kind: ProviderErrorUnavailable, Cause: err}
		default:
			return ProviderError{Kind: ProviderErrorHTTP, Cause: err}
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "decode ai json response") {
		return ProviderError{Kind: ProviderErrorInvalidJSON, Cause: err}
	}
	if errors.Is(err, context.Canceled) {
		return ProviderError{Kind: ProviderErrorNetwork, Cause: err}
	}
	return ProviderError{Kind: ProviderErrorNetwork, Cause: err}
}

type ProviderStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Warning    string `json:"warning,omitempty"`
}

// NewConfiguredProvider resolves the review provider config without exposing
// key material. Inheritance deliberately copies the configured text profile,
// including its model and endpoint settings.
func NewConfiguredProvider(cfg *config.Config) (Provider, ProviderStatus) {
	status := ProviderStatus{}
	if cfg == nil {
		status.Warning = "review config is unavailable"
		return nil, status
	}
	reviewConfig := cfg.Review.Provider
	providerName := strings.TrimSpace(reviewConfig.Provider)
	model := strings.TrimSpace(reviewConfig.Model)
	endpoint := strings.TrimSpace(reviewConfig.BaseURL)
	apiKey := resolveReviewKey(reviewConfig.APIKeyEnv, reviewConfig.APIKeyFile)
	jsonMode := reviewConfig.JSONMode
	timeoutSeconds := reviewConfig.TimeoutSeconds
	temperature := reviewConfig.Temperature
	if reviewConfig.InheritTextProfile {
		text := cfg.AI.Text
		providerName = strings.TrimSpace(text.Provider)
		model = strings.TrimSpace(text.Model)
		endpoint = strings.TrimSpace(text.BaseURL)
		apiKey = resolveEndpointKey(text.APIKeyEnv, text.APIKeyFile, "")
		jsonMode = text.JSONMode
		timeoutSeconds = text.TimeoutSeconds
		temperature = text.Temperature
		if apiKey == "" {
			apiKey = strings.TrimSpace(cfg.AI.APIKey)
		}
	}
	status.Provider, status.Model = providerName, model
	if providerName == "" || providerName == "none" {
		status.Warning = "review provider is not configured"
		return nil, status
	}
	profile, ok := providers.ProfileFor(providerName)
	if !ok || providerName == "fake" {
		status.Warning = "review provider is unsupported"
		return nil, status
	}
	if model == "" {
		status.Warning = "review provider model is required"
		return nil, status
	}
	if endpoint == "" {
		endpoint = profile.DefaultBaseURL
	}
	if endpoint == "" || (apiKey == "" && !isLocalReviewEndpoint(endpoint)) {
		status.Warning = "review provider API key is not configured"
		return nil, status
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	provider, err := NewProvider(ProviderOptions{Name: providerName, Endpoint: endpoint, APIKey: apiKey, Model: model, JSONMode: jsonMode, SupportsJSONMode: profile.SupportsJSONMode, Timeout: time.Duration(timeoutSeconds) * time.Second, Temperature: temperature})
	if err != nil {
		status.Warning = "review provider configuration is invalid"
		return nil, status
	}
	status.Configured = true
	return provider, status
}

func resolveReviewKey(envName, fileName string) string {
	return resolveEndpointKey(envName, fileName, "")
}

func resolveEndpointKey(envName, fileName, fallbackEnv string) string {
	if envName != "" {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return value
		}
	}
	if fallbackEnv != "" {
		if value := strings.TrimSpace(os.Getenv(fallbackEnv)); value != "" {
			return value
		}
	}
	if fileName != "" {
		if data, err := os.ReadFile(fileName); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func isLocalReviewEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
