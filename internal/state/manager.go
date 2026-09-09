package state

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

var sessionSequence atomic.Uint64

const maxTickGap = 30 * time.Second

func newSessionID(now time.Time) string {
	return fmt.Sprintf("sess-%d-%d", now.UnixNano(), sessionSequence.Add(1))
}

type RuleClassifier interface {
	Classify(app, title, domain, task string) ClassificationResult
}

type PrivacyEvaluator interface {
	Evaluate(app, title, domain string) PrivacyState
}

type ReminderEvaluator interface {
	Evaluate(input ReminderDecisionInput) *ReminderEvent
}

type Manager struct {
	mu          sync.RWMutex
	clock       Clock
	cfg         *config.Config
	storage     *storage.Storage
	ruleEngine  RuleClassifier
	privacyGate PrivacyEvaluator
	reminderEng ReminderEvaluator

	currentDate         string
	userMode            UserMode
	task                string
	currentSessID       string
	modeOrigin          ModeOrigin
	pauseReason         PauseReason
	autoResumeEligible  bool
	manualOverrideUntil *time.Time

	interaction InteractionState
	relation    TaskRelation
	privacy     PrivacyState
	confidence  float64

	studySeconds       int64
	breakSeconds       int64
	standbySeconds     int64
	offSeconds         int64
	activeSeconds      int64
	distractedSeconds  int64
	idleStaticSeconds  int64
	currentModeSeconds int64

	modeStartTime  time.Time
	lastTickTime   time.Time
	lastActivityAt *time.Time

	activityWatchOK                  bool
	activityWatchLastSuccessAt       *time.Time
	activityWatchConsecutiveFailures int
	activityWatchStableOK            bool
	activityWatchHealthPhase         ActivityWatchHealthPhase
	screenSensorOK                   bool
	currentReminder                  *ReminderEvent
	reminderRecoverySince            time.Time
	pendingAutomationIntent          *AutomationIntent
	autoPauseSnoozeUntil             *time.Time
	feedbacks                        []FeedbackRecord

	toastNotifier func(title, msg string) error
}

func NewPersistentManager(
	clock Clock,
	cfg *config.Config,
	store *storage.Storage,
	ruleEngine RuleClassifier,
	privacyGate PrivacyEvaluator,
	reminderEng ReminderEvaluator,
) *Manager {
	if clock == nil {
		clock = RealClock{}
	}
	now := clock.Now()
	dateStr := now.Format("2006-01-02")

	m := &Manager{
		clock:                    clock,
		cfg:                      cfg,
		storage:                  store,
		ruleEngine:               ruleEngine,
		privacyGate:              privacyGate,
		reminderEng:              reminderEng,
		currentDate:              dateStr,
		userMode:                 UserModeStandby,
		interaction:              InteractionUnknown,
		relation:                 RelationUnknown,
		privacy:                  PrivacyNormal,
		confidence:               1.0,
		modeOrigin:               ModeOriginManual,
		pauseReason:              PauseReasonNone,
		modeStartTime:            now,
		lastTickTime:             now,
		lastActivityAt:           &now,
		activityWatchOK:          true,
		activityWatchStableOK:    true,
		activityWatchHealthPhase: ActivityWatchAvailable,
		screenSensorOK:           true,
	}

	// 1. Load Daily State
	if store != nil {
		ctx := context.Background()
		if stand, std, brk, off, act, err := store.LoadDailyState(ctx, dateStr); err == nil {
			m.standbySeconds = stand
			m.studySeconds = std
			m.breakSeconds = brk
			m.offSeconds = off
			m.activeSeconds = act
		}

		// 2. Recover only an interrupted open session. A completed session must
		// never change the user's mode after a restart.
		openSess, openErr := store.LoadOpenSession(ctx)
		if openErr == nil {
			if storage.LocalDate(openSess.StartedAt) == dateStr {
				switch UserMode(openSess.Mode) {
				case UserModeStandby, UserModeStudy, UserModeBreak, UserModeOff:
					m.userMode = UserMode(openSess.Mode)
					m.task = openSess.Task
					// The interrupted row owns the last persisted duration. A
					// new post-restart session starts at zero; inheriting this
					// value would double-count every restart in review aggregates.
					m.currentModeSeconds = 0
					if openSess.ModeOrigin != "" {
						m.modeOrigin = ModeOrigin(openSess.ModeOrigin)
					}
					if openSess.PauseReason != "" {
						m.pauseReason = PauseReason(openSess.PauseReason)
					}
					m.autoResumeEligible = openSess.AutoResumeEligible
				}
			}
			// Close the interrupted record using its last persisted duration. Do
			// not derive duration from wall-clock time, which includes downtime,
			// sleep and lock-screen time.
		}
		_ = store.CloseOpenSessions(ctx, now, "RESTART_RECOVERY")
		m.modeStartTime = now
		m.currentSessID = newSessionID(now)
		_ = store.SaveSession(ctx, storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(m.userMode),
			Task:               m.task,
			StartedAt:          now,
			DurationSeconds:    0,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		})
	} else {
		m.currentSessID = newSessionID(now)
	}

	return m
}

func NewManager(clock Clock) *Manager {
	return NewPersistentManager(clock, config.DefaultConfig(), nil, nil, nil, nil)
}

// Close persists the current session as cleanly ended. Interrupted sessions
// are handled by the constructor on the next start.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.storage != nil && m.currentSessID != "" {
		now := m.clock.Now()
		m.closeCurrentSessionLocked(now, "SHUTDOWN")
	}
}

func (m *Manager) SetToastNotifier(fn func(title, msg string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toastNotifier = fn
}

const reminderRecoveryStableFor = 20 * time.Second

func (m *Manager) clearCurrentReminderLocked() {
	if m.currentReminder != nil && m.storage != nil {
		_ = m.storage.MarkReminderInactive(context.Background(), m.currentReminder.ID)
	}
	m.currentReminder = nil
	m.reminderRecoverySince = time.Time{}
}

func (m *Manager) clearExpiredReminderLocked(now time.Time) {
	if m.currentReminder == nil {
		return
	}
	if !m.currentReminder.ExpiresAt.IsZero() && !now.Before(m.currentReminder.ExpiresAt) {
		m.clearCurrentReminderLocked()
	}
}

func (m *Manager) clearRecoveredReminderLocked(now time.Time) {
	m.clearExpiredReminderLocked(now)
	if m.currentReminder == nil {
		return
	}
	recovered := m.userMode == UserModeStudy && m.activityWatchOK && m.privacy == PrivacyNormal && m.interaction == InteractionActive && m.relation == RelationFocused && m.confidence >= 0.6
	if !recovered {
		m.reminderRecoverySince = time.Time{}
		return
	}
	if m.reminderRecoverySince.IsZero() {
		m.reminderRecoverySince = now
		return
	}
	if now.Sub(m.reminderRecoverySince) >= reminderRecoveryStableFor {
		m.clearCurrentReminderLocked()
	}
}

func (m *Manager) GetStatus() SystemStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.clearExpiredReminderLocked(now)
	pending := cloneAutomationIntent(m.pendingAutomationIntent)
	return SystemStatus{
		UserMode:                         m.userMode,
		InteractionState:                 m.interaction,
		TaskRelation:                     m.relation,
		PrivacyState:                     m.privacy,
		Confidence:                       m.confidence,
		Task:                             m.task,
		StudySeconds:                     m.studySeconds,
		BreakSeconds:                     m.breakSeconds,
		ActiveSeconds:                    m.activeSeconds,
		LastActivityAt:                   m.lastActivityAt,
		ActivityWatchOK:                  m.activityWatchOK,
		ActivityWatchLastSuccessAt:       m.activityWatchLastSuccessAt,
		ActivityWatchConsecutiveFailures: m.activityWatchConsecutiveFailures,
		ActivityWatchStableOK:            m.activityWatchStableOK,
		ActivityWatchHealthPhase:         m.activityWatchHealthPhase,
		ScreenSensorOK:                   m.screenSensorOK,
		CurrentReminder:                  m.currentReminder,
		ModeOrigin:                       m.modeOrigin,
		PauseReason:                      m.pauseReason,
		AutoResumeEligible:               m.autoResumeEligible,
		ManualOverrideUntil:              m.manualOverrideUntil,
		AutoPauseSnoozeUntil:             m.autoPauseSnoozeUntil,
		PendingAutomationIntent:          pending,
	}
}

func (m *Manager) GetCurrentTask() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.task
}

func (m *Manager) SetModeStudy(task string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	m.checkMidnightResetLocked(now)

	m.closeCurrentSessionLocked(now, "USER_SWITCH_STUDY")

	m.userMode = UserModeStudy
	m.modeOrigin = ModeOriginManual
	m.pauseReason = PauseReasonNone
	m.autoResumeEligible = false
	m.setManualOverrideLocked(now)
	if task != "" {
		m.task = task
	}
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.clearCurrentReminderLocked()
	m.pendingAutomationIntent = nil
	m.autoPauseSnoozeUntil = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(UserModeStudy),
			Task:               m.task,
			StartedAt:          now,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		})
	}
	return nil
}

func (m *Manager) SetModeBreak() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.userMode != UserModeStudy && m.userMode != UserModeStandby {
		return errors.New("cannot enter BREAK from current mode")
	}

	now := m.clock.Now()
	m.checkMidnightResetLocked(now)

	m.closeCurrentSessionLocked(now, "USER_SWITCH_BREAK")

	m.userMode = UserModeBreak
	m.modeOrigin = ModeOriginManual
	m.pauseReason = PauseReasonNone
	m.autoResumeEligible = false
	m.setManualOverrideLocked(now)
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.clearCurrentReminderLocked()
	m.pendingAutomationIntent = nil
	m.autoPauseSnoozeUntil = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(UserModeBreak),
			Task:               m.task,
			StartedAt:          now,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		})
	}
	return nil
}

func (m *Manager) SetModeOff() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	m.checkMidnightResetLocked(now)

	m.closeCurrentSessionLocked(now, "USER_SWITCH_OFF")

	m.userMode = UserModeOff
	m.modeOrigin = ModeOriginManual
	m.pauseReason = PauseReasonNone
	m.autoResumeEligible = false
	m.setManualOverrideLocked(now)
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.clearCurrentReminderLocked()
	m.pendingAutomationIntent = nil
	m.autoPauseSnoozeUntil = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(UserModeOff),
			Task:               m.task,
			StartedAt:          now,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		})
	}
	return nil
}

func (m *Manager) setManualOverrideLocked(now time.Time) {
	minutes := 30
	if m.cfg != nil {
		minutes = m.cfg.Automation.ManualOverrideMinutes
	}
	if minutes <= 0 {
		m.manualOverrideUntil = nil
		return
	}
	deadline := now.Add(time.Duration(minutes) * time.Minute)
	m.manualOverrideUntil = &deadline
}

// ApplyAutomationIntent is the only path by which the independent automation
// controller changes mode. It runs after TickWithClassification has released
// the manager mutex, so classification and time accounting never recurse into
// a mode transition while the manager is locked.
func cloneAutomationIntent(intent *AutomationIntent) *AutomationIntent {
	if intent == nil {
		return nil
	}
	copy := *intent
	return &copy
}

type automationIntentResolution uint8

const (
	automationIntentNotExpired automationIntentResolution = iota
	automationIntentApply
	automationIntentDismiss
)

func defaultAutomationExpiryAction(intent AutomationIntent) AutomationExpiryAction {
	if intent.ExpiryAction != "" {
		return intent.ExpiryAction
	}
	if intent.Transition == AutomationPause || intent.Transition == AutomationResume {
		return AutomationExpiryApply
	}
	return AutomationExpiryDismiss
}

// resolveExpiredAutomationIntentLocked is the single state transition for an
// expired confirmation. It claims the intent while holding the manager lock;
// callers must perform the actual mode transition after unlocking.
func (m *Manager) resolveExpiredAutomationIntentLocked(now time.Time) (*AutomationIntent, automationIntentResolution) {
	if now.IsZero() {
		now = m.clock.Now()
	}
	pending := m.pendingAutomationIntent
	if pending == nil || pending.ExpiresAt.IsZero() || now.Before(pending.ExpiresAt) {
		return nil, automationIntentNotExpired
	}
	intent := *pending
	m.pendingAutomationIntent = nil
	intent.RequiresConfirmation = false
	if defaultAutomationExpiryAction(intent) == AutomationExpiryApply {
		return &intent, automationIntentApply
	}
	return &intent, automationIntentDismiss
}

func (m *Manager) PendingAutomationIntent() *AutomationIntent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneAutomationIntent(m.pendingAutomationIntent)
}

func (m *Manager) AcceptAutomationIntent(id string) error {
	m.mu.Lock()
	now := m.clock.Now()
	if m.pendingAutomationIntent == nil || (id != "" && m.pendingAutomationIntent.ID != id) {
		m.mu.Unlock()
		return errors.New("automation intent is not pending")
	}
	if expired, resolution := m.resolveExpiredAutomationIntentLocked(now); resolution != automationIntentNotExpired {
		m.mu.Unlock()
		if resolution == automationIntentApply && expired != nil {
			return m.applyAutomationIntentNow(*expired)
		}
		return nil
	}
	intent := *m.pendingAutomationIntent
	m.pendingAutomationIntent = nil
	intent.RequiresConfirmation = false
	m.mu.Unlock()
	return m.applyAutomationIntentNow(intent)
}

func (m *Manager) RejectAutomationIntent(id string) error {
	m.mu.Lock()
	now := m.clock.Now()
	if m.pendingAutomationIntent == nil || (id != "" && m.pendingAutomationIntent.ID != id) {
		m.mu.Unlock()
		return errors.New("automation intent is not pending")
	}
	if expired, resolution := m.resolveExpiredAutomationIntentLocked(now); resolution != automationIntentNotExpired {
		m.mu.Unlock()
		if resolution == automationIntentApply && expired != nil {
			return m.applyAutomationIntentNow(*expired)
		}
		return nil
	}
	intent := *m.pendingAutomationIntent
	m.pendingAutomationIntent = nil
	if intent.Transition == AutomationPause {
		deadline := now.Add(4 * time.Minute)
		m.autoPauseSnoozeUntil = &deadline
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) ProcessExpiredAutomationIntent(now time.Time) error {
	m.mu.Lock()
	intent, resolution := m.resolveExpiredAutomationIntentLocked(now)
	m.mu.Unlock()

	if resolution != automationIntentApply || intent == nil {
		return nil
	}
	return m.applyAutomationIntentNow(*intent)
}

// ApplyAutomationIntent either applies an intent immediately or records a
// short-lived pending intent when the configured UI confirmation is required.
func (m *Manager) ApplyAutomationIntent(intent AutomationIntent) error {
	if intent.RequiresConfirmation {
		m.mu.Lock()
		now := m.clock.Now()
		if expired, resolution := m.resolveExpiredAutomationIntentLocked(now); resolution != automationIntentNotExpired {
			m.mu.Unlock()
			if resolution == automationIntentApply && expired != nil {
				return m.applyAutomationIntentNow(*expired)
			}
			return m.ApplyAutomationIntent(intent)
		}
		if m.pendingAutomationIntent != nil {
			m.mu.Unlock()
			return nil
		}
		if intent.ID == "" {
			intent.ID = fmt.Sprintf("intent-%d", now.UnixNano())
		}
		if intent.CreatedAt.IsZero() {
			intent.CreatedAt = now
		}
		if intent.ExpiresAt.IsZero() {
			intent.ExpiresAt = now.Add(15 * time.Second)
		}
		if intent.ExpiryAction == "" {
			if intent.Transition == AutomationPause || intent.Transition == AutomationResume {
				intent.ExpiryAction = AutomationExpiryApply
			} else {
				intent.ExpiryAction = AutomationExpiryDismiss
			}
		}
		m.pendingAutomationIntent = &intent
		notifier := m.toastNotifier
		m.mu.Unlock()
		if notifier != nil {
			message := "检测到持续学习，是否开始计时？"
			if intent.Transition == AutomationPause {
				message = "检测到你可能已离开，是否暂停计时？"
			}
			_ = notifier("StudyGuardian", message)
		}
		return nil
	}
	return m.applyAutomationIntentNow(intent)
}

func (m *Manager) applyAutomationIntentNow(intent AutomationIntent) error {
	m.mu.Lock()
	now := m.clock.Now()
	if intent.Transition == AutomationStart && m.manualOverrideUntil != nil && now.Before(*m.manualOverrideUntil) {
		m.mu.Unlock()
		return errors.New("manual override is active for automatic start")
	}
	m.checkMidnightResetLocked(now)
	notice := ""
	switch intent.Transition {
	case AutomationStart:
		if m.userMode != UserModeStandby {
			m.mu.Unlock()
			return errors.New("automatic start requires STANDBY")
		}
		if strings.TrimSpace(intent.Task) == "" {
			m.mu.Unlock()
			return errors.New("automatic start requires a task")
		}
		m.closeCurrentSessionLocked(now, "AUTOMATION_START")
		m.userMode = UserModeStudy
		m.task = strings.Join(strings.Fields(intent.Task), " ")
		m.modeOrigin, m.pauseReason, m.autoResumeEligible = ModeOriginAutomation, PauseReasonNone, false
		notice = "检测到持续学习，已开始计时"
	case AutomationPause:
		if m.userMode != UserModeStudy {
			m.mu.Unlock()
			return errors.New("automatic pause requires STUDY")
		}
		m.closeCurrentSessionLocked(now, "AUTOMATION_PAUSE")
		m.userMode = UserModeBreak
		m.modeOrigin, m.pauseReason, m.autoResumeEligible = ModeOriginAutomation, intent.Reason, true
		m.manualOverrideUntil = nil
		m.autoPauseSnoozeUntil = nil
		notice = "已离开学习，计时已自动暂停。"
	case AutomationResume:
		if m.userMode != UserModeBreak || m.modeOrigin != ModeOriginAutomation || !m.autoResumeEligible {
			m.mu.Unlock()
			return errors.New("automatic resume is not eligible")
		}
		m.closeCurrentSessionLocked(now, "AUTOMATION_RESUME")
		m.userMode = UserModeStudy
		m.modeOrigin, m.pauseReason, m.autoResumeEligible = ModeOriginAutomation, PauseReasonNone, false
		notice = "检测到恢复学习，已继续计时"
	default:
		m.mu.Unlock()
		return errors.New("unknown automation transition")
	}
	m.modeStartTime = now
	m.currentModeSeconds = 0
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.clearCurrentReminderLocked()
	m.currentSessID = newSessionID(now)
	var saveErr error
	if m.storage != nil {
		saveErr = m.storage.SaveSession(context.Background(), storage.SessionRecord{ID: m.currentSessID, Mode: string(m.userMode), Task: m.task, StartedAt: now, ModeOrigin: string(m.modeOrigin), PauseReason: string(m.pauseReason), AutoResumeEligible: m.autoResumeEligible})
	}
	notifier := m.toastNotifier
	task := strings.TrimSpace(m.task)
	m.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}
	if notifier != nil {
		if task != "" && intent.Transition == AutomationStart {
			notice += "：" + task
		}
		_ = notifier("StudyGuardian", notice)
	}
	return nil
}

func (m *Manager) SetTask(task string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task = strings.Join(strings.Fields(task), " ")
	if m.storage != nil && m.currentSessID != "" {
		if err := m.storage.UpdateOpenSessionTask(context.Background(), m.currentSessID, task); err != nil {
			return fmt.Errorf("persist current task: %w", err)
		}
	}
	m.task = task
	return nil
}

func (m *Manager) RecordFeedback(eventID, feedback string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.feedbacks = append(m.feedbacks, FeedbackRecord{
		EventID:   eventID,
		Feedback:  feedback,
		CreatedAt: now,
	})
	if m.storage != nil {
		_ = m.storage.RecordFeedback(context.Background(), eventID, feedback, now)
	}
	return nil
}

func (m *Manager) SetHealth(awOK, sensorOK bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activityWatchOK = awOK
	m.activityWatchStableOK = awOK
	if awOK {
		m.activityWatchHealthPhase = ActivityWatchAvailable
	} else {
		m.activityWatchHealthPhase = ActivityWatchUnavailable
	}
	m.screenSensorOK = sensorOK
}

func (m *Manager) SetActivityWatchHealth(diagnostics ActivityWatchDiagnostics, sensorOK bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activityWatchOK = diagnostics.StableOK
	m.activityWatchStableOK = diagnostics.StableOK
	m.activityWatchLastSuccessAt = cloneTimePtr(diagnostics.LastSuccessAt)
	m.activityWatchConsecutiveFailures = diagnostics.ConsecutiveFailures
	m.activityWatchHealthPhase = diagnostics.Phase
	m.screenSensorOK = sensorOK
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.Round(0)
	return &copy
}

func (m *Manager) UpdateObservation(obs Observation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interaction = obs.Interaction
	m.relation = obs.Relation
	m.privacy = obs.Privacy
	m.confidence = obs.Confidence
}

func (m *Manager) Tick(now time.Time, app, title, domain string, isAFK bool, screenChanged bool, isLocked bool) TickOutcome {
	var classification ClassificationResult
	if m.ruleEngine != nil {
		classification = m.ruleEngine.Classify(app, title, domain, m.task)
	} else {
		classification = ClassificationResult{Relation: RelationUnknown, Confidence: 0.5}
	}
	if classification.SourceKind == "" {
		classification.SourceKind = SourceKindLocalRule
	}
	classification.IsFromRule = true
	return m.TickWithClassification(now, app, title, domain, isAFK, screenChanged, isLocked, classification)
}

func (m *Manager) TickWithClassification(
	now time.Time,
	app, title, domain string,
	isAFK bool,
	screenChanged bool,
	isLocked bool,
	classification ClassificationResult,
) TickOutcome {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Check midnight crossing
	m.checkMidnightResetLocked(now)

	// 2. Compute elapsed time (with sleep/hibernate pause filter)
	delta := now.Sub(m.lastTickTime)
	m.lastTickTime = now
	if delta < 0 {
		return TickOutcome{Now: now}
	}
	deltaSec := int64(delta.Seconds())
	if delta > maxTickGap {
		// A long gap is most likely suspend/hibernate or a stopped process.
		// Advance the clock anchor but fail closed instead of crediting downtime.
		deltaSec = 0
	} else if deltaSec <= 0 {
		deltaSec = 1
	}

	// 3. Update Activity time
	if !isAFK && m.activityWatchOK { // Fix: Must not accumulate if AW is dead
		m.activeSeconds += deltaSec
		m.lastActivityAt = &now
	}

	// 4. Update Mode duration. A lock screen is not user time in any mode.
	if !isLocked {
		m.currentModeSeconds += deltaSec
		switch m.userMode {
		case UserModeStandby:
			m.standbySeconds += deltaSec
		case UserModeStudy:
			m.studySeconds += deltaSec
		case UserModeBreak:
			m.breakSeconds += deltaSec
		case UserModeOff:
			m.offSeconds += deltaSec
		}
	}

	// 5. Privacy Gate Evaluation (local rules first)
	if m.privacyGate != nil {
		m.privacy = m.privacyGate.Evaluate(app, title, domain)
	} else {
		m.privacy = PrivacyNormal
	}

	// 6. Interaction State Evaluation
	if !m.activityWatchOK {
		m.interaction = InteractionUnknown
		m.idleStaticSeconds = 0
	} else if !isAFK {
		m.interaction = InteractionActive
		m.idleStaticSeconds = 0
	} else {
		if screenChanged {
			m.interaction = InteractionIdleDynamic
			m.idleStaticSeconds = 0
		} else {
			m.interaction = InteractionIdleStatic
			m.idleStaticSeconds += deltaSec
		}
	}

	// 7. Task Relation Evaluation (from classification result). Lock screen
	// observations must not inherit a stale DISTRACTED result.
	effectiveClassification := classification
	if isLocked {
		m.interaction = InteractionUnknown
		m.relation = RelationUnknown
		m.confidence = 1.0
		effectiveClassification = ClassificationResult{Relation: RelationUnknown, Confidence: 1.0, Reason: "lock screen", SourceKind: SourceKindLocalRule, IsFromRule: true}
	} else {
		m.relation = classification.Relation
		m.confidence = classification.Confidence
	}

	if m.relation == RelationDistracted {
		m.distractedSeconds += deltaSec
	} else {
		m.distractedSeconds = 0
	}

	// 8. A reminder remains visible only until it expires or the user has
	// recovered a stable focused/healthy state. Recovery uses the same state
	// semantics as supervision and does not depend on a second wall clock.
	m.clearRecoveredReminderLocked(now)

	// 8. Reminder Engine Evaluation
	if m.reminderEng != nil {
		rem := m.reminderEng.Evaluate(ReminderDecisionInput{
			Now:               now,
			UserMode:          m.userMode,
			Task:              m.task,
			Interaction:       m.interaction,
			Relation:          m.relation,
			Privacy:           m.privacy,
			Confidence:        m.confidence,
			ActiveSeconds:     m.activeSeconds,
			StudySeconds:      m.studySeconds,
			BreakSeconds:      m.currentModeSeconds, // use current session instead of daily total for reminders
			DistractedSeconds: m.distractedSeconds,
			IdleStaticSeconds: m.idleStaticSeconds,
		})
		if rem != nil {
			if rem.ExpiresAt.IsZero() {
				cooldown := 10 * time.Minute
				if m.cfg != nil && m.cfg.Reminder.CooldownMinutes > 0 {
					cooldown = time.Duration(m.cfg.Reminder.CooldownMinutes) * time.Minute
				}
				rem.ExpiresAt = now.Add(cooldown)
			}
			rem.Active = true
			m.currentReminder = rem
			m.reminderRecoverySince = time.Time{}
			if m.storage != nil {
				cooldown := 10 * time.Minute
				if m.cfg != nil && m.cfg.Reminder.CooldownMinutes > 0 {
					cooldown = time.Duration(m.cfg.Reminder.CooldownMinutes) * time.Minute
				}
				_ = m.storage.RecordReminder(context.Background(), storage.ReminderRecord{
					ID:                   rem.ID,
					CreatedAt:            rem.CreatedAt,
					Mode:                 string(m.userMode),
					Level:                string(rem.Level),
					Message:              rem.Message,
					Reason:               rem.Reason,
					CooldownUntil:        now.Add(cooldown),
					ExpiresAt:            rem.ExpiresAt,
					RelatedDistractionID: rem.RelatedDistractionID,
					Active:               true,
				})
			}
			if m.toastNotifier != nil {
				_ = m.toastNotifier("StudyGuardian 提醒", rem.Message)
			}
		}
	}

	// 9. Persist Observation & Daily State periodically
	if m.storage != nil {
		_ = m.storage.RecordObservation(context.Background(), storage.ObservationRecord{
			Timestamp:   now,
			Interaction: string(m.interaction),
			Relation:    string(m.relation),
			Privacy:     string(m.privacy),
			Confidence:  m.confidence,
			Reason:      classification.Reason,
			CurrentMode: string(m.userMode),
			Task:        m.task,
		})

		_ = m.storage.UpdateDailyState(context.Background(), m.currentDate,
			m.standbySeconds, m.studySeconds, m.breakSeconds, m.offSeconds, m.activeSeconds, now)
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(m.userMode),
			Task:               m.task,
			StartedAt:          m.modeStartTime,
			DurationSeconds:    m.currentModeSeconds,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		})
	}

	return TickOutcome{
		Now:               now,
		DeltaSeconds:      deltaSec,
		UserMode:          m.userMode,
		Interaction:       m.interaction,
		Relation:          m.relation,
		ActivityValid:     m.activityWatchOK,
		Locked:            isLocked,
		IdleStaticSeconds: m.idleStaticSeconds,
		Classification:    effectiveClassification,
	}
}

func (m *Manager) checkMidnightResetLocked(now time.Time) {
	dateStr := now.Format("2006-01-02")
	if dateStr != m.currentDate {
		// Midnight crossed!
		m.closeCurrentSessionLocked(now, "DAILY_RESET")

		// Reset daily state
		m.currentDate = dateStr
		m.userMode = UserModeStandby
		m.studySeconds = 0
		m.breakSeconds = 0
		m.standbySeconds = 0
		m.offSeconds = 0
		m.activeSeconds = 0
		m.distractedSeconds = 0
		m.idleStaticSeconds = 0
		m.currentModeSeconds = 0
		m.clearCurrentReminderLocked()
		m.modeOrigin = ModeOriginManual
		m.pauseReason = PauseReasonNone
		m.autoResumeEligible = false
		m.modeStartTime = now
		m.currentSessID = newSessionID(now)

		if m.storage != nil {
			_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
				ID:                 m.currentSessID,
				Mode:               string(UserModeStandby),
				Task:               m.task,
				StartedAt:          now,
				ModeOrigin:         string(m.modeOrigin),
				PauseReason:        string(m.pauseReason),
				AutoResumeEligible: m.autoResumeEligible,
			})
		}
	}
}

func (m *Manager) closeCurrentSessionLocked(now time.Time, reason string) {
	if m.currentSessID != "" && m.storage != nil {
		if err := m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:                 m.currentSessID,
			Mode:               string(m.userMode),
			Task:               m.task,
			StartedAt:          m.modeStartTime,
			EndedAt:            &now,
			DurationSeconds:    m.currentModeSeconds,
			EndReason:          reason,
			ModeOrigin:         string(m.modeOrigin),
			PauseReason:        string(m.pauseReason),
			AutoResumeEligible: m.autoResumeEligible,
		}); err == nil && m.userMode == UserModeStudy {
			_, _ = m.storage.BumpEvidenceRevision(context.Background(), storage.LocalDate(m.modeStartTime), now)
		}
	}
}
