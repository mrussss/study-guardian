package state

import (
	"errors"
	"sync"
	"time"
)

type Manager struct {
	mu              sync.RWMutex
	clock           Clock
	userMode        UserMode
	task            string
	interaction     InteractionState
	relation        TaskRelation
	privacy         PrivacyState
	confidence      float64
	studyStartTime  *time.Time
	breakStartTime  *time.Time
	studySeconds    int64
	breakSeconds    int64
	activeSeconds   int64
	lastActivityAt  *time.Time
	activityWatchOK bool
	screenSensorOK  bool
	currentReminder *ReminderEvent
	feedbacks       []FeedbackRecord
}

type FeedbackRecord struct {
	EventID   string
	Feedback  string
	CreatedAt time.Time
}

func NewManager(clock Clock) *Manager {
	if clock == nil {
		clock = RealClock{}
	}
	now := clock.Now()
	return &Manager{
		clock:           clock,
		userMode:        UserModeStandby,
		interaction:     InteractionUnknown,
		relation:        RelationUnknown,
		privacy:         PrivacyNormal,
		confidence:      1.0,
		lastActivityAt:  &now,
		activityWatchOK: true,
		screenSensorOK:  true,
	}
}

func (m *Manager) GetStatus() SystemStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := m.clock.Now()
	studySec := m.studySeconds
	if m.userMode == UserModeStudy && m.studyStartTime != nil {
		studySec += int64(now.Sub(*m.studyStartTime).Seconds())
	}

	breakSec := m.breakSeconds
	if m.userMode == UserModeBreak && m.breakStartTime != nil {
		breakSec += int64(now.Sub(*m.breakStartTime).Seconds())
	}

	return SystemStatus{
		UserMode:         m.userMode,
		InteractionState: m.interaction,
		TaskRelation:     m.relation,
		PrivacyState:     m.privacy,
		Confidence:       m.confidence,
		Task:             m.task,
		StudySeconds:     studySec,
		BreakSeconds:     breakSec,
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

	// Valid transitions: STANDBY -> STUDY, BREAK -> STUDY, OFF -> STUDY, STUDY -> STUDY (task update)
	now := m.clock.Now()
	if m.userMode == UserModeBreak && m.breakStartTime != nil {
		m.breakSeconds += int64(now.Sub(*m.breakStartTime).Seconds())
		m.breakStartTime = nil
	}

	m.userMode = UserModeStudy
	if task != "" {
		m.task = task
	}
	if m.studyStartTime == nil {
		m.studyStartTime = &now
	}
	m.currentReminder = nil
	return nil
}

func (m *Manager) SetModeBreak() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.userMode != UserModeStudy && m.userMode != UserModeStandby {
		return errors.New("cannot enter BREAK from current mode")
	}

	now := m.clock.Now()
	if m.userMode == UserModeStudy && m.studyStartTime != nil {
		m.studySeconds += int64(now.Sub(*m.studyStartTime).Seconds())
		m.studyStartTime = nil
	}

	m.userMode = UserModeBreak
	m.breakStartTime = &now
	m.currentReminder = nil
	return nil
}

func (m *Manager) SetModeOff() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	if m.userMode == UserModeStudy && m.studyStartTime != nil {
		m.studySeconds += int64(now.Sub(*m.studyStartTime).Seconds())
		m.studyStartTime = nil
	}
	if m.userMode == UserModeBreak && m.breakStartTime != nil {
		m.breakSeconds += int64(now.Sub(*m.breakStartTime).Seconds())
		m.breakStartTime = nil
	}

	m.userMode = UserModeOff
	m.currentReminder = nil
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
	m.feedbacks = append(m.feedbacks, FeedbackRecord{
		EventID:   eventID,
		Feedback:  feedback,
		CreatedAt: m.clock.Now(),
	})
	return nil
}

func (m *Manager) UpdateObservation(obs Observation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interaction = obs.Interaction
	m.relation = obs.Relation
	m.privacy = obs.Privacy
	m.confidence = obs.Confidence
}

func (m *Manager) SetHealth(awOK, sensorOK bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activityWatchOK = awOK
	m.screenSensorOK = sensorOK
}
