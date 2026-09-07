package ai

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
)

type FailureKind string

const (
	FailureAuthentication      FailureKind = "authentication"
	FailureModelNotFound       FailureKind = "model_not_found"
	FailureModelUnavailable    FailureKind = "model_unavailable"
	FailureModelRateLimited    FailureKind = "model_rate_limited"
	FailureAccountRateLimited  FailureKind = "account_rate_limited"
	FailureProviderUnavailable FailureKind = "provider_unavailable"
	FailureProxyUnreachable    FailureKind = "proxy_unreachable"
	FailureNetworkUnreachable  FailureKind = "network_unreachable"
	FailureTLSFailed           FailureKind = "tls_failed"
	FailureTimeout             FailureKind = "timeout"
	FailureProviderTimeout     FailureKind = "provider_timeout"
	FailureModelTimeout        FailureKind = "model_timeout"
	FailureInvalidOutput       FailureKind = "invalid_output"
)

type FailureScope string

const (
	FailureScopeModel     FailureScope = "model"
	FailureScopeProvider  FailureScope = "provider"
	FailureScopeTransport FailureScope = "transport"
	FailureScopeAccount   FailureScope = "account"
)

// Failure is the bounded error contract used by every AI path. Cause remains
// available to internal classification, but Error never exposes provider
// response text, keys, proxy URLs, or filesystem paths.
type Failure struct {
	Kind  FailureKind
	Scope FailureScope
	Cause error
}

func NewFailure(kind FailureKind, scope FailureScope, cause error) Failure {
	return Failure{Kind: kind, Scope: scope, Cause: cause}
}

func (e Failure) Error() string                                 { return "AI request failed: " + string(e.Kind) }
func (e Failure) Unwrap() error                                 { return e.Cause }
func (e Failure) AIClassification() (FailureKind, FailureScope) { return e.Kind, e.Scope }

type classificationProvider interface {
	AIClassification() (FailureKind, FailureScope)
}

func ClassifyError(err error) Failure {
	if err == nil {
		return Failure{}
	}
	var classified classificationProvider
	if errors.As(err, &classified) {
		kind, scope := classified.AIClassification()
		if kind != "" && scope != "" {
			return Failure{Kind: kind, Scope: scope, Cause: err}
		}
	}
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return classifyHTTPError(httpErr)
	}
	if errors.Is(err, context.Canceled) {
		return Failure{Kind: FailureNetworkUnreachable, Scope: FailureScopeTransport, Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Failure{Kind: FailureTimeout, Scope: FailureScopeTransport, Cause: err}
	}
	var tlsRecord tls.RecordHeaderError
	if errors.As(err, &tlsRecord) {
		return Failure{Kind: FailureTLSFailed, Scope: FailureScopeTransport, Cause: err}
	}
	var tlsCertificate *tls.CertificateVerificationError
	if errors.As(err, &tlsCertificate) {
		return Failure{Kind: FailureTLSFailed, Scope: FailureScopeTransport, Cause: err}
	}
	var proxyErr ProxyError
	if errors.As(err, &proxyErr) {
		return Failure{Kind: FailureProxyUnreachable, Scope: FailureScopeTransport, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return Failure{Kind: FailureTimeout, Scope: FailureScopeTransport, Cause: err}
		}
		return Failure{Kind: FailureNetworkUnreachable, Scope: FailureScopeTransport, Cause: err}
	}
	return Failure{Kind: FailureNetworkUnreachable, Scope: FailureScopeTransport, Cause: err}
}

func classifyHTTPError(err HTTPError) Failure {
	code := strings.ToLower(strings.TrimSpace(err.Code))
	typ := strings.ToLower(strings.TrimSpace(err.Type))
	switch code {
	case "no_available_channel", "model_not_found", "model_not_available", "model_unavailable":
		if code == "model_not_found" {
			return Failure{Kind: FailureModelNotFound, Scope: FailureScopeModel, Cause: err}
		}
		return Failure{Kind: FailureModelUnavailable, Scope: FailureScopeModel, Cause: err}
	case "model_rate_limited", "model_rate_limit", "model_rate_limit_exceeded":
		return Failure{Kind: FailureModelRateLimited, Scope: FailureScopeModel, Cause: err}
	case "model_timeout":
		return Failure{Kind: FailureModelTimeout, Scope: FailureScopeModel, Cause: err}
	case "provider_timeout":
		return Failure{Kind: FailureProviderTimeout, Scope: FailureScopeProvider, Cause: err}
	case "insufficient_quota", "quota_exceeded", "billing_hard_limit_reached", "account_rate_limited", "account_deactivated":
		return Failure{Kind: FailureAccountRateLimited, Scope: FailureScopeAccount, Cause: err}
	case "authentication_error", "invalid_api_key", "invalid_api_key_error", "unauthorized":
		return Failure{Kind: FailureAuthentication, Scope: FailureScopeAccount, Cause: err}
	}
	if err.Status == 401 || err.Status == 403 || strings.Contains(typ, "authentication") {
		return Failure{Kind: FailureAuthentication, Scope: FailureScopeAccount, Cause: err}
	}
	if err.Status == 404 {
		return Failure{Kind: FailureModelNotFound, Scope: FailureScopeModel, Cause: err}
	}
	if err.Status == 429 {
		// A bare 429 is deliberately account/provider scoped. Switching to a
		// paid model is unsafe unless the provider gave an explicit model code.
		return Failure{Kind: FailureAccountRateLimited, Scope: FailureScopeAccount, Cause: err}
	}
	if err.Status == 408 {
		return Failure{Kind: FailureProviderTimeout, Scope: FailureScopeProvider, Cause: err}
	}
	if err.Status >= 500 {
		return Failure{Kind: FailureProviderUnavailable, Scope: FailureScopeProvider, Cause: err}
	}
	return Failure{Kind: FailureProviderUnavailable, Scope: FailureScopeProvider, Cause: err}
}

// ShouldFallbackModel is intentionally strict: only failures whose scope is
// demonstrably one model may advance within the same provider chain.
func ShouldFallbackModel(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	failure := ClassifyError(err)
	if failure.Scope != FailureScopeModel {
		return false
	}
	switch failure.Kind {
	case FailureModelNotFound, FailureModelUnavailable, FailureModelRateLimited, FailureInvalidOutput:
		return true
	default:
		return false
	}
}
