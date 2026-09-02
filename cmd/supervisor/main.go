package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"study-guardian/internal/activitywatch"
	"study-guardian/internal/api"
	"study-guardian/internal/classifier"
	"study-guardian/internal/config"
	"study-guardian/internal/platform/windows"
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

	// Resolve database path
	targetDB := *dbPath
	if targetDB == "" {
		targetDB = "data/studyguardian.db"
	}
	_ = os.MkdirAll(filepath.Dir(targetDB), 0755)

	// Set up rotating logger in logs directory next to data
	logPath := filepath.Join(filepath.Dir(targetDB), "..", "logs", "supervisor.log")
	logFile, err := windows.SetupLogger(logPath)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	} else {
		log.Printf("Warning: Failed to setup rotating logger at %s: %v", logPath, err)
	}

	log.Printf("[Supervisor] Starting StudyGuardian Supervisor...")

	cfg, err := config.LoadConfig(*configPath, *tokenPath)
	if err != nil {
		log.Fatalf("[Supervisor] Error loading config: %v", err)
	}

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

	// Configure AI Provider
	var aiProvider classifier.TaskRelationProvider
	if cfg.AI.Enabled {
		if cfg.AI.Provider == "openai" || cfg.AI.Provider == "deepseek" || cfg.AI.Provider == "ollama" {
			aiProvider = classifier.NewOpenAICompatibleProvider(cfg.AI.Endpoint, cfg.AI.APIKey, cfg.AI.Model)
		} else {
			aiProvider = classifier.NewFakeProvider()
		}
	}

	classifierService := classifier.NewService(cfg, ruleEngine, privacyGate, aiProvider, store)

	stateMgr := state.NewPersistentManager(clock, cfg, store, ruleEngine, privacyGate, reminderEng)
	stateMgr.SetToastNotifier(windows.SendToast)

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
		var lastScreenHash string

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
				imageBase64 := ""

				if awOK {
					snap, err := awClient.GetLatestActivity(tickerCtx)
					if err == nil && snap != nil {
						app = snap.App
						title = snap.Title
						domain = snap.Domain
						isAFK = snap.IsAFK
					}
				}

				// Check privacy gate first!
				priv := privacyGate.Evaluate(app, title, domain)

				// If screen sensor available & enabled & not sensitive
				if sensorOK && cfg.Screen.Enabled && priv == state.PrivacyNormal {
					includeImg := cfg.AI.Enabled && cfg.AI.UseVisionOnlyWhenNeeded
					capResp, err := sensorClient.Capture(tickerCtx, sensor.CaptureRequest{
						Monitor:              1,
						IncludeAnalysisImage: includeImg,
						MaxWidth:             960,
					})
					if err == nil && capResp != nil {
						lastScreenChanged = capResp.Changed
						lastScreenHash = capResp.Hash
						if capResp.AnalysisImage != nil {
							imageBase64 = *capResp.AnalysisImage
						}
					}
				} else {
					lastScreenChanged = false
				}

				// Perform classification
				currentTask := stateMgr.GetCurrentTask()
				classRes := classifierService.Classify(tickerCtx, app, title, domain, currentTask, lastScreenHash, imageBase64)

				stateMgr.TickWithClassification(t, app, title, domain, isAFK, lastScreenChanged, classRes)
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
