package main

import (
	"context"
	"encoding/json"
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
	"study-guardian/internal/aisettings"
	"study-guardian/internal/api"
	"study-guardian/internal/automation"
	"study-guardian/internal/classifier"
	"study-guardian/internal/classifier/providers"
	"study-guardian/internal/config"
	"study-guardian/internal/distraction"
	"study-guardian/internal/motivation"
	"study-guardian/internal/platform/windows"
	"study-guardian/internal/reminder"
	"study-guardian/internal/review"
	"study-guardian/internal/rules"
	"study-guardian/internal/semantic"
	"study-guardian/internal/sensor"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

func main() {
	configPath := flag.String("config", "", "Path to config YAML file")
	tokenPath := flag.String("token", "", "Path to auth token file")
	collectorTokenPath := flag.String("collector-token", "", "Path to scoped collector token file")
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
	collectorTokenFile := *collectorTokenPath
	if collectorTokenFile == "" {
		if *tokenPath != "" {
			collectorTokenFile = filepath.Join(filepath.Dir(*tokenPath), "collector-token")
		} else {
			collectorTokenFile = filepath.Join(filepath.Dir(targetDB), "..", "config", "collector-token")
		}
	}
	collectorToken, err := config.EnsureToken(collectorTokenFile)
	if err != nil {
		log.Fatalf("[Supervisor] Error resolving collector token: %v", err)
	}
	cfg.IPC.CollectorToken = collectorToken

	store, err := storage.OpenSQLite(targetDB)
	if err != nil {
		log.Printf("[Supervisor] Warning: Failed to open persistent SQLite (%v), falling back to in-memory", err)
		store, _ = storage.OpenSQLite(":memory:")
	}
	defer store.Close()
	configDir := filepath.Join(filepath.Dir(targetDB), "..", "config")
	if *configPath != "" {
		configDir = filepath.Dir(*configPath)
	}
	if migrated, migrateErr := aisettings.LoadAndMigratePersistedAI(context.Background(), store, filepath.Join(configDir, "secrets"), cfg.AI); migrateErr != nil {
		log.Printf("[AI] settings migration failed; original settings retained (%v)", migrateErr)
		cfg.AI.MigrationWarning = "persisted AI settings unavailable; using YAML/default configuration"
	} else if migrated.Found {
		cfg.AI = migrated.Config
		config.NormalizeAIConfig(cfg, false)
	}
	if raw, ok, loadErr := store.GetSetting(context.Background(), "automation.config.v1"); loadErr != nil {
		log.Printf("[Automation] settings load failed: %v", loadErr)
	} else if ok {
		var persisted automation.Settings
		if decodeErr := json.Unmarshal([]byte(raw), &persisted); decodeErr == nil && automation.ValidateSettings(persisted) == nil {
			cfg.Automation = automation.ConfigFromSettings(persisted)
		}
	}
	runRetentionCleanup := func(ctx context.Context) {
		retentionStats, err := store.PruneRetention(ctx, time.Now(), cfg.Review.Retention.RawChatDays, cfg.Review.Retention.SemanticDays)
		if err != nil {
			log.Printf("[Retention] cleanup failed: %v", err)
		} else if retentionStats.RawMessagesDeleted > 0 || retentionStats.RawTurnsDeleted > 0 || retentionStats.RawConversationsDeleted > 0 || retentionStats.SemanticDeleted > 0 {
			log.Printf("[Retention] removed raw_messages=%d raw_turns=%d conversations=%d semantic_snapshots=%d", retentionStats.RawMessagesDeleted, retentionStats.RawTurnsDeleted, retentionStats.RawConversationsDeleted, retentionStats.SemanticDeleted)
		}
	}
	runRetentionCleanup(context.Background())
	retentionCtx, cancelRetention := context.WithCancel(context.Background())
	defer cancelRetention()
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runRetentionCleanup(retentionCtx)
			case <-retentionCtx.Done():
				return
			}
		}
	}()

	if raw, ok, loadErr := store.GetSetting(context.Background(), "reminder.config.v1"); loadErr != nil {
		log.Printf("[Reminder] settings load failed: %v", loadErr)
	} else if ok {
		var persisted config.ReminderConfig
		if decodeErr := json.Unmarshal([]byte(raw), &persisted); decodeErr == nil {
			if _, validateErr := config.ParseQuietPeriods(persisted.QuietPeriods); validateErr == nil && persisted.CooldownMinutes > 0 {
				cfg.Reminder = persisted
			}
		}
	}

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
	motivationService := motivation.NewServiceWithClock(cfg, store, clock)

	stateMgr := state.NewPersistentManager(clock, cfg, store, ruleEngine, privacyGate, reminderEng)
	stateMgr.SetToastNotifier(windows.SendToast)
	automationController := automation.New(cfg.Automation)
	automationSettings := automation.NewSettingsService(cfg, store, automationController)

	server := api.NewServer(cfg, stateMgr)
	server.SetAutomationSettings(automationSettings)
	server.SetAutomationIntentManager(stateMgr)
	server.SetStorage(store)
	server.SetReminderSettings(reminderEng)
	reviewService := review.NewService(store, time.Local, filepath.Join(filepath.Dir(targetDB), "reviews"))
	reviewService.SetLimits(review.ReviewLimits{MaxTurnChars: cfg.Review.Limits.MaxTurnChars, MaxConversationChars: cfg.Review.Limits.MaxConversationChars, MaxFinalInputChars: cfg.Review.Limits.MaxFinalInputChars})
	if reviewProvider, reviewStatus := review.NewConfiguredProvider(cfg); reviewProvider != nil {
		reviewService.SetProvider(reviewProvider)
	} else if reviewStatus.Warning != "" {
		log.Printf("[Review] Warning: %s", reviewStatus.Warning)
	}
	server.SetReview(reviewService)
	reviewCoordinator := server.ReviewCoordinator()
	if cfg.Review.Trigger.BackfillPreviousDay {
		go func() {
			if _, err := reviewCoordinator.StartPreviousDayIfNeeded(context.Background(), time.Now(), true); err != nil {
				log.Printf("[Review] Previous-day backfill failed: %v", err)
			}
		}()
	}
	server.SetMotivation(motivationService)
	aiSettingsService := aisettings.New(cfg, store, filepath.Join(configDir, "secrets"), classifierService, reviewService)
	server.SetAISettings(aiSettingsService)
	server.SetAIStatus(func() interface{} { return aiSettingsService.Status() })
	semanticService := semantic.NewService(store)
	server.SetSemantic(semanticService)
	distractionTracker := distraction.New(store)
	if err := distractionTracker.Restore(context.Background()); err != nil {
		log.Printf("[Distraction] restore failed: %v", err)
	}

	// ActivityWatch & Screen Sensor clients
	awClient := activitywatch.NewClient(*awURL)
	awAvailability := activitywatch.NewAvailabilityDebouncer()
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
		var latestSnapshot *activitywatch.ActivitySnapshot
		var lastTrustedSnapshot *activitywatch.ActivitySnapshot
		lastAWPhase := activitywatch.PhaseAvailable
		lastObservedMode := state.UserModeStandby
		lastClassRes := state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "No observation yet"}
		lastAFKAudit := false

		for {
			select {
			case <-tickerCtx.Done():
				return
			case t := <-ticker.C:
				latestSnapshot = nil
				awServerOK := awClient.Health(tickerCtx)
				var currentSnapshot *activitywatch.ActivitySnapshot
				activitySampleSuccess := false
				if awServerOK {
					snap, err := awClient.GetLatestActivity(tickerCtx)
					if err == nil && snap != nil && snap.IsFresh(t, 2*time.Minute) {
						currentSnapshot = snap
						activitySampleSuccess = true
					}
				}
				sensorHealth, _ := sensorClient.Health(tickerCtx)
				sensorOK := (sensorHealth != nil && sensorHealth.Status == "ok" && sensorHealth.MSSAvailable)
				awStatus := awAvailability.Observe(t, activitySampleSuccess)
				var lastSuccessAt *time.Time
				if !awStatus.LastSuccessAt.IsZero() {
					value := awStatus.LastSuccessAt
					lastSuccessAt = &value
				}
				stateMgr.SetActivityWatchHealth(state.ActivityWatchDiagnostics{
					LastSuccessAt: lastSuccessAt, ConsecutiveFailures: awStatus.ConsecutiveFailures,
					StableOK: awStatus.StableOK, Phase: state.ActivityWatchHealthPhase(awStatus.Phase),
				}, sensorOK)
				if awStatus.Phase != lastAWPhase {
					log.Printf("[ActivityWatch] %s -> %s", lastAWPhase, awStatus.Phase)
					lastAWPhase = awStatus.Phase
				}
				if err := stateMgr.ProcessExpiredAutomationIntent(t); err != nil {
					log.Printf("[Automation] expired intent apply failed: %v", err)
				}
				sysStatus := stateMgr.GetStatus()

				app := ""
				title := ""
				domain := ""
				isAFK := false
				if activitySampleSuccess {
					latestSnapshot = currentSnapshot
					lastTrustedSnapshot = currentSnapshot
					app = currentSnapshot.App
					title = currentSnapshot.Title
					domain = currentSnapshot.Domain
					isAFK = currentSnapshot.IsAFK
				} else if awStatus.StableOK && lastTrustedSnapshot != nil {
					latestSnapshot = lastTrustedSnapshot
					app = lastTrustedSnapshot.App
					title = lastTrustedSnapshot.Title
					domain = lastTrustedSnapshot.Domain
					isAFK = lastTrustedSnapshot.IsAFK
				}

				isLocked := windows.IsLocked()
				if isLocked {
					isAFK = true
				}
				if !isAFK {
					lastAFKAudit = false
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

				if !activitySampleSuccess {
					// A degraded but not yet unavailable watcher retains the last
					// trusted classification for distraction continuity, but AFK
					// must never inherit the pre-AFK relation.
					if isAFK {
						lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "AFK; AI skipped", SourceKind: state.SourceKindLocalRule, IsFromRule: true}
					} else if !awStatus.StableOK {
						lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "ActivityWatch unavailable or stale", SourceKind: state.SourceKindLocalRule, IsFromRule: true}
					}
					lastScreenChanged = false
				} else if shouldSample {
					if sensorOK && cfg.Screen.Enabled && priv == state.PrivacyNormal {
						capResp, err := sensorClient.Capture(tickerCtx, sensor.CaptureRequest{
							Monitor:              cfg.Screen.Monitor,
							IncludeAnalysisImage: false,
							MaxWidth:             960,
						})
						if err == nil && capResp != nil {
							lastScreenChanged = capResp.Changed
							lastScreenHash = capResp.Hash
						}
					} else {
						lastScreenChanged = false
					}
					lastCaptureTime = t

					currentTask := stateMgr.GetCurrentTask()
					localClassify := func() state.ClassificationResult {
						result := ruleEngine.Classify(app, title, domain, currentTask)
						if result.Relation == state.RelationUnknown && sysStatus.UserMode == state.UserModeBreak {
							result.Reason = "BREAK mode; no local focus evidence"
						}
						return result
					}
					remoteClassify := func() state.ClassificationResult {
						result := classifierService.Classify(tickerCtx, app, title, domain, currentTask, lastScreenHash, string(sysStatus.UserMode), "")
						minConfidence := cfg.AI.MinConfidence
						if minConfidence <= 0 {
							minConfidence = 0.75
						}
						needsVision := cfg.AI.Enabled && cfg.AI.Vision.Enabled && aiRegistry.VisionProvider() != nil && (result.Relation == state.RelationUnknown || result.Confidence < minConfidence)
						if needsVision && sensorOK && cfg.Screen.Enabled && priv == state.PrivacyNormal {
							visionResp, visionErr := sensorClient.Capture(tickerCtx, sensor.CaptureRequest{Monitor: cfg.Screen.Monitor, IncludeAnalysisImage: true, MaxWidth: 960})
							if visionErr == nil && visionResp != nil && visionResp.AnalysisImage != nil {
								result = classifierService.Classify(tickerCtx, app, title, domain, currentTask, lastScreenHash, string(sysStatus.UserMode), *visionResp.AnalysisImage)
							}
						}
						return result
					}
					lastClassRes = classifyObservation(sysStatus.UserMode, isAFK, isLocked, awStatus.StableOK, priv, localClassify, remoteClassify)
					if isAFK && !isLocked && !lastAFKAudit {
						stateMgr.RecordAutomationAudit("AI_SKIPPED_FOR_AFK", nil, 0, "local_interaction")
						lastAFKAudit = true
					}
				} else if sysStatus.UserMode == state.UserModeOff {
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "System is OFF"}
					lastScreenChanged = false
				} else if isAFK {
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "AFK; AI skipped", SourceKind: state.SourceKindLocalRule, IsFromRule: true}
					if !lastAFKAudit {
						stateMgr.RecordAutomationAudit("AI_SKIPPED_FOR_AFK", nil, 0, "local_interaction")
						lastAFKAudit = true
					}
				} else if !activitySampleSuccess || !awStatus.StableOK || priv == state.PrivacySensitive || isLocked {
					lastClassRes = state.ClassificationResult{Relation: state.RelationUnknown, Confidence: 1.0, Reason: "AI unavailable or skipped", SourceKind: state.SourceKindLocalRule, IsFromRule: true}
				} else if sysStatus.UserMode == state.UserModeBreak {
					// BREAK must fully replace the previous result on every
					// poll. In particular, UNKNOWN must clear an earlier FOCUSED
					// result instead of allowing stale evidence to resume study.
					currentTask := stateMgr.GetCurrentTask()
					lastClassRes = classifyObservation(sysStatus.UserMode, isAFK, isLocked, awStatus.StableOK, priv, func() state.ClassificationResult {
						result := ruleEngine.Classify(app, title, domain, currentTask)
						if result.Relation == state.RelationUnknown {
							result.Reason = "BREAK mode; no local focus evidence"
						}
						return result
					}, nil)
				} else {
					// Between samples, just run rule engine (very cheap) to keep
					// reaction fast if the window changes. STUDY/STANDBY retain
					// the existing result-lag behavior for UNKNOWN.
					ruleRes := ruleEngine.Classify(app, title, domain, stateMgr.GetCurrentTask())
					if ruleRes.Relation != state.RelationUnknown {
						lastClassRes = ruleRes
					}
				}

				outcome := stateMgr.TickWithClassification(t, app, title, domain, isAFK, lastScreenChanged, isLocked, lastClassRes)
				motivationService.RecordTick(outcome)
				postStatus := stateMgr.GetStatus()
				if intent := automationController.Evaluate(outcome.Now, outcome, postStatus); intent != nil {
					stateMgr.RecordAutomationAudit("AUTOMATION_CANDIDATE_STARTED", intent, 0, "threshold_reached")
					if err := stateMgr.ApplyAutomationIntent(*intent); err != nil {
						stateMgr.RecordAutomationAudit("AUTOMATION_CANDIDATE_CANCELLED", intent, 0, "apply_rejected")
						log.Printf("[Automation] transition rejected: %v", err)
					} else {
						postStatus = stateMgr.GetStatus()
						log.Printf("[Automation] transition=%s reason=%s", intent.Transition, intent.Reason)
					}
				}
				if lastObservedMode != postStatus.UserMode && postStatus.UserMode == state.UserModeOff {
					if _, err := reviewService.MarkStaleIfChanged(tickerCtx, outcome.Now.In(time.Local).Format("2006-01-02")); err != nil {
						log.Printf("[Review] OFF transition stale check failed: %v", err)
					}
				}
				lastObservedMode = postStatus.UserMode
				// observed_at is the time Supervisor actually observed this
				// candidate, not the ActivityWatch event time or DB insert time.
				// The source event time is used only for the age-based freshness
				// decision, so a stable AW event can still satisfy the transition
				// window across multiple Supervisor ticks.
				observedAt := outcome.Now
				semanticFresh := activitySampleSuccess && latestSnapshot != nil && latestSnapshot.IsFresh(outcome.Now, semantic.DefaultTiming.LiveMaxAge)
				reminderLevel := "NONE"
				if postStatus.CurrentReminder != nil {
					reminderLevel = string(postStatus.CurrentReminder.Level)
				}
				if err := distractionTracker.Observe(tickerCtx, distraction.Input{
					Now: outcome.Now, UserMode: outcome.UserMode, ActivityWatchOK: outcome.ActivityValid,
					ActivityFresh: semanticFresh, Privacy: postStatus.PrivacyState, Relation: outcome.Relation,
					Confidence: outcome.Classification.Confidence, Source: outcome.Classification.SourceKind,
					Interaction: outcome.Interaction, Locked: outcome.Locked, App: app, Title: title,
					Domain: domain, Task: postStatus.Task, ReminderLevel: reminderLevel,
				}); err != nil {
					log.Printf("[Distraction] observation failed: %v", err)
				}
				if err := semanticService.Observe(tickerCtx, semantic.Candidate{
					ObservedAt:     observedAt,
					Fresh:          semanticFresh,
					UserMode:       outcome.UserMode,
					Task:           postStatus.Task,
					Interaction:    outcome.Interaction,
					Relation:       outcome.Relation,
					Privacy:        postStatus.PrivacyState,
					App:            app,
					Title:          title,
					Domain:         domain,
					ScreenHash:     lastScreenHash,
					Classification: outcome.Classification,
				}); err != nil {
					log.Printf("[Semantic] observation failed: %v", err)
				}
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[Supervisor] Shutting down...")
	cancelRetention()
	cancelTicker()
	_ = distractionTracker.Close(context.Background(), time.Now(), "SUPERVISOR_SHUTDOWN")
	stateMgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[Supervisor] Shutdown error: %v", err)
	}
	log.Printf("[Supervisor] Goodbye.")
}
