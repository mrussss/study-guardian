package eyecare

import (
	"context"
	"testing"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type testModes struct {
	status     state.SystemStatus
	starts     int
	resumes    int
	startError error
}

func (m *testModes) SetModeEyeCareBreak(long bool) error {
	if m.startError != nil {
		return m.startError
	}
	m.starts++
	m.status.UserMode = state.UserModeBreak
	m.status.ModeOrigin = state.ModeOriginEyeCare
	if long {
		m.status.PauseReason = state.PauseReasonEyeCareLong
	} else {
		m.status.PauseReason = state.PauseReasonEyeCareShort
	}
	return nil
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
