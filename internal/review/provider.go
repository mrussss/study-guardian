package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type ModelFallbackProvider struct {
	providers []Provider
}

func NewModelFallbackProvider(providers ...Provider) Provider {
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			items = append(items, provider)
		}
	}
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return items[0]
	}
	return &ModelFallbackProvider{providers: items}
}

func (p *ModelFallbackProvider) Generate(ctx context.Context, input ReviewInput) (Document, ProviderMetadata, error) {
	var lastMetadata ProviderMetadata
	var lastErr error
	for index, provider := range p.providers {
		if index > 0 && reviewProviderWouldExceedDeadline(ctx, provider) {
			break
		}
		document, metadata, err := provider.Generate(ctx, input)
		lastMetadata = metadata
		if err == nil {
			return document, metadata, nil
		}
		lastErr = err
		if index == len(p.providers)-1 || !shouldFallbackReviewModel(ctx, err) {
			break
		}
	}
	return Document{}, lastMetadata, lastErr
}

func reviewProviderWouldExceedDeadline(ctx context.Context, provider Provider) bool {
	if ctx.Err() != nil {
		return true
	}
	timed, ok := provider.(interface{ Timeout() time.Duration })
	if !ok || timed.Timeout() <= 0 {
		return false
	}
	deadline, ok := ctx.Deadline()
	return ok && time.Until(deadline) <= timed.Timeout()
}

func shouldFallbackReviewModel(ctx context.Context, err error) bool {
	return ai.ShouldFallbackModel(ctx, err)
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
	Proxy            config.AIProxyConfig
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
	Kind         ProviderErrorKind
	FailureKind  ai.FailureKind
	FailureScope ai.FailureScope
	Cause        error
}

func (e ProviderError) Error() string {
	return fmt.Sprintf("daily review provider error: %s", e.Kind)
}

func (e ProviderError) Unwrap() error { return e.Cause }
func (e ProviderError) AIClassification() (ai.FailureKind, ai.FailureScope) {
	if e.FailureKind != "" && e.FailureScope != "" {
		return e.FailureKind, e.FailureScope
	}
	return ai.FailureProviderUnavailable, ai.FailureScopeProvider
}

type HTTPReviewProvider struct {
	name     string
	model    string
	apiKey   string
	endpoint string
	timeout  time.Duration
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
		timeout: o.Timeout,
		client:  ai.NewClient(ai.Options{Endpoint: o.Endpoint, APIKey: o.APIKey, Model: o.Model, JSONMode: o.JSONMode, SupportsJSONMode: o.SupportsJSONMode, Timeout: o.Timeout, Temperature: o.Temperature, HTTPClient: o.HTTPClient, Proxy: o.Proxy}),
	}, nil
}

func (p *HTTPReviewProvider) Timeout() time.Duration { return p.timeout }

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
		return Document{}, metadata, ProviderError{Kind: ProviderErrorSchemaInvalid, FailureKind: ai.FailureInvalidOutput, FailureScope: ai.FailureScopeModel, Cause: err}
	}
	if document.SchemaVersion == 0 {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorSchemaInvalid, FailureKind: ai.FailureInvalidOutput, FailureScope: ai.FailureScopeModel, Cause: errors.New("review document schema_version is required")}
	}
	if document.SchemaVersion != 1 {
		return Document{}, metadata, ProviderError{Kind: ProviderErrorUnsupported, FailureKind: ai.FailureInvalidOutput, FailureScope: ai.FailureScopeModel, Cause: fmt.Errorf("review document schema version %d", document.SchemaVersion)}
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
	failure := ai.ClassifyError(err)
	switch failure.Kind {
	case ai.FailureInvalidOutput:
		return ProviderError{Kind: ProviderErrorInvalidJSON, FailureKind: failure.Kind, FailureScope: failure.Scope, Cause: err}
	case ai.FailureTimeout, ai.FailureProviderTimeout, ai.FailureModelTimeout:
		return ProviderError{Kind: ProviderErrorTimeout, FailureKind: failure.Kind, FailureScope: failure.Scope, Cause: err}
	case ai.FailureProviderUnavailable:
		return ProviderError{Kind: ProviderErrorUnavailable, FailureKind: failure.Kind, FailureScope: failure.Scope, Cause: err}
	case ai.FailureAuthentication, ai.FailureModelNotFound, ai.FailureModelUnavailable, ai.FailureModelRateLimited, ai.FailureAccountRateLimited:
		return ProviderError{Kind: ProviderErrorHTTP, FailureKind: failure.Kind, FailureScope: failure.Scope, Cause: err}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "decode ai json response") {
		return ProviderError{Kind: ProviderErrorInvalidJSON, FailureKind: ai.FailureInvalidOutput, FailureScope: ai.FailureScopeModel, Cause: err}
	}
	if errors.Is(err, context.Canceled) {
		return ProviderError{Kind: ProviderErrorNetwork, FailureKind: ai.FailureNetworkUnreachable, FailureScope: ai.FailureScopeTransport, Cause: err}
	}
	return ProviderError{Kind: ProviderErrorNetwork, FailureKind: failure.Kind, FailureScope: failure.Scope, Cause: err}
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
	if err := config.ValidateAIProxyConfig(cfg.AI.Proxy); err != nil {
		status.Warning = "AI proxy configuration is invalid"
		return nil, status
	}
	if !cfg.Review.Enabled {
		status.Warning = "daily review is disabled"
		return nil, status
	}
	reviewConfig := cfg.Review.Provider
	providerName := strings.TrimSpace(reviewConfig.Provider)
	model := strings.TrimSpace(reviewConfig.Model)
	fallbackModels := append([]string(nil), reviewConfig.FallbackModels...)
	endpoint := strings.TrimSpace(reviewConfig.BaseURL)
	apiKey := resolveReviewKey(reviewConfig.APIKeyEnv, reviewConfig.APIKeyFile)
	jsonMode := reviewConfig.JSONMode
	timeoutSeconds := reviewConfig.TimeoutSeconds
	temperature := reviewConfig.Temperature
	if reviewConfig.InheritTextProfile {
		text := cfg.AI.Text
		providerName = strings.TrimSpace(text.Provider)
		model = strings.TrimSpace(text.Model)
		fallbackModels = append([]string(nil), text.FallbackModels...)
		endpoint = strings.TrimSpace(text.BaseURL)
		apiKey = resolveEndpointKey(text.APIKeyEnv, text.APIKeyFile, "")
		jsonMode = text.JSONMode
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
	models := append([]string{model}, fallbackModels...)
	chain := make([]Provider, 0, len(models))
	for _, candidate := range models {
		provider, err := NewProvider(ProviderOptions{Name: providerName, Endpoint: endpoint, APIKey: apiKey, Model: strings.TrimSpace(candidate), JSONMode: jsonMode, SupportsJSONMode: profile.SupportsJSONMode, Timeout: time.Duration(timeoutSeconds) * time.Second, Temperature: temperature, Proxy: cfg.AI.Proxy})
		if err != nil {
			status.Warning = "review provider configuration is invalid"
			return nil, status
		}
		chain = append(chain, provider)
	}
	status.Configured = true
	return NewModelFallbackProvider(chain...), status
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
