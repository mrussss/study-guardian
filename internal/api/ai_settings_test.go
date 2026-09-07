package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"study-guardian/internal/aisettings"
	"study-guardian/internal/config"
)

type proxyTestManager struct {
	result aisettings.ProxyTestResult
}

func (m proxyTestManager) Settings() aisettings.SettingsDTO { return aisettings.SettingsDTO{} }
func (m proxyTestManager) Save(context.Context, aisettings.SettingsDTO) (aisettings.SettingsDTO, error) {
	return aisettings.SettingsDTO{}, nil
}
func (m proxyTestManager) PutSecret(context.Context, string, string) (aisettings.SettingsDTO, error) {
	return aisettings.SettingsDTO{}, nil
}
func (m proxyTestManager) DeleteSecret(context.Context, string) (aisettings.SettingsDTO, error) {
	return aisettings.SettingsDTO{}, nil
}
func (m proxyTestManager) Test(context.Context, string) aisettings.TestResult {
	return aisettings.TestResult{}
}
func (m proxyTestManager) TestProxy(context.Context) aisettings.ProxyTestResult { return m.result }

func TestAIProxyTestEndpointReturnsOnlyBoundedDiagnostics(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "test-token"
	server := NewServer(cfg, nil)
	server.SetAISettings(proxyTestManager{result: aisettings.ProxyTestResult{OK: false, Mode: "manual", LatencyMS: 19, ErrorKind: "proxy_unreachable"}})
	request := httptest.NewRequest(http.MethodPost, "/v1/settings/ai/proxy/test", nil)
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result aisettings.ProxyTestResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ErrorKind != "proxy_unreachable" || result.Mode != "manual" || result.LatencyMS != 19 {
		t.Fatalf("result=%+v", result)
	}
	if len(recorder.Body.Bytes()) > 512 {
		t.Fatal("proxy diagnostic response exceeded its bounded shape")
	}
}
