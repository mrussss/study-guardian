package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"study-guardian/internal/config"
)

func TestManualProxyRoutesHTTPRequestsWithoutChangingDefaultTransport(t *testing.T) {
	called := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.String() != "http://example.invalid/models" {
			t.Fatalf("proxy request target=%q", r.URL.String())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	defaultProxy := reflect.ValueOf(http.DefaultTransport.(*http.Transport).Proxy).Pointer()
	client, err := NewHTTPClient(time.Second, config.AIProxyConfig{Mode: config.AIProxyManual, URL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/models", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !called {
		t.Fatal("manual proxy was not used")
	}
	if reflect.ValueOf(http.DefaultTransport.(*http.Transport).Proxy).Pointer() != defaultProxy {
		t.Fatal("default transport was mutated")
	}
}

func TestDirectProxyModeIgnoresEnvironmentProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("direct request used proxy")
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	client, err := NewHTTPClient(time.Second, config.AIProxyConfig{Mode: config.AIProxyDirect})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL, nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestEnvironmentProxyModeUsesGoEnvironmentProxy(t *testing.T) {
	transport, err := BuildTransport(config.AIProxyConfig{Mode: config.AIProxyEnvironment})
	if err != nil {
		t.Fatal(err)
	}
	aware, ok := transport.(*proxyAwareTransport)
	if !ok || aware.transport.Proxy == nil {
		t.Fatal("environment mode must install Go's environment proxy resolver")
	}
}

func TestAIProxyValidationRejectsCredentialsAndURLComponents(t *testing.T) {
	valid := config.AIProxyConfig{Mode: config.AIProxyManual, URL: "http://127.0.0.1:7890"}
	for _, testCase := range []struct {
		name string
		url  string
	}{
		{"credentials", "http://user:secret@127.0.0.1:7890"},
		{"path", "http://127.0.0.1:7890/path"},
		{"query", "http://127.0.0.1:7890/?token=secret"},
		{"fragment", "http://127.0.0.1:7890/#secret"},
		{"wrong scheme", "socks5://127.0.0.1:7890"},
		{"missing host", "http://"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := config.ValidateAIProxyConfig(config.AIProxyConfig{Mode: config.AIProxyManual, URL: testCase.url}); err == nil {
				t.Fatal("expected invalid proxy URL")
			}
		})
	}
	if err := config.ValidateAIProxyConfig(valid); err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateAIProxyConfig(config.AIProxyConfig{Mode: "direct", URL: valid.URL}); err == nil {
		t.Fatal("non-manual proxy URL should be rejected")
	}
	if err := config.ValidateAIProxyConfig(config.AIProxyConfig{Mode: config.AIProxyManual, URL: strings.Repeat("x", 2049)}); err == nil {
		t.Fatal("overlong proxy URL should be rejected")
	}
}

func TestFailureClassificationDoesNotFallbackForBareAccountRateLimit(t *testing.T) {
	bare := ClassifyError(HTTPError{Status: http.StatusTooManyRequests})
	if bare.Kind != FailureAccountRateLimited || ShouldFallbackModel(context.Background(), HTTPError{Status: http.StatusTooManyRequests}) {
		t.Fatalf("bare 429=%+v should stop model fallback", bare)
	}
	explicit := ClassifyError(HTTPError{Status: http.StatusTooManyRequests, Code: "model_rate_limited"})
	if explicit.Kind != FailureModelRateLimited || !ShouldFallbackModel(context.Background(), HTTPError{Status: http.StatusTooManyRequests, Code: "model_rate_limited"}) {
		t.Fatalf("explicit model rate limit=%+v should advance fallback", explicit)
	}
	if got := ClassifyError(ProxyError{Cause: errors.New("connection refused")}); got.Kind != FailureProxyUnreachable {
		t.Fatalf("proxy failure=%+v", got)
	}
}
