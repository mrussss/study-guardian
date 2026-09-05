package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func TestTaskPresetAPISelectsCanonicalTaskAndPersistsSession(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IPC.AuthToken = "token"
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clock := state.NewFakeClock(time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC))
	mgr := state.NewPersistentManager(clock, cfg, store, nil, nil, nil)
	server := NewServer(cfg, mgr)
	server.SetStorage(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/task-presets", server.withAuth(server.handleTaskPresets))
	mux.HandleFunc("/v1/task-presets/", server.withAuth(server.handleTaskPresetAction))

	request := httptest.NewRequest(http.MethodPost, "/v1/task-presets", bytes.NewBufferString(`{"name":" Go ","pinned":true}`))
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create=%d body=%s", response.Code, response.Body.String())
	}
	var created storage.TaskPreset
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/task-presets/"+created.ID+"/select", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("select=%d body=%s", response.Code, response.Body.String())
	}
	if got := mgr.GetStatus().Task; got != "Go" {
		t.Fatalf("canonical task=%q", got)
	}
	open, err := store.LoadOpenSession(context.Background())
	if err != nil || open.Task != "Go" {
		t.Fatalf("open session=%+v err=%v", open, err)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/task-presets", nil)
	request.Header.Set("Authorization", "Bearer token")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"use_count":1`)) {
		t.Fatalf("list=%d body=%s", response.Code, response.Body.String())
	}
}
