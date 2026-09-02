package classifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleProviderRejectsInvalidSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"message": map[string]string{
					"content": `{"relation":"FOCUSED","confidence":7.8,"activity":"work","task_related":true,"reason_short":"bad confidence"}`,
				},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(server.URL, "", "test-model")
	_, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"})
	if err == nil {
		t.Fatal("expected invalid confidence to be rejected")
	}
}

func TestOpenAICompatibleProviderRejectsMissingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"message": map[string]string{
					"content": `{"relation":"UNKNOWN","confidence":0.5,"activity":"work","task_related":false}`,
				},
			}},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider(server.URL, "", "test-model")
	_, err := provider.Classify(context.Background(), ClassificationRequest{Task: "Go"})
	if err == nil {
		t.Fatal("expected missing reason_short to be rejected")
	}
}
