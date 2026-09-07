package ai

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"study-guardian/internal/config"
)

// BuildTransport creates a request-scoped transport from a cloned standard
// transport. It never mutates http.DefaultTransport, which is shared by other
// integrations in the Supervisor process.
func BuildTransport(proxy config.AIProxyConfig) (http.RoundTripper, error) {
	proxy.Mode = strings.ToLower(strings.TrimSpace(proxy.Mode))
	if proxy.Mode == "" {
		proxy.Mode = config.AIProxyEnvironment
	}
	if err := config.ValidateAIProxyConfig(proxy); err != nil {
		return nil, err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport is unsupported")
	}
	transport := base.Clone()
	switch proxy.Mode {
	case config.AIProxyEnvironment:
		transport.Proxy = http.ProxyFromEnvironment
	case config.AIProxyDirect:
		transport.Proxy = nil
	case config.AIProxyManual:
		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			return nil, errors.New("manual proxy url is invalid")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &proxyAwareTransport{transport: transport}, nil
}

func NewHTTPClient(timeout time.Duration, proxy config.AIProxyConfig) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport, err := BuildTransport(proxy)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

type proxyAwareTransport struct {
	transport *http.Transport
}

func (t *proxyAwareTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var proxyURL *url.URL
	var err error
	if t.transport.Proxy != nil {
		proxyURL, err = t.transport.Proxy(request)
		if err != nil {
			return nil, ProxyError{Cause: err}
		}
	}
	response, err := t.transport.RoundTrip(request)
	if err != nil && proxyURL != nil {
		return nil, ProxyError{Cause: err}
	}
	return response, err
}

// ProxyError marks a failure while an HTTP proxy was selected. Its Error
// method intentionally omits the URL, credentials, and operating-system
// error text; callers can still inspect Cause internally for classification.
type ProxyError struct{ Cause error }

func (e ProxyError) Error() string { return "AI proxy connection failed" }
func (e ProxyError) Unwrap() error { return e.Cause }

type TransportError struct{ Cause error }

func (e TransportError) Error() string { return "AI network request failed" }
func (e TransportError) Unwrap() error { return e.Cause }
