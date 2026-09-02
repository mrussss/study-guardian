package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

var sessionSequence atomic.Uint64

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

	currentDate   string
	userMode      UserMode
	task          string
	currentSessID string

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

	activityWatchOK bool
	screenSensorOK  bool
	currentReminder *ReminderEvent
	feedbacks       []FeedbackRecord

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
		clock:           clock,
		cfg:             cfg,
		storage:         store,
		ruleEngine:      ruleEngine,
		privacyGate:     privacyGate,
		reminderEng:     reminderEng,
		currentDate:     dateStr,
		userMode:        UserModeStandby,
		interaction:     InteractionUnknown,
		relation:        RelationUnknown,
		privacy:         PrivacyNormal,
		confidence:      1.0,
		modeStartTime:   now,
		lastTickTime:    now,
		lastActivityAt:  &now,
		activityWatchOK: true,
		screenSensorOK:  true,
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
			if openSess.StartedAt.Format("2006-01-02") == dateStr {
				switch UserMode(openSess.Mode) {
				case UserModeStandby, UserModeStudy, UserModeBreak, UserModeOff:
					m.userMode = UserMode(openSess.Mode)
					m.task = openSess.Task
					m.currentModeSeconds = openSess.DurationSeconds
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
			ID:              m.currentSessID,
			Mode:            string(m.userMode),
			Task:            m.task,
			StartedAt:       now,
			DurationSeconds: m.currentModeSeconds,
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

func (m *Manager) GetStatus() SystemStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SystemStatus{
		UserMode:         m.userMode,
		InteractionState: m.interaction,
		TaskRelation:     m.relation,
		PrivacyState:     m.privacy,
		Confidence:       m.confidence,
		Task:             m.task,
		StudySeconds:     m.studySeconds,
		BreakSeconds:     m.breakSeconds,
		ActiveSeconds:    m.activeSeconds,
		LastActivityAt:   m.lastActivityAt,
		ActivityWatchOK:  m.activityWatchOK,
		ScreenSensorOK:   m.screenSensorOK,
		CurrentReminder:  m.currentReminder,
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
	if task != "" {
		m.task = task
	}
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.currentReminder = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:        m.currentSessID,
			Mode:      string(UserModeStudy),
			Task:      m.task,
			StartedAt: now,
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
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.currentReminder = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:        m.currentSessID,
			Mode:      string(UserModeBreak),
			Task:      m.task,
			StartedAt: now,
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
	m.modeStartTime = now
	m.distractedSeconds = 0
	m.idleStaticSeconds = 0
	m.currentModeSeconds = 0
	m.currentReminder = nil
	m.currentSessID = newSessionID(now)

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:        m.currentSessID,
			Mode:      string(UserModeOff),
			Task:      m.task,
			StartedAt: now,
		})
	}
	return nil
}

func (m *Manager) SetTask(task string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.screenSensorOK = sensorOK
}

func (m *Manager) UpdateObservation(obs Observation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interaction = obs.Interaction
	m.relation = obs.Relation
	m.privacy = obs.Privacy
	m.confidence = obs.Confidence
}

func (m *Manager) Tick(now time.Time, app, title, domain string, isAFK bool, screenChanged bool, isLocked bool) {
	var classification ClassificationResult
	if m.ruleEngine != nil {
		classification = m.ruleEngine.Classify(app, title, domain, m.task)
	} else {
		classification = ClassificationResult{Relation: RelationUnknown, Confidence: 0.5}
	}
	m.TickWithClassification(now, app, title, domain, isAFK, screenChanged, isLocked, classification)
}

func (m *Manager) TickWithClassification(
	now time.Time,
	app, title, domain string,
	isAFK bool,
	screenChanged bool,
	isLocked bool,
	classification ClassificationResult,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Check midnight crossing
	m.checkMidnightResetLocked(now)

	// 2. Compute elapsed time (with sleep/hibernate pause filter)
	delta := now.Sub(m.lastTickTime)
	m.lastTickTime = now
	if delta < 0 {
		return
	}
	if delta > 30*time.Second {
		delta = 5 * time.Second
	}
	deltaSec := int64(delta.Seconds())
	if deltaSec <= 0 {
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
	if isLocked {
		m.interaction = InteractionUnknown
		m.relation = RelationUnknown
		m.confidence = 1.0
	} else {
		m.relation = classification.Relation
		m.confidence = classification.Confidence
	}

	if m.relation == RelationDistracted {
		m.distractedSeconds += deltaSec
	} else {
		m.distractedSeconds = 0
	}

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
			m.currentReminder = rem
			if m.storage != nil {
				_ = m.storage.RecordReminder(context.Background(), storage.ReminderRecord{
					ID:            rem.ID,
					CreatedAt:     rem.CreatedAt,
					Mode:          string(m.userMode),
					Level:         string(rem.Level),
					Message:       rem.Message,
					Reason:        rem.Reason,
					CooldownUntil: now.Add(time.Duration(m.cfg.Reminder.CooldownMinutes) * time.Minute),
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
			ID:              m.currentSessID,
			Mode:            string(m.userMode),
			Task:            m.task,
			StartedAt:       m.modeStartTime,
			DurationSeconds: m.currentModeSeconds,
		})
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
		m.currentReminder = nil
		m.modeStartTime = now
		m.currentSessID = newSessionID(now)

		if m.storage != nil {
			_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
				ID:        m.currentSessID,
				Mode:      string(UserModeStandby),
				Task:      m.task,
				StartedAt: now,
			})
		}
	}
}

func (m *Manager) closeCurrentSessionLocked(now time.Time, reason string) {
	if m.currentSessID != "" && m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:              m.currentSessID,
			Mode:            string(m.userMode),
			Task:            m.task,
			StartedAt:       m.modeStartTime,
			EndedAt:         &now,
			DurationSeconds: m.currentModeSeconds,
			EndReason:       reason,
		})
	}
}
