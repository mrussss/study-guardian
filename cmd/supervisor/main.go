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
	"study-guardian/internal/classifier/providers"
	"study-guardian/internal/config"
	"study-guardian/internal/motivation"
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

	// Configure AI through the explicit registry. Unknown providers never
	// silently become FakeProvider; fake is reserved for developer/test mode.
	aiRegistry := providers.New(cfg)
	aiProvider := aiRegistry.Provider()
	if cfg.AI.MigrationWarning != "" {
		log.Printf("[AI] Warning: %s", cfg.AI.MigrationWarning)
	}

	classifierService := classifier.NewServiceWithProviders(cfg, ruleEngine, privacyGate, aiProvider, aiRegistry.VisionProvider(), store)
	motivationService := motivation.NewService(cfg, store)

	stateMgr := state.NewPersistentManager(clock, cfg, store, ruleEngine, privacyGate, reminderEng)
	stateMgr.SetToastNotifier(windows.SendToast)

	server := api.NewServer(cfg, stateMgr)
	server.SetMotivation(motivationService)
	server.SetAIStatus(func() interface{} { return aiRegistry.Status() })

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
		var lastCaptureTime time.Time
		lastClassRes := state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "No observation yet"}

		for {
			select {
			case <-tickerCtx.Done():
				return
			case t := <-ticker.C:
				awOK := awClient.Health(tickerCtx)
				sensorHealth, _ := sensorClient.Health(tickerCtx)
				// Issue 6 Fix: Check MSSAvailable
				sensorOK := (sensorHealth != nil && sensorHealth.Status == "ok" && sensorHealth.MSSAvailable)

				stateMgr.SetHealth(awOK, sensorOK)
				sysStatus := stateMgr.GetStatus()

				app := ""
				title := ""
				domain := ""
				isAFK := false
				imageBase64 := ""
				isStale := false

				if awOK {
					snap, err := awClient.GetLatestActivity(tickerCtx)
					if err == nil && snap != nil {
						// Issue 4 Fix: Drop stale events (older than 2 minutes)
						if !snap.IsFresh(t, 2*time.Minute) {
							isStale = true
						} else {
							app = snap.App
							title = snap.Title
							domain = snap.Domain
							isAFK = snap.IsAFK
						}
					}
				}

				if isStale || !awOK {
					app = ""
					title = ""
					domain = ""
					isAFK = false // AW Offline/Stale forces Unknown Interaction
				}
				// aw-server health alone does not prove watcher freshness. A stale
				// window event must stop active-time accumulation and force UNKNOWN.
				stateMgr.SetHealth(awOK && !isStale, sensorOK)

				isLocked := windows.IsLocked()
				if isLocked {
					isAFK = true
				}

				// Issue 3 Fix: Dynamic sampling and skipping AI in OFF mode
				sampleInterval := cfg.Screen.ActiveSampleSeconds
				if sysStatus.UserMode == state.UserModeBreak {
					sampleInterval = cfg.Screen.BreakSampleSeconds
				} else if isAFK {
					sampleInterval = cfg.Screen.UnknownSampleSeconds
				}
				if sampleInterval <= 0 {
					sampleInterval = 15
				}

				shouldSample := sysStatus.UserMode != state.UserModeOff && time.Since(lastCaptureTime) >= time.Duration(sampleInterval)*time.Second
				priv := state.PrivacyNormal

				if sysStatus.UserMode != state.UserModeOff {
					priv = privacyGate.Evaluate(app, title, domain)
				}

				if !awOK || isStale {
					// Without a fresh ActivityWatch event there is no current
					// activity to classify. Keep the system UNKNOWN and avoid
					// unnecessary screenshots or AI calls.
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "ActivityWatch unavailable or stale"}
					lastScreenChanged = false
				} else if shouldSample {
					if sensorOK && cfg.Screen.Enabled && priv == state.PrivacyNormal {
						includeImg := cfg.AI.Enabled && cfg.AI.UseVisionOnlyWhenNeeded && sysStatus.UserMode == state.UserModeStudy
						capResp, err := sensorClient.Capture(tickerCtx, sensor.CaptureRequest{
							Monitor:              cfg.Screen.Monitor,
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
					lastCaptureTime = t

					// BREAK is time-only: do not judge entertainment or invoke AI.
					if sysStatus.UserMode == state.UserModeBreak {
						lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "BREAK mode"}
					} else {
						currentTask := stateMgr.GetCurrentTask()
						lastClassRes = classifierService.Classify(tickerCtx, app, title, domain, currentTask, lastScreenHash, string(sysStatus.UserMode), imageBase64)
					}
				} else if sysStatus.UserMode == state.UserModeOff {
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "System is OFF"}
					lastScreenChanged = false
				} else if sysStatus.UserMode == state.UserModeBreak {
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "BREAK mode"}
				} else {
					// Between samples, just run rule engine (very cheap) to keep reaction fast if window changes
					// But we don't do AI or Capture.
					ruleRes := ruleEngine.Classify(app, title, domain, stateMgr.GetCurrentTask())
					if ruleRes.Relation != state.RelationUnknown {
						lastClassRes = ruleRes
					}
				}

				outcome := stateMgr.TickWithClassification(t, app, title, domain, isAFK, lastScreenChanged, isLocked, lastClassRes)
				motivationService.RecordTick(outcome)
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[Supervisor] Shutting down...")
	cancelTicker()
	stateMgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[Supervisor] Shutdown error: %v", err)
	}
	log.Printf("[Supervisor] Goodbye.")
}
