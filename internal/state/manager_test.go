package state

import (
	"context"
	"sync"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

type captureReminderEvaluator struct {
	last ReminderDecisionInput
}

func (r *captureReminderEvaluator) Evaluate(input ReminderDecisionInput) *ReminderEvent {
	r.last = input
	return nil
}

type mockRuleClassifier struct{}

func (mockRuleClassifier) Classify(app, title, domain, task string) ClassificationResult {
	if app == "steam.exe" {
		return ClassificationResult{
			Relation:   RelationDistracted,
			Confidence: 0.95,
			Reason:     "Steam distraction",
			IsFromRule: true,
		}
	}
	return ClassificationResult{
		Relation:   RelationFocused,
		Confidence: 0.90,
		Reason:     "Dev work",
		IsFromRule: true,
	}
}

type mockPrivacyEvaluator struct{}

func (mockPrivacyEvaluator) Evaluate(app, title, domain string) PrivacyState {
	if app == "bitwarden.exe" {
		return PrivacySensitive
	}
	return PrivacyNormal
}

type mockReminderEvaluator struct{}

func (mockReminderEvaluator) Evaluate(input ReminderDecisionInput) *ReminderEvent {
	if input.DistractedSeconds >= 480 {
		return &ReminderEvent{
			ID:        "rem-distract",
			Level:     ReminderLevelBubble,
			Message:   "Distraction warning",
			Reason:    "DISTRACTION_WARN",
			CreatedAt: input.Now,
		}
	}
	return nil
}

func TestManagerTickAndMidnightReset(t *testing.T) {
	now := time.Date(2026, 9, 2, 23, 50, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()

	mgr := NewPersistentManager(
		clock,
		cfg,
		store,
		mockRuleClassifier{},
		mockPrivacyEvaluator{},
		mockReminderEvaluator{},
	)

	// Set to STUDY
	err := mgr.SetModeStudy("Go Concurrency Lab")
	if err != nil {
		t.Fatalf("failed to set STUDY: %v", err)
	}

	// Tick 5 seconds of active study
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	st := mgr.GetStatus()
	if st.InteractionState != InteractionActive || st.TaskRelation != RelationFocused {
		t.Fatalf("expected ACTIVE & FOCUSED, got %s & %s", st.InteractionState, st.TaskRelation)
	}

	// Advance time across midnight (to 00:05:00 next day)
	nextDay := time.Date(2026, 9, 3, 0, 5, 0, 0, time.Local)
	clock.Set(nextDay)
	mgr.Tick(nextDay, "code.exe", "main.go", "", false, true, false)

	st = mgr.GetStatus()
	// Should reset to STANDBY
	if st.UserMode != UserModeStandby {
		t.Fatalf("expected reset to STANDBY on midnight cross, got %s", st.UserMode)
	}
	// Task name should be preserved for UI suggestion
	if st.Task != "Go Concurrency Lab" {
		t.Fatalf("expected preserved task name, got %s", st.Task)
	}
	// Study seconds for the new day should be reset
	if st.StudySeconds != 0 {
		t.Fatalf("expected 0 study seconds for new day, got %d", st.StudySeconds)
	}
}

func TestManagerDistractionReminderTrigger(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, _ := storage.OpenSQLite(":memory:")
	defer store.Close()

	mgr := NewPersistentManager(
		clock,
		cfg,
		store,
		mockRuleClassifier{},
		mockPrivacyEvaluator{},
		mockReminderEvaluator{},
	)

	_ = mgr.SetModeStudy("Writing Report")

	// Tick 500 seconds with Steam
	for i := 1; i <= 100; i++ {
		now = now.Add(5 * time.Second)
		clock.Set(now)
		mgr.Tick(now, "steam.exe", "Steam Store", "", false, true, false)
	}

	st := mgr.GetStatus()
	if st.TaskRelation != RelationDistracted {
		t.Fatalf("expected DISTRACTED relation, got %s", st.TaskRelation)
	}
	if st.CurrentReminder == nil || st.CurrentReminder.Level != ReminderLevelBubble {
		t.Fatalf("expected Bubble reminder after 500s distraction, got %+v", st.CurrentReminder)
	}
}

func TestManagerSupervisionStateChainAndActivityWatchFailSoft(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)
	clock := NewFakeClock(now)
	mgr := NewPersistentManager(clock, config.DefaultConfig(), nil, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(5 * time.Second)
	clock.Set(now)
	active := mgr.Tick(now, "code.exe", "main.go", "", false, false, false)
	if active.Interaction != InteractionActive || active.Relation != RelationFocused || !active.ActivityValid {
		t.Fatalf("expected ACTIVE/FOCUSED with healthy ActivityWatch, got %+v", active)
	}

	now = now.Add(5 * time.Second)
	clock.Set(now)
	static := mgr.Tick(now, "code.exe", "main.go", "", true, false, false)
	if static.Interaction != InteractionIdleStatic || static.IdleStaticSeconds <= 0 {
		t.Fatalf("expected IDLE_STATIC with unchanged screen, got %+v", static)
	}

	now = now.Add(5 * time.Second)
	clock.Set(now)
	dynamic := mgr.Tick(now, "code.exe", "main.go", "", true, true, false)
	if dynamic.Interaction != InteractionIdleDynamic || dynamic.IdleStaticSeconds != 0 {
		t.Fatalf("expected IDLE_DYNAMIC with screen change, got %+v", dynamic)
	}
	if dynamic.AfkSeconds <= static.AfkSeconds || dynamic.AfkSeconds <= 0 || dynamic.AfkSince == nil {
		t.Fatalf("AFK duration must span STATIC to DYNAMIC, static=%+v dynamic=%+v", static, dynamic)
	}
	now = now.Add(5 * time.Second)
	clock.Set(now)
	activeAgain := mgr.Tick(now, "code.exe", "main.go", "", false, false, false)
	if activeAgain.AfkSeconds != 0 || activeAgain.AfkSince != nil {
		t.Fatalf("active input must clear AFK interval: %+v", activeAgain)
	}
	now = now.Add(5 * time.Second)
	clock.Set(now)
	afkClassification := mgr.TickWithClassification(now, "code.exe", "main.go", "", true, true, false, ClassificationResult{Relation: RelationFocused, Confidence: .99, SourceKind: SourceKindTextAI})
	if afkClassification.Relation != RelationUnknown || afkClassification.Classification.SourceKind != SourceKindLocalRule || afkClassification.Classification.Reason != "AFK; AI skipped" {
		t.Fatalf("AFK must not inherit or persist AI relation: %+v", afkClassification)
	}

	beforeOffline := mgr.GetStatus().ActiveSeconds
	mgr.SetHealth(false, true)
	now = now.Add(5 * time.Second)
	clock.Set(now)
	offline := mgr.Tick(now, "code.exe", "main.go", "", false, false, false)
	status := mgr.GetStatus()
	if offline.Interaction != InteractionUnknown || offline.ActivityValid || status.ActivityWatchOK {
		t.Fatalf("expected UNKNOWN and unhealthy ActivityWatch, outcome=%+v status=%+v", offline, status)
	}
	if status.ActiveSeconds != beforeOffline {
		t.Fatalf("offline ActivityWatch must not add active seconds: before=%d after=%d", beforeOffline, status.ActiveSeconds)
	}

	mgr.SetHealth(true, true)
	now = now.Add(5 * time.Second)
	clock.Set(now)
	recovered := mgr.Tick(now, "code.exe", "main.go", "", false, false, false)
	if recovered.Interaction != InteractionActive || !recovered.ActivityValid {
		t.Fatalf("expected ACTIVE after ActivityWatch recovery, got %+v", recovered)
	}
}

func TestManagerRestartRecoversOnlyInterruptedSession(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	dbPath := t.TempDir() + "/studyguardian.db"
	cfg := config.DefaultConfig()
	clock := NewFakeClock(now)
	store1, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mgr1 := NewPersistentManager(clock, cfg, store1, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if err := mgr1.SetModeOff(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(12 * time.Second)
	clock.Set(now)
	mgr1.Tick(now, "", "", "", true, false, false)
	// Simulate a process kill: close the database without a clean manager close.
	_ = store1.Close()

	store2, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	mgr2 := NewPersistentManager(clock, cfg, store2, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if got := mgr2.GetStatus().UserMode; got != UserModeOff {
		t.Fatalf("expected same-day OFF recovery, got %s", got)
	}
	openCount, err := store2.CountOpenSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if openCount != 1 {
		t.Fatalf("expected exactly one open session after recovery, got %d", openCount)
	}

	mgr2.Close()
	_ = store2.Close()
	store3, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store3.Close()
	mgr3 := NewPersistentManager(clock, cfg, store3, mockRuleClassifier{}, mockPrivacyEvaluator{}, &captureReminderEvaluator{})
	if got := mgr3.GetStatus().UserMode; got != UserModeStandby {
		t.Fatalf("expected cleanly completed OFF not to be restored, got %s", got)
	}
}

func TestManagerLockAndLongGapDoNotAddUserTime(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	mgr := NewPersistentManager(clock, config.DefaultConfig(), nil, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	_ = mgr.SetModeStudy("Go")

	now = now.Add(10 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	beforeLock := mgr.GetStatus().StudySeconds

	now = now.Add(20 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, true)
	locked := mgr.GetStatus()
	if locked.StudySeconds != beforeLock || locked.InteractionState != InteractionUnknown || locked.TaskRelation != RelationUnknown {
		t.Fatalf("lock screen must pause time and clear observation, before=%d after=%+v", beforeLock, locked)
	}

	now = now.Add(2 * time.Hour)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	afterResume := mgr.GetStatus()
	if afterResume.StudySeconds != beforeLock {
		t.Fatalf("long resume gap must not add user time, got %d want %d", afterResume.StudySeconds, beforeLock)
	}

	now = now.Add(4 * time.Second)
	clock.Set(now)
	resumed := mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	if resumed.DeltaSeconds != 4 || mgr.GetStatus().StudySeconds != beforeLock+4 {
		t.Fatalf("normal ticks after resume should recover timing, outcome=%+v status=%+v", resumed, mgr.GetStatus())
	}
}

func TestBreakReminderUsesCurrentBreakSessionDuration(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	reminders := &captureReminderEvaluator{}
	mgr := NewPersistentManager(clock, config.DefaultConfig(), nil, mockRuleClassifier{}, mockPrivacyEvaluator{}, reminders)
	_ = mgr.SetModeBreak()
	now = now.Add(15 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, false)
	_ = mgr.SetModeStudy("Go")
	now = now.Add(1 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	_ = mgr.SetModeBreak()
	now = now.Add(5 * time.Second)
	clock.Set(now)
	mgr.Tick(now, "", "", "", true, false, false)
	if reminders.last.BreakSeconds != 5 {
		t.Fatalf("expected current BREAK duration 5, got %d", reminders.last.BreakSeconds)
	}
}

func TestSetTaskPersistsIntoOpenSessionAndRestartRecovery(t *testing.T) {
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.Local)
	dbPath := t.TempDir() + "/studyguardian.db"
	cfg := config.DefaultConfig()
	clock := NewFakeClock(now)
	store1, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	mgr1 := NewPersistentManager(clock, cfg, store1, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	if err := mgr1.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := mgr1.SetTask("  算法   练习  "); err != nil {
		t.Fatal(err)
	}
	open, err := store1.LoadOpenSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if open.Task != "算法 练习" {
		t.Fatalf("open session task=%q", open.Task)
	}
	_ = store1.Close() // process-kill simulation: preserve the open row

	store2, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	mgr2 := NewPersistentManager(clock, cfg, store2, mockRuleClassifier{}, mockPrivacyEvaluator{}, nil)
	if got := mgr2.GetStatus().Task; got != "算法 练习" {
		t.Fatalf("recovered task=%q, want 算法 练习", got)
	}
}

func TestManagerClearsReminderAfterStableFocusedRecovery(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mgr := NewPersistentManager(clock, cfg, store, mockRuleClassifier{}, mockPrivacyEvaluator{}, mockReminderEvaluator{})
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		now = now.Add(5 * time.Second)
		clock.Set(now)
		mgr.Tick(now, "steam.exe", "Steam", "", false, true, false)
	}
	if mgr.GetStatus().CurrentReminder == nil {
		t.Fatal("expected distraction reminder")
	}
	for i := 0; i < 5; i++ {
		now = now.Add(5 * time.Second)
		clock.Set(now)
		mgr.Tick(now, "code.exe", "main.go", "", false, true, false)
	}
	if got := mgr.GetStatus().CurrentReminder; got != nil {
		t.Fatalf("reminder cleared before recovery window: %+v", got)
	}
}

func TestAutomationConfirmationAcceptRejectAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	cfg.Automation.ManualOverrideMinutes = 0
	mgr := NewPersistentManager(clock, cfg, nil, nil, nil, nil)
	intent := AutomationIntent{ID: "intent-start", Transition: AutomationStart, Task: "Go", Reason: PauseReasonNone, RequiresConfirmation: true}
	if err := mgr.ApplyAutomationIntent(intent); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.UserMode != UserModeStandby || got.PendingAutomationIntent == nil {
		t.Fatalf("pending=%+v", got)
	}
	if err := mgr.RejectAutomationIntent("intent-start"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.PendingAutomationIntent != nil || got.UserMode != UserModeStandby {
		t.Fatalf("rejected intent still active: %+v", got)
	}

	if err := mgr.ApplyAutomationIntent(intent); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AcceptAutomationIntent("intent-start"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.UserMode != UserModeStudy || got.Task != "Go" {
		t.Fatalf("accepted intent did not start study: %+v", got)
	}

	if err := mgr.SetModeOff(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ApplyAutomationIntent(intent); err != nil {
		t.Fatal(err)
	}
	now = now.Add(16 * time.Second)
	clock.Set(now)
	if got := mgr.GetStatus(); got.PendingAutomationIntent == nil {
		t.Fatalf("expired intent should remain pending until processed: %+v", got)
	}
	if err := mgr.ProcessExpiredAutomationIntent(now); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.PendingAutomationIntent != nil || got.UserMode != UserModeOff {
		t.Fatalf("expired AUTO_START should dismiss without starting: %+v", got)
	}
}

func TestManualStudyStillAllowsAutomaticPause(t *testing.T) {
	now := time.Date(2026, 9, 8, 18, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	cfg.Automation.ManualOverrideMinutes = 30
	mgr := NewPersistentManager(clock, cfg, nil, nil, nil, nil)
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ApplyAutomationIntent(AutomationIntent{Transition: AutomationPause, Reason: PauseReasonLocked}); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.UserMode != UserModeBreak || got.ModeOrigin != ModeOriginAutomation || !got.AutoResumeEligible {
		t.Fatalf("manual study was not auto-paused: %+v", got)
	}
}

func TestExpiredAutoPauseAppliesOnlyWhenExplicitlyProcessed(t *testing.T) {
	now := time.Date(2026, 9, 8, 19, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	mgr := NewPersistentManager(clock, cfg, nil, nil, nil, nil)
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	intent := AutomationIntent{ID: "intent-pause", Transition: AutomationPause, Reason: PauseReasonIdle, RequiresConfirmation: true}
	if err := mgr.ApplyAutomationIntent(intent); err != nil {
		t.Fatal(err)
	}
	clock.Set(now.Add(16 * time.Second))
	if got := mgr.GetStatus(); got.UserMode != UserModeStudy || got.PendingAutomationIntent == nil {
		t.Fatalf("GetStatus had side effects: %+v", got)
	}
	if err := mgr.ProcessExpiredAutomationIntent(clock.Now()); err != nil {
		t.Fatal(err)
	}
	got := mgr.GetStatus()
	if got.UserMode != UserModeBreak || got.ModeOrigin != ModeOriginAutomation || !got.AutoResumeEligible || got.PendingAutomationIntent != nil {
		t.Fatalf("expired AUTO_PAUSE was not applied: %+v", got)
	}
	if err := mgr.ProcessExpiredAutomationIntent(clock.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetStatus(); got.UserMode != UserModeBreak {
		t.Fatalf("second expiry processing changed mode: %+v", got)
	}
}

func TestRejectAutomaticPauseCreatesShortSnooze(t *testing.T) {
	now := time.Date(2026, 9, 8, 20, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	mgr := NewPersistentManager(clock, cfg, nil, nil, nil, nil)
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ApplyAutomationIntent(AutomationIntent{ID: "intent-pause", Transition: AutomationPause, Reason: PauseReasonIdle, RequiresConfirmation: true}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RejectAutomationIntent("intent-pause"); err != nil {
		t.Fatal(err)
	}
	got := mgr.GetStatus()
	if got.AutoPauseSnoozeUntil == nil || !got.AutoPauseSnoozeUntil.After(now.Add(3*time.Minute)) || !got.AutoPauseSnoozeUntil.Before(now.Add(5*time.Minute)) {
		t.Fatalf("snooze=%v", got.AutoPauseSnoozeUntil)
	}
	if got.UserMode != UserModeStudy {
		t.Fatalf("reject changed mode: %+v", got)
	}
}

func TestExpiredAutoPauseAcceptAndRejectApplyExpirySemantics(t *testing.T) {
	now := time.Date(2026, 9, 9, 9, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	first := NewPersistentManager(clock, cfg, nil, nil, nil, nil)
	if err := first.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := first.ApplyAutomationIntent(AutomationIntent{ID: "pause-accept", Transition: AutomationPause, Reason: PauseReasonIdle, RequiresConfirmation: true}); err != nil {
		t.Fatal(err)
	}
	clock.Set(now.Add(16 * time.Second))
	if err := first.AcceptAutomationIntent("pause-accept"); err != nil {
		t.Fatal(err)
	}
	if got := first.GetStatus(); got.UserMode != UserModeBreak || got.PendingAutomationIntent != nil {
		t.Fatalf("expired accept status=%+v", got)
	}
	if err := first.ProcessExpiredAutomationIntent(clock.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := first.GetStatus(); got.UserMode != UserModeBreak {
		t.Fatalf("post-accept processing changed mode=%s", got.UserMode)
	}

	secondClock := NewFakeClock(now)
	second := NewPersistentManager(secondClock, cfg, nil, nil, nil, nil)
	if err := second.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := second.ApplyAutomationIntent(AutomationIntent{ID: "pause-reject", Transition: AutomationPause, Reason: PauseReasonIdle, RequiresConfirmation: true}); err != nil {
		t.Fatal(err)
	}
	secondClock.Set(now.Add(16 * time.Second))
	if err := second.RejectAutomationIntent("pause-reject"); err != nil {
		t.Fatal(err)
	}
	if got := second.GetStatus(); got.UserMode != UserModeBreak || got.PendingAutomationIntent != nil || got.AutoPauseSnoozeUntil != nil {
		t.Fatalf("expired reject status=%+v", got)
	}
}

func TestExpiredAutomationIntentIsExactlyOnceAcrossProcessAndAccept(t *testing.T) {
	now := time.Date(2026, 9, 9, 10, 0, 0, 0, time.Local)
	clock := NewFakeClock(now)
	cfg := config.DefaultConfig()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mgr := NewPersistentManager(clock, cfg, store, nil, nil, nil)
	if err := mgr.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ApplyAutomationIntent(AutomationIntent{ID: "pause-race", Transition: AutomationPause, Reason: PauseReasonIdle, RequiresConfirmation: true}); err != nil {
		t.Fatal(err)
	}
	clock.Set(now.Add(16 * time.Second))

	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- mgr.ProcessExpiredAutomationIntent(clock.Now())
	}()
	go func() {
		defer wg.Done()
		results <- mgr.AcceptAutomationIntent("pause-race")
	}()
	wg.Wait()
	close(results)

	knownConsumedError := 0
	for err := range results {
		if err == nil {
			continue
		}
		if err.Error() != "automation intent is not pending" {
			t.Fatalf("unexpected concurrent decision error: %v", err)
		}
		knownConsumedError++
	}
	if knownConsumedError > 1 {
		t.Fatalf("both concurrent callers reported a consumed intent: %d", knownConsumedError)
	}
	if got := mgr.GetStatus(); got.UserMode != UserModeBreak || got.PendingAutomationIntent != nil {
		t.Fatalf("race status=%+v", got)
	}
	sessions, err := store.ListSessionsForDate(context.Background(), storage.LocalDate(now))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("session transitions=%d, want standby, study close, plus one automatic pause", len(sessions))
	}
}
