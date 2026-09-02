package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"study-guardian/internal/activitywatch"
	"study-guardian/internal/api"
	"study-guardian/internal/config"
	"study-guardian/internal/reminder"
	"study-guardian/internal/rules"
	"study-guardian/internal/sensor"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func main() {
	configPath := flag.String("config", "", "Path to config YAML file")
	tokenPath := flag.String("token", "", "Path to auth token file")
	dbPath := flag.String("db", "", "Path to SQLite database")
	awURL := flag.String("aw-url", "http://127.0.0.1:5600", "ActivityWatch base URL")
	flag.Parse()

	log.Printf("[Supervisor] Starting StudyGuardian Supervisor...")

	cfg, err := config.LoadConfig(*configPath, *tokenPath)
	if err != nil {
		log.Fatalf("[Supervisor] Error loading config: %v", err)
	}

	// Resolve database path
	targetDB := *dbPath
	if targetDB == "" {
		targetDB = "data/studyguardian.db"
	}
	_ = os.MkdirAll(filepath.Dir(targetDB), 0755)

	store, err := storage.OpenSQLite(targetDB)
	if err != nil {
		log.Printf("[Supervisor] Warning: Failed to open persistent SQLite (%v), falling back to in-memory", err)
		store, _ = storage.OpenSQLite(":memory:")
	}
	defer store.Close()

	clock := state.RealClock{}
	privacyGate := rules.NewPrivacyGate(cfg)
	ruleEngine := rules.NewRuleEngine()
	reminderEng := reminder.NewEngine(cfg)

	stateMgr := state.NewPersistentManager(clock, cfg, store, ruleEngine, privacyGate, reminderEng)
	server := api.NewServer(cfg, stateMgr)

	// ActivityWatch & Screen Sensor clients
	awClient := activitywatch.NewClient(*awURL)
	sensorClient := sensor.NewHTTPClient(cfg.IPC.SensorHost, cfg.IPC.SensorPort, cfg.IPC.AuthToken)

	// Start API server in goroutine
	go func() {
		log.Printf("[Supervisor] API listening on http://%s:%d", cfg.IPC.SupervisorHost, cfg.IPC.SupervisorPort)
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[Supervisor] Server error: %v", err)
		}
	}()

	// Background supervision worker
	tickerCtx, cancelTicker := context.WithCancel(context.Background())
	defer cancelTicker()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		var lastScreenChanged bool
		for {
			select {
			case <-tickerCtx.Done():
				return
			case t := <-ticker.C:
				awOK := awClient.Health(tickerCtx)
				sensorHealth, _ := sensorClient.Health(tickerCtx)
				sensorOK := (sensorHealth != nil && sensorHealth.Status == "ok")

				stateMgr.SetHealth(awOK, sensorOK)

				app := ""
				title := ""
				domain := ""
				isAFK := false

				if awOK {
					snap, err := awClient.GetLatestActivity(tickerCtx)
					if err == nil && snap != nil {
						app = snap.App
						title = snap.Title
						domain = snap.Domain
						isAFK = snap.IsAFK
					}
				}

				// If AFK and screen sensor is available and enabled, capture diff
				if isAFK && sensorOK && cfg.Screen.Enabled {
					// Check privacy gate first before capturing!
					priv := privacyGate.Evaluate(app, title, domain)
					if priv == state.PrivacyNormal {
						capResp, err := sensorClient.Capture(tickerCtx, sensor.CaptureRequest{
							Monitor:              1,
							IncludeAnalysisImage: false,
						})
						if err == nil && capResp != nil {
							lastScreenChanged = capResp.Changed
						}
					} else {
						lastScreenChanged = false
					}
				}

				stateMgr.Tick(t, app, title, domain, isAFK, lastScreenChanged)
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[Supervisor] Shutting down...")
	cancelTicker()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[Supervisor] Shutdown error: %v", err)
	}
	log.Printf("[Supervisor] Goodbye.")
}
