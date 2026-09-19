package eyecare

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/reminder"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type testModes struct {
	status     state.SystemStatus
	starts     int
	restores   int
	resumes    int
	startError error
}

type failingEyeCareStore struct {
	Store
	completeErr error
	stateErr    error
	settingsErr error
}

type cycleRule struct{}

func (cycleRule) Classify(string, string, string, string) state.ClassificationResult {
	return state.ClassificationResult{Relation: state.RelationFocused, Confidence: 1}
}

type cyclePrivacy struct{}

func (cyclePrivacy) Evaluate(string, string, string) state.PrivacyState { return state.PrivacyNormal }

type cycleReminder struct{}

func (cycleReminder) Evaluate(state.ReminderDecisionInput) *state.ReminderEvent { return nil }

func (s *failingEyeCareStore) SaveEyeCareSettingsAndState(ctx context.Context, key, raw string, now time.Time, value storage.EyeCareStateRecord) error {
	if s.settingsErr != nil {
		return s.settingsErr
	}
	return s.Store.SaveEyeCareSettingsAndState(ctx, key, raw, now, value)
}

func (s *failingEyeCareStore) CompleteEyeCareRequest(ctx context.Context, requestID, action string, value storage.EyeCareStateRecord, audit *storage.EyeCareAuditRecord, resultJSON string, now time.Time) error {
	if s.completeErr != nil {
		err := s.completeErr
		s.completeErr = nil
		return err
	}
	return s.Store.CompleteEyeCareRequest(ctx, requestID, action, value, audit, resultJSON, now)
}

func (s *failingEyeCareStore) SaveEyeCareState(ctx context.Context, value storage.EyeCareStateRecord, audit *storage.EyeCareAuditRecord, requestID string) (bool, error) {
	if s.stateErr != nil {
		err := s.stateErr
		s.stateErr = nil
		return false, err
	}
	return s.Store.SaveEyeCareState(ctx, value, audit, requestID)
}

func (m *testModes) SetModeEyeCareBreak(long bool) error {
	if m.startError != nil {
		return m.startError
	}
	m.starts++
	m.setEyeCareBreakStatus(long)
	return nil
}

func (m *testModes) RestoreEyeCareBreak(long bool) error {
	if m.startError != nil {
		return m.startError
	}
	m.restores++
	m.setEyeCareBreakStatus(long)
	return nil
}

func (m *testModes) setEyeCareBreakStatus(long bool) {
	m.status.UserMode = state.UserModeBreak
	m.status.ModeOrigin = state.ModeOriginEyeCare
	if long {
		m.status.PauseReason = state.PauseReasonEyeCareLong
	} else {
		m.status.PauseReason = state.PauseReasonEyeCareShort
	}
}

func (m *testModes) ResumeEyeCareStudy() error {
	m.resumes++
	m.status.UserMode = state.UserModeStudy
	m.status.ModeOrigin = state.ModeOriginManual
	m.status.PauseReason = state.PauseReasonNone
	return nil
}

func (m *testModes) GetStatus() state.SystemStatus { return m.status }

func newTestService(t *testing.T, start time.Time) (*Service, *storage.Storage, *testModes, *time.Time) {
	t.Helper()
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock := start
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	cfg := config.DefaultConfig().EyeCare
	cfg.Enabled = true
	service, err := NewWithClock(cfg, config.DefaultConfig().Reminder, store, modes, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	return service, store, modes, &clock
}

func addFocus(service *Service, seconds int64, now time.Time) {
	service.RecordCreditedFocus(seconds, state.TickOutcome{
		Now: now, DeltaSeconds: seconds, UserMode: state.UserModeStudy,
		ActivityValid: true, Interaction: state.InteractionActive, Relation: state.RelationFocused,
	}, state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual})
}

func TestEffectiveFocusThresholdIsInclusiveAndDueIsAuditedOnce(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, _, _ := newTestService(t, now)
	addFocus(service, 2399, now)
	if status := service.Status(); status.Phase != Focusing || status.FocusSegmentSeconds != 2399 {
		t.Fatalf("before threshold: %+v", status)
	}
	addFocus(service, 1, now.Add(time.Second))
	if status := service.Status(); status.Phase != ShortBreakDue || status.FocusSegmentSeconds != 2400 {
		t.Fatalf("at threshold: %+v", status)
	}
	addFocus(service, 2, now.Add(3*time.Second))
	status := service.Status()
	if status.Phase != ShortBreakDue || status.Revision != 1 {
		t.Fatalf("due should be emitted once: %+v", status)
	}
	events, err := store.ListEyeCareAudit(context.Background(), 20)
	if err != nil || len(events) != 1 || events[0].EventType != "DUE" {
		t.Fatalf("audit=%+v err=%v", events, err)
	}
}

func TestShortBreakClearsSegmentRetainsLongFocusAndWaitsForExplicitResume(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	started, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "start-short-1"})
	if err != nil || started.Phase != ShortBreak || modes.starts != 1 || modes.status.AutoResumeEligible {
		t.Fatalf("start short: status=%+v err=%v modes=%+v", started, err, modes)
	}
	*clock = now.Add(5*time.Minute + time.Second)
	waiting := service.Status()
	if waiting.Phase != WaitingReturn || waiting.FocusSegmentSeconds != 0 || waiting.FocusSinceLongBreakSeconds != 2400 || waiting.CompletedShortBreaks != 1 {
		t.Fatalf("short completion accounting: %+v", waiting)
	}
	if modes.status.UserMode != state.UserModeBreak {
		t.Fatal("rest completion must not automatically resume study")
	}
	resumed, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "resume-short-1"})
	if err != nil || resumed.Phase != Focusing || modes.status.UserMode != state.UserModeStudy || modes.resumes != 1 {
		t.Fatalf("resume short: status=%+v err=%v", resumed, err)
	}
}

func TestLongBreakTakesPriorityAndOnlyCompletionClearsLongAccumulator(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, modes, clock := newTestService(t, now)
	addFocus(service, 7200, now)
	due := service.Status()
	if due.Phase != LongBreakDue {
		t.Fatalf("long rest should override short due: %+v", due)
	}
	started, err := service.Act(ActionRequest{Action: StartLongBreak, ExpectedRevision: due.Revision, RequestID: "start-long-1"})
	if err != nil || started.Phase != LongBreak || modes.status.PauseReason != state.PauseReasonEyeCareLong {
		t.Fatalf("start long rest: %+v err=%v", started, err)
	}
	*clock = now.Add(20*time.Minute + time.Second)
	waiting := service.Status()
	if waiting.Phase != WaitingReturn || waiting.FocusSegmentSeconds != 0 || waiting.FocusSinceLongBreakSeconds != 0 || waiting.CompletedLongBreaks != 1 {
		t.Fatalf("completed long rest should clear both focus counters: %+v", waiting)
	}
	if modes.status.UserMode != state.UserModeBreak {
		t.Fatal("long rest completion changed user mode automatically")
	}
}

func TestEarlyFinishPreservesFocusAndDefersReminderByEffectiveFocus(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, _, _ := newTestService(t, now)
	addFocus(service, 7200, now)
	due := service.Status()
	started, err := service.Act(ActionRequest{Action: StartLongBreak, ExpectedRevision: due.Revision, RequestID: "early-long-start"})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := service.Act(ActionRequest{Action: FinishEarly, ExpectedRevision: started.Revision, RequestID: "early-long-finish"})
	if err != nil || finished.Phase != Focusing || finished.FocusSinceLongBreakSeconds != 7200 || finished.CompletedLongBreaks != 0 || finished.RetryFocusAfterSeconds != 7800 {
		t.Fatalf("early completion lost protected focus: %+v err=%v", finished, err)
	}
	addFocus(service, 599, now.Add(time.Second))
	if status := service.Status(); status.Phase != Focusing {
		t.Fatalf("reminded before 10 effective focus minutes: %+v", status)
	}
	addFocus(service, 1, now.Add(2*time.Second))
	if status := service.Status(); status.Phase != LongBreakDue {
		t.Fatalf("long reminder did not return after effective-focus delay: %+v", status)
	}
}

func TestSkippedShortReminderWaitsForTenEffectiveMinutes(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, _, _ := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	skipped, err := service.Act(ActionRequest{Action: Skip, ExpectedRevision: due.Revision, RequestID: "skip-short"})
	if err != nil || skipped.Phase != Focusing || skipped.RetryFocusAfterSeconds != 3000 {
		t.Fatalf("skip: %+v err=%v", skipped, err)
	}
	addFocus(service, 599, now.Add(time.Second))
	if status := service.Status(); status.Phase != Focusing {
		t.Fatalf("short reminder returned too early: %+v", status)
	}
	addFocus(service, 1, now.Add(2*time.Second))
	if status := service.Status(); status.Phase != ShortBreakDue {
		t.Fatalf("short reminder failed to return: %+v", status)
	}
}

func TestSkippedLongReminderWaitsTenEffectiveMinutesAfterACompletedShortRest(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, _, clock := newTestService(t, now)
	service.cfg.FocusMinutes = 20
	service.cfg.LongBreakAfterFocusMinutes = 60
	addFocus(service, 1200, now)
	due := service.Status()
	_, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "long-after-short-start"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = now.Add(5*time.Minute + time.Second)
	waiting := service.Status()
	if _, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "long-after-short-resume"}); err != nil {
		t.Fatal(err)
	}
	addFocus(service, 1200, *clock)
	due = service.Status()
	_, err = service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "long-after-short-start-2"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(5*time.Minute + time.Second)
	waiting = service.Status()
	if _, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "long-after-short-resume-2"}); err != nil {
		t.Fatal(err)
	}
	addFocus(service, 1200, *clock)
	longDue := service.Status()
	if longDue.Phase != LongBreakDue || longDue.FocusSegmentSeconds != 1200 {
		t.Fatalf("expected long break due after completed short rests: %+v", longDue)
	}
	skipped, err := service.Act(ActionRequest{Action: Skip, ExpectedRevision: longDue.Revision, RequestID: "long-after-short-skip"})
	if err != nil || skipped.RetryFocusAfterSeconds != 4200 {
		t.Fatalf("long skip retry must use long-focus accumulator: %+v err=%v", skipped, err)
	}
	addFocus(service, 1, clock.Add(time.Second))
	if status := service.Status(); status.Phase != Focusing {
		t.Fatalf("long break reappeared immediately after skip: %+v", status)
	}
	addFocus(service, 599, clock.Add(2*time.Second))
	if status := service.Status(); status.Phase != LongBreakDue {
		t.Fatalf("long break did not return after ten effective minutes: %+v", status)
	}
}

func TestSnoozeIsBoundedAndActionRequestIsExactlyOnce(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, _ := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	snoozed, err := service.Act(ActionRequest{Action: Snooze, ExpectedRevision: due.Revision, RequestID: "snooze-1"})
	if err != nil || snoozed.SnoozeCount != 1 || snoozed.SnoozeUntil == nil {
		t.Fatalf("snooze: %+v err=%v", snoozed, err)
	}
	if _, err := service.Act(ActionRequest{Action: Snooze, ExpectedRevision: snoozed.Revision, RequestID: "snooze-2"}); err != nil {
		t.Fatal(err)
	}
	second := service.Status()
	if _, err := service.Act(ActionRequest{Action: Snooze, ExpectedRevision: second.Revision, RequestID: "snooze-3"}); err == nil {
		t.Fatal("snooze exceeded configured max")
	}
	started, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: second.Revision, RequestID: "exactly-once-start"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "exactly-once-start"})
	if err != nil || replayed.Phase != started.Phase || modes.starts != 1 {
		t.Fatalf("request replay executed twice: replay=%+v starts=%d err=%v", replayed, modes.starts, err)
	}
	events, err := store.ListEyeCareAudit(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	starts := 0
	for _, event := range events {
		if event.EventType == "BREAK_STARTED" {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("BREAK_STARTED audit count=%d", starts)
	}
}

func TestRestorePreservesActiveBreakAndExpiredBreakWaitsForUser(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	if _, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "restore-start"}); err != nil {
		t.Fatal(err)
	}
	*clock = now.Add(time.Minute)
	restored, err := NewWithClock(service.Settings(), config.DefaultConfig().Reminder, store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Status(); got.Phase != ShortBreak || modes.status.UserMode != state.UserModeBreak {
		t.Fatalf("active break not restored: %+v", got)
	}
	*clock = now.Add(6 * time.Minute)
	waiting := restored.Status()
	if waiting.Phase != WaitingReturn || modes.status.UserMode != state.UserModeBreak {
		t.Fatalf("expired break should wait for user: %+v mode=%s", waiting, modes.status.UserMode)
	}
	if _, err := restored.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "restore-resume"}); err != nil {
		t.Fatal(err)
	}
}

func TestManualBreakIsNeverReportedAsEyeCareAndDateRollbackIsNonNegative(t *testing.T) {
	now := time.Date(2026, 9, 13, 23, 59, 0, 0, time.Local)
	service, _, modes, clock := newTestService(t, now)
	addFocus(service, 900, now)
	modes.status.UserMode = state.UserModeBreak
	modes.status.ModeOrigin = state.ModeOriginManual
	service.Observe(now.Add(time.Second), modes.status)
	if got := service.Status(); got.Phase != Focusing || got.CompletedShortBreaks != 0 {
		t.Fatalf("manual BREAK was misclassified: %+v", got)
	}
	*clock = now.Add(-time.Minute)
	addFocus(service, -10, *clock)
	if got := service.Status(); got.FocusSegmentSeconds < 0 || got.FocusSinceLongBreakSeconds < 0 {
		t.Fatalf("negative or time rollback accounting: %+v", got)
	}
}

func TestSettingsDefaultOffAndValidationRanges(t *testing.T) {
	cfg := config.DefaultConfig().EyeCare
	if cfg.Enabled || cfg.FocusMinutes != 40 || cfg.ShortBreakMinutes != 5 || cfg.LongBreakAfterFocusMinutes != 120 || cfg.LongBreakMinutes != 20 {
		t.Fatalf("bad opt-in defaults: %+v", cfg)
	}
	for _, mutate := range []func(*config.EyeCareConfig){
		func(v *config.EyeCareConfig) { v.FocusMinutes = 19 },
		func(v *config.EyeCareConfig) { v.ShortBreakMinutes = 21 },
		func(v *config.EyeCareConfig) { v.LongBreakAfterFocusMinutes = 241 },
		func(v *config.EyeCareConfig) { v.LongBreakMinutes = 61 },
		func(v *config.EyeCareConfig) { v.SnoozeMinutes = 31 },
		func(v *config.EyeCareConfig) { v.MaxSnoozes = 6 },
	} {
		invalid := cfg
		mutate(&invalid)
		if err := config.ValidateEyeCareConfig(invalid); err == nil {
			t.Fatalf("invalid config was accepted: %+v", invalid)
		}
	}
}

func TestEyeCareToastClaimsOnlyOnceAndSurvivesRestart(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	first, err := service.TakePendingNotification(now)
	if err != nil || first == nil || first.Kind != "SHORT_BREAK_DUE" {
		t.Fatalf("initial notification=%+v err=%v", first, err)
	}
	for i := 0; i < 100; i++ {
		if again, err := service.TakePendingNotification(now); err != nil || again != nil {
			t.Fatalf("duplicate notification at poll %d: %+v err=%v", i, again, err)
		}
	}
	restarted, err := NewWithClock(service.Settings(), config.DefaultConfig().Reminder, store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	if again, err := restarted.TakePendingNotification(now); err != nil || again != nil {
		t.Fatalf("restart repeated claimed notification: %+v err=%v", again, err)
	}
}

func TestQuietHoursAndSnoozeRetainDueUntilOneNotificationCanBeClaimed(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, modes, clock := newTestService(t, now)
	reminderConfig := config.DefaultConfig()
	reminderConfig.Reminder.QuietPeriods = nil
	reminderEngine := reminder.NewEngine(reminderConfig)
	service, err := NewWithReminderSettingsProviderAndClock(service.Settings(), reminderEngine, service.store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	reminderSettings := reminderEngine.GetSettings()
	reminderSettings.QuietPeriods = []config.QuietPeriodConfig{{Start: "10:00", End: "11:00"}}
	if err := reminderEngine.SetSettings(reminderSettings); err != nil {
		t.Fatal(err)
	}
	addFocus(service, 2400, now)
	if notification, err := service.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("quiet hours should suppress toast: %+v err=%v", notification, err)
	}
	if got := service.Status(); got.Phase != ShortBreakDue || got.DueGeneration != 1 {
		t.Fatalf("quiet period lost due state: %+v", got)
	}
	*clock = now.Add(61 * time.Minute)
	notification, err := service.TakePendingNotification(*clock)
	if err != nil || notification == nil || notification.Kind != "SHORT_BREAK_DUE" {
		t.Fatalf("quiet end notification=%+v err=%v", notification, err)
	}
	if again, err := service.TakePendingNotification(*clock); err != nil || again != nil {
		t.Fatalf("quiet-end duplicate=%+v err=%v", again, err)
	}

	now = time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, modes, clock = newTestService(t, now)
	service.cfg.SnoozeMinutes = 1
	reminderConfig = config.DefaultConfig()
	reminderEngine = reminder.NewEngine(reminderConfig)
	service, err = NewWithReminderSettingsProviderAndClock(service.Settings(), reminderEngine, service.store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	service.cfg.SnoozeMinutes = 1
	addFocus(service, 2400, now)
	due := service.Status()
	if _, err := service.Act(ActionRequest{Action: Snooze, ExpectedRevision: due.Revision, RequestID: "toast-snooze"}); err != nil {
		t.Fatal(err)
	}
	if notification, err := service.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("snooze should suppress notification: %+v err=%v", notification, err)
	}
	*clock = now.Add(time.Minute + time.Second)
	service.Status()
	notification, err = service.TakePendingNotification(*clock)
	if err != nil || notification == nil || notification.Kind != "SNOOZE_EXPIRED" {
		t.Fatalf("snooze expiry notification=%+v err=%v", notification, err)
	}
	if again, err := service.TakePendingNotification(*clock); err != nil || again != nil {
		t.Fatalf("snooze expiry duplicate=%+v err=%v", again, err)
	}
}

func TestQuietSettingsChangeImmediatelyWithoutRestartAndKeepNotificationUnclaimed(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 30, 0, 0, time.Local)
	cfg := config.DefaultConfig()
	cfg.EyeCare.Enabled = true
	cfg.Reminder.QuietPeriods = nil
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock := now
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	reminderEngine := reminder.NewEngine(cfg)
	service, err := NewWithReminderSettingsProviderAndClock(cfg.EyeCare, reminderEngine, store, modes, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 2400, now)
	due := service.Status()
	quiet := reminderEngine.GetSettings()
	quiet.QuietPeriods = []config.QuietPeriodConfig{{Start: "10:00", End: "11:00"}}
	if err := reminderEngine.SetSettings(quiet); err != nil {
		t.Fatal(err)
	}
	if notification, err := service.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("new quiet period did not suppress toast: notification=%+v err=%v", notification, err)
	}
	status := service.Status()
	if !status.NotificationSuppressed || status.NotifiedGeneration >= status.DueGeneration || status.DueGeneration != due.DueGeneration {
		t.Fatalf("quiet suppression consumed the due generation: before=%+v after=%+v", due, status)
	}
	quiet.QuietPeriods = nil
	if err := reminderEngine.SetSettings(quiet); err != nil {
		t.Fatal(err)
	}
	notification, err := service.TakePendingNotification(now)
	if err != nil || notification == nil || notification.Kind != "SHORT_BREAK_DUE" {
		t.Fatalf("removing quiet hours did not release pending toast: notification=%+v err=%v", notification, err)
	}
	if repeated, err := service.TakePendingNotification(now); err != nil || repeated != nil {
		t.Fatalf("claimed generation repeated after live update: notification=%+v err=%v", repeated, err)
	}
}

func TestDynamicCrossMidnightQuietPeriodSuppressesAndThenReleasesToast(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 30, 0, 0, time.Local)
	cfg := config.DefaultConfig()
	cfg.EyeCare.Enabled = true
	cfg.Reminder.QuietPeriods = nil
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	reminderEngine := reminder.NewEngine(cfg)
	service, err := NewWithReminderSettingsProviderAndClock(cfg.EyeCare, reminderEngine, store, modes, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 2400, now)
	quiet := reminderEngine.GetSettings()
	quiet.QuietPeriods = []config.QuietPeriodConfig{{Start: "22:00", End: "02:00"}}
	if err := reminderEngine.SetSettings(quiet); err != nil {
		t.Fatal(err)
	}
	if notification, err := service.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("cross-midnight quiet period did not suppress toast: notification=%+v err=%v", notification, err)
	}
	quiet.QuietPeriods = nil
	if err := reminderEngine.SetSettings(quiet); err != nil {
		t.Fatal(err)
	}
	if notification, err := service.TakePendingNotification(now); err != nil || notification == nil {
		t.Fatalf("cross-midnight notification was lost after quiet period removal: notification=%+v err=%v", notification, err)
	}
}

func TestInvalidReminderSettingsFailClosedWithoutClaimingDueGeneration(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 30, 0, 0, time.Local)
	cfg := config.DefaultConfig()
	cfg.EyeCare.Enabled = true
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	provider := staticReminderSettingsProvider{settings: config.ReminderConfig{
		QuietPeriods: []config.QuietPeriodConfig{{Start: "invalid", End: "11:00"}},
	}}
	service, err := NewWithReminderSettingsProviderAndClock(cfg.EyeCare, provider, store, modes, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 2400, now)
	if status := service.Status(); !status.NotificationSuppressed {
		t.Fatalf("invalid settings should be treated as quiet for safety: %+v", status)
	}
	if notification, err := service.TakePendingNotification(now); err == nil || notification != nil {
		t.Fatalf("invalid settings did not fail closed: notification=%+v err=%v", notification, err)
	}
	status := service.Status()
	if status.NotifiedGeneration >= status.DueGeneration {
		t.Fatalf("invalid settings consumed the notification generation: %+v", status)
	}
}

func TestReminderSettingsConcurrentReadsAndUpdatesAreSafe(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 30, 0, 0, time.Local)
	cfg := config.DefaultConfig()
	cfg.EyeCare.Enabled = true
	cfg.Reminder.QuietPeriods = nil
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	reminderEngine := reminder.NewEngine(cfg)
	service, err := NewWithReminderSettingsProviderAndClock(cfg.EyeCare, reminderEngine, store, modes, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 2400, now)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		for i := 0; i < 200; i++ {
			settings := reminderEngine.GetSettings()
			if i%2 == 0 {
				settings.QuietPeriods = []config.QuietPeriodConfig{{Start: "10:00", End: "11:00"}}
			} else {
				settings.QuietPeriods = nil
			}
			if err := reminderEngine.SetSettings(settings); err != nil {
				t.Errorf("update reminder settings: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < 200; i++ {
			_ = service.Status()
			if _, err := service.TakePendingNotification(now); err != nil {
				t.Errorf("read notification state: %v", err)
				return
			}
		}
	}()
	wait.Wait()
}

func TestLegacyWaitingReturnStudyIsResumedAndOffCancelsDue(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	started, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "legacy-resume-start"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = now.Add(6 * time.Minute)
	waiting := service.Status()
	if waiting.Phase != WaitingReturn {
		t.Fatalf("phase=%s", waiting.Phase)
	}
	modes.status.UserMode = state.UserModeStudy
	modes.status.ModeOrigin = state.ModeOriginManual
	if err := service.Observe(*clock, modes.status); err != nil {
		t.Fatal(err)
	}
	if got := service.Status(); got.Phase != Focusing {
		t.Fatalf("legacy study restore did not recover focusing: %+v", got)
	}
	events, err := store.ListEyeCareAudit(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if events[0].EventType != "RESUMED" || events[0].EventType == "ABORTED" {
		t.Fatalf("legacy resume audit=%+v (started=%+v)", events[0], started)
	}

	service, _, modes, _ = newTestService(t, now)
	addFocus(service, 2400, now)
	modes.status.UserMode = state.UserModeOff
	if err := service.Observe(now, modes.status); err != nil {
		t.Fatal(err)
	}
	if got := service.Status(); got.Phase != Focusing {
		t.Fatalf("OFF should clear due state: %+v", got)
	}
	if notification, err := service.TakePendingNotification(now); err != nil || notification != nil {
		t.Fatalf("OFF left a stale notification: %+v err=%v", notification, err)
	}
}

func TestRequestReplayReturnsOriginalResultAndRejectsActionConflict(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, _, _, _ := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	started, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "replay-original"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Act(ActionRequest{Action: FinishEarly, ExpectedRevision: started.Revision, RequestID: "finish-after-replay"}); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "replay-original"})
	if err != nil || replay.Phase != ShortBreak || replay.Revision != started.Revision {
		t.Fatalf("replay=%+v original=%+v err=%v", replay, started, err)
	}
	if _, err := service.Act(ActionRequest{Action: FinishEarly, ExpectedRevision: due.Revision, RequestID: "replay-original"}); err == nil {
		t.Fatal("same id with different action was accepted")
	}
}

func TestModeTransitionPersistenceFailureCompensatesCanonicalMode(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	service, err := NewWithClock(service.Settings(), config.DefaultConfig().Reminder,
		&failingEyeCareStore{Store: store, completeErr: errors.New("injected transaction failure")}, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 1, now.Add(time.Second))
	due = service.Status()
	_, err = service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "compensated-start"})
	if err == nil {
		t.Fatal("injected commit failure was hidden")
	}
	if modes.status.UserMode != state.UserModeStudy || modes.starts != 1 || modes.resumes != 1 {
		t.Fatalf("mode was not compensated: %+v", modes)
	}
	if got := service.Status(); got.Phase != ShortBreakDue {
		t.Fatalf("failed transition changed in-memory phase: %+v", got)
	}
}

func TestResumeCommitFailureCompensatesTheOriginalShortOrLongBreakAndAllowsRetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		focus       int64
		action      Action
		duration    time.Duration
		pauseReason state.PauseReason
	}{
		{name: "short", focus: 2400, action: StartShortBreak, duration: 5*time.Minute + time.Second, pauseReason: state.PauseReasonEyeCareShort},
		{name: "long", focus: 7200, action: StartLongBreak, duration: 20*time.Minute + time.Second, pauseReason: state.PauseReasonEyeCareLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
			base, store, modes, clock := newTestService(t, now)
			failingStore := &failingEyeCareStore{Store: store}
			service, err := NewWithClock(base.Settings(), config.DefaultConfig().Reminder, failingStore, modes, func() time.Time { return *clock })
			if err != nil {
				t.Fatal(err)
			}
			addFocus(service, tc.focus, now)
			due := service.Status()
			started, err := service.Act(ActionRequest{Action: tc.action, ExpectedRevision: due.Revision, RequestID: "start-" + tc.name})
			if err != nil {
				t.Fatal(err)
			}
			*clock = now.Add(tc.duration)
			waiting := service.Status()
			if waiting.Phase != WaitingReturn || modes.status.PauseReason != tc.pauseReason {
				t.Fatalf("did not reach expected waiting break: status=%+v mode=%+v", waiting, modes.status)
			}
			failingStore.completeErr = errors.New("injected resume transaction failure")
			failedRequestID := "resume-failed-" + tc.name
			_, err = service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: failedRequestID})
			if err == nil {
				t.Fatal("resume transaction failure was hidden")
			}
			if modes.status.UserMode != state.UserModeBreak || modes.status.ModeOrigin != state.ModeOriginEyeCare || modes.status.PauseReason != tc.pauseReason {
				t.Fatalf("compensation changed break type: mode=%+v", modes.status)
			}
			if got := service.Status(); got.Phase != WaitingReturn {
				t.Fatalf("failed resume changed canonical eye-care phase: %+v", got)
			}
			ledger, found, err := store.GetEyeCareRequest(context.Background(), failedRequestID)
			if err != nil || !found || ledger.Status != "FAILED" || ledger.ErrorKind != "storage_unavailable" {
				t.Fatalf("failed request did not reach a deterministic terminal result: %+v found=%v err=%v", ledger, found, err)
			}
			starts, resumes := modes.starts, modes.resumes
			if _, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "resume-retry-" + tc.name}); err != nil {
				t.Fatalf("new request ID could not retry resume: %v", err)
			}
			if modes.status.UserMode != state.UserModeStudy || service.Status().Phase != Focusing || modes.starts != starts || modes.resumes != resumes+1 {
				t.Fatalf("retry did not complete exactly one resume: mode=%+v status=%+v starts=%d resumes=%d", modes.status, service.Status(), modes.starts, modes.resumes)
			}
			if _, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: failedRequestID}); err == nil {
				t.Fatal("failed request replay was reported as success")
			}
			if modes.starts != starts || modes.resumes != resumes+1 {
				t.Fatalf("replayed failed request repeated a mode transition: %+v", modes)
			}
			if started.Phase != map[Action]Phase{StartShortBreak: ShortBreak, StartLongBreak: LongBreak}[tc.action] {
				t.Fatalf("unexpected initial break phase: %+v", started)
			}
		})
	}
}

func TestResumeCompensationFailureLeavesPendingRequestForCanonicalRestartReconciliation(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	base, store, modes, clock := newTestService(t, now)
	failingStore := &failingEyeCareStore{Store: store}
	service, err := NewWithClock(base.Settings(), config.DefaultConfig().Reminder, failingStore, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	addFocus(service, 7200, now)
	due := service.Status()
	started, err := service.Act(ActionRequest{Action: StartLongBreak, ExpectedRevision: due.Revision, RequestID: "restart-long-start"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = now.Add(20*time.Minute + time.Second)
	waiting := service.Status()
	if waiting.Phase != WaitingReturn || started.Phase != LongBreak {
		t.Fatalf("test precondition failed: started=%+v waiting=%+v", started, waiting)
	}
	failingStore.completeErr = errors.New("injected resume transaction failure")
	modes.startError = errors.New("injected compensation failure")
	const requestID = "restart-long-resume-pending"
	_, err = service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: requestID})
	if err == nil || ErrorKind(err) != "reconciliation_required" {
		t.Fatalf("compensation failure was not explicit: err=%v kind=%s", err, ErrorKind(err))
	}
	ledger, found, err := store.GetEyeCareRequest(context.Background(), requestID)
	if err != nil || !found || ledger.Status != "PENDING" {
		t.Fatalf("ambiguous transition was not retained for reconciliation: %+v found=%v err=%v", ledger, found, err)
	}
	if modes.status.UserMode != state.UserModeStudy {
		t.Fatalf("test expected resume to commit before compensation failure: %+v", modes.status)
	}
	modes.startError = nil
	restarted, err := NewWithClock(base.Settings(), config.DefaultConfig().Reminder, store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Status(); got.Phase != Focusing || modes.status.UserMode != state.UserModeStudy {
		t.Fatalf("restart did not reconcile against canonical study mode: status=%+v mode=%+v", got, modes.status)
	}
	ledger, found, err = store.GetEyeCareRequest(context.Background(), requestID)
	if err != nil || !found || ledger.Status != "APPLIED" {
		t.Fatalf("pending request not deterministically completed after restart: %+v found=%v err=%v", ledger, found, err)
	}
}

func TestPersistenceFailureIsVisibleAndFocusCheckpointRetriesCreditedSeconds(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	base, store, modes, clock := newTestService(t, now)
	failing := &failingEyeCareStore{Store: store, stateErr: errors.New("injected checkpoint failure")}
	service, err := NewWithClock(base.Settings(), config.DefaultConfig().Reminder, failing, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	outcome := state.TickOutcome{Now: now, UserMode: state.UserModeStudy, ActivityValid: true}
	if err := service.RecordCreditedFocus(100, outcome, modes.status); err == nil {
		t.Fatal("checkpoint failure was hidden")
	}
	if got := service.Status(); !got.StorageDegraded {
		t.Fatalf("storage failure not visible: %+v", got)
	}
	outcome.Now = now.Add(2 * time.Second)
	if err := service.RecordCreditedFocus(100, outcome, modes.status); err != nil {
		t.Fatal(err)
	}
	if got := service.Status(); got.FocusSegmentSeconds != 200 || got.StorageDegraded {
		t.Fatalf("retry lost credited checkpoint or retained false degradation: %+v", got)
	}
}

func TestSettingsAndRuntimeStateRemainUnchangedWhenAtomicSaveFails(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	store, err := storage.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock := now
	modes := &testModes{status: state.SystemStatus{UserMode: state.UserModeStudy, ModeOrigin: state.ModeOriginManual}}
	disabled := config.DefaultConfig().EyeCare
	base, err := NewWithClock(disabled, config.DefaultConfig().Reminder, store, modes, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewWithClock(disabled, config.DefaultConfig().Reminder, &failingEyeCareStore{
		Store: store, settingsErr: errors.New("injected settings transaction failure"),
	}, modes, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	enabled := base.Settings()
	enabled.Enabled = true
	if _, err := service.SaveSettings(enabled); err == nil {
		t.Fatal("atomic settings failure was hidden")
	}
	if service.Settings().Enabled || service.Status().Phase != Disabled || !service.Status().StorageDegraded {
		t.Fatalf("failed settings transaction partially changed canonical state: settings=%+v status=%+v", service.Settings(), service.Status())
	}
}

func TestPendingStartIsReconciledFromCanonicalModeAfterRestart(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	service, store, modes, clock := newTestService(t, now)
	addFocus(service, 2400, now)
	due := service.Status()
	planned := cloneStatus(due)
	planned.Phase = ShortBreak
	planned.BreakStartedAt = timePointer(now)
	planned.PlannedBreakEndAt = timePointer(now.Add(5 * time.Minute))
	planned.Revision++
	encoded, _ := json.Marshal(planned)
	if _, _, err := store.BeginEyeCareRequest(context.Background(), storage.EyeCareRequestRecord{
		RequestID: "restart-pending", Action: string(StartShortBreak), ResultJSON: string(encoded),
		ResultRevision: planned.Revision, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	modes.status.UserMode = state.UserModeBreak
	modes.status.ModeOrigin = state.ModeOriginEyeCare
	restarted, err := NewWithClock(service.Settings(), config.DefaultConfig().Reminder, store, modes, func() time.Time { return *clock })
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Status(); got.Phase != ShortBreak {
		t.Fatalf("pending action did not reconcile to canonical break: %+v", got)
	}
	request, found, err := store.GetEyeCareRequest(context.Background(), "restart-pending")
	if err != nil || !found || request.Status != "APPLIED" {
		t.Fatalf("request=%+v found=%v err=%v", request, found, err)
	}
}

func TestShortCycleUsesOnlyTemporarySQLiteAndFakeClock(t *testing.T) {
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.Local)
	dbPath := filepath.Join(t.TempDir(), "isolated-eye-care.db")
	store, err := storage.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clock := state.NewFakeClock(now)
	cfg := config.DefaultConfig()
	cfg.EyeCare.Enabled = true
	modes := state.NewPersistentManager(clock, cfg, store, cycleRule{}, cyclePrivacy{}, cycleReminder{})
	if err := modes.SetModeStudy("Go"); err != nil {
		t.Fatal(err)
	}
	service, err := NewWithClock(cfg.EyeCare, cfg.Reminder, store, modes, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	// Shorten only this isolated Service instance; production validation and
	// production configuration remain unchanged.
	service.cfg.FocusMinutes = 1
	service.cfg.ShortBreakMinutes = 1
	service.cfg.LongBreakAfterFocusMinutes = 2
	service.cfg.LongBreakMinutes = 1
	credit := func(seconds int64) error {
		current := clock.Now()
		return service.RecordCreditedFocus(seconds, state.TickOutcome{
			Now: current, UserMode: state.UserModeStudy, ActivityValid: true,
			Interaction: state.InteractionActive, Relation: state.RelationFocused,
		}, modes.GetStatus())
	}
	if err := credit(60); err != nil {
		t.Fatal(err)
	}
	due := service.Status()
	if due.Phase != ShortBreakDue || due.DueGeneration != 1 {
		t.Fatalf("isolated 60-second short cycle did not become due: %+v", due)
	}
	if notice, err := service.TakePendingNotification(clock.Now()); err != nil || notice == nil || notice.Message != "已有效专注 1 分钟，起来走动一下，看看远处吧。" {
		t.Fatalf("isolated short-cycle notice=%+v err=%v", notice, err)
	}
	started, err := service.Act(ActionRequest{Action: StartShortBreak, ExpectedRevision: due.Revision, RequestID: "short-cycle-start"})
	if err != nil || started.Phase != ShortBreak || modes.GetStatus().ModeOrigin != state.ModeOriginEyeCare {
		t.Fatalf("short rest start=%+v mode=%+v err=%v", started, modes.GetStatus(), err)
	}
	setTestBreakEnd(t, service, clock.Now().Add(15*time.Second))
	clock.Advance(16 * time.Second)
	waiting := service.Status()
	if waiting.Phase != WaitingReturn || modes.GetStatus().UserMode != state.UserModeBreak {
		t.Fatalf("short rest did not wait after isolated 15 seconds: %+v mode=%s", waiting, modes.GetStatus().UserMode)
	}
	if _, err := service.Act(ActionRequest{Action: ResumeStudy, ExpectedRevision: waiting.Revision, RequestID: "short-cycle-resume"}); err != nil {
		t.Fatal(err)
	}
	if err := credit(60); err != nil {
		t.Fatal(err)
	}
	longDue := service.Status()
	if longDue.Phase != LongBreakDue || longDue.FocusSinceLongBreakSeconds != 120 {
		t.Fatalf("isolated 120-second long cycle did not become due: %+v", longDue)
	}
	if notice, err := service.TakePendingNotification(clock.Now()); err != nil || notice == nil || notice.Message != "已累计有效专注 2 分钟，建议安排一次 1 分钟完整休息。" {
		t.Fatalf("isolated long-cycle notice=%+v err=%v", notice, err)
	}
	long, err := service.Act(ActionRequest{Action: StartLongBreak, ExpectedRevision: longDue.Revision, RequestID: "long-cycle-start"})
	if err != nil || long.Phase != LongBreak {
		t.Fatalf("long rest start=%+v err=%v", long, err)
	}
	setTestBreakEnd(t, service, clock.Now().Add(30*time.Second))
	clock.Advance(31 * time.Second)
	if got := service.Status(); got.Phase != WaitingReturn || modes.GetStatus().UserMode != state.UserModeBreak {
		t.Fatalf("long rest did not wait after isolated 30 seconds: %+v mode=%s", got, modes.GetStatus().UserMode)
	}
	loaded, found, err := store.LoadEyeCareState(context.Background())
	if err != nil || !found || loaded.Phase != string(WaitingReturn) || loaded.Revision <= long.Revision {
		t.Fatalf("temporary SQLite did not retain canonical final state: %+v found=%v err=%v", loaded, found, err)
	}
}

func setTestBreakEnd(t *testing.T, service *Service, end time.Time) {
	t.Helper()
	service.mu.Lock()
	defer service.mu.Unlock()
	next := cloneStatus(service.status)
	next.PlannedBreakEndAt = timePointer(end)
	next.UpdatedAt = end.Add(-time.Second)
	if err := service.persistCandidateLocked(next, next.UpdatedAt, "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	service.status = next
}
