package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/storage"
)

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
	mu            sync.RWMutex
	clock         Clock
	cfg           *config.Config
	storage       *storage.Storage
	ruleEngine    RuleClassifier
	privacyGate   PrivacyEvaluator
	reminderEng   ReminderEvaluator

	currentDate   string
	userMode      UserMode
	task          string
	currentSessID string

	interaction   InteractionState
	relation      TaskRelation
	privacy       PrivacyState
	confidence    float64

	studySeconds      int64
	breakSeconds      int64
	standbySeconds    int64
	offSeconds        int64
	activeSeconds     int64
	distractedSeconds int64
	idleStaticSeconds int64

	modeStartTime  time.Time
	lastTickTime   time.Time
	lastActivityAt *time.Time

	activityWatchOK bool
	screenSensorOK  bool
	currentReminder *ReminderEvent
	feedbacks       []FeedbackRecord
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
		currentSessID:   fmt.Sprintf("sess-%d", now.Unix()),
	}

	if m.storage != nil {
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:        m.currentSessID,
			Mode:      string(UserModeStandby),
			StartedAt: now,
		})
	}

	return m
}

func NewManager(clock Clock) *Manager {
	return NewPersistentManager(clock, config.DefaultConfig(), nil, nil, nil, nil)
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
	m.currentReminder = nil
	m.currentSessID = fmt.Sprintf("sess-%d", now.Unix())

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
	m.currentReminder = nil
	m.currentSessID = fmt.Sprintf("sess-%d", now.Unix())

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
	m.currentReminder = nil
	m.currentSessID = fmt.Sprintf("sess-%d", now.Unix())

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

func (m *Manager) Tick(now time.Time, app, title, domain string, isAFK bool, screenChanged bool) {
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
	if !isAFK {
		m.activeSeconds += deltaSec
		m.lastActivityAt = &now
	}

	// 4. Update Mode duration
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

	// 5. Privacy Gate Evaluation (local rules first)
	if m.privacyGate != nil {
		m.privacy = m.privacyGate.Evaluate(app, title, domain)
	} else {
		m.privacy = PrivacyNormal
	}

	// 6. Interaction State Evaluation
	if !isAFK {
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

	// 7. Task Relation Evaluation (local rules)
	if m.ruleEngine != nil {
		res := m.ruleEngine.Classify(app, title, domain, m.task)
		m.relation = res.Relation
		m.confidence = res.Confidence

		if m.relation == RelationDistracted {
			m.distractedSeconds += deltaSec
		} else {
			m.distractedSeconds = 0
		}
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
			BreakSeconds:      m.breakSeconds,
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
					CooldownUntil: now.Add(10 * time.Minute),
				})
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
			CurrentMode: string(m.userMode),
			Task:        m.task,
		})

		_ = m.storage.UpdateDailyState(context.Background(), m.currentDate,
			m.standbySeconds, m.studySeconds, m.breakSeconds, m.offSeconds, m.activeSeconds, now)
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
		m.currentReminder = nil
		m.modeStartTime = now
		m.currentSessID = fmt.Sprintf("sess-%d", now.Unix())

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
		dur := int64(now.Sub(m.modeStartTime).Seconds())
		_ = m.storage.SaveSession(context.Background(), storage.SessionRecord{
			ID:              m.currentSessID,
			Mode:            string(m.userMode),
			Task:            m.task,
			StartedAt:       m.modeStartTime,
			EndedAt:         &now,
			DurationSeconds: dur,
			EndReason:       reason,
		})
	}
}
