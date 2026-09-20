package eyecare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

const SettingsKey = "eye_care.config.v1"

type Phase string

const (
	Disabled      Phase = "DISABLED"
	Focusing      Phase = "FOCUSING"
	ShortBreakDue Phase = "SHORT_BREAK_DUE"
	ShortBreak    Phase = "SHORT_BREAK"
	LongBreakDue  Phase = "LONG_BREAK_DUE"
	LongBreak     Phase = "LONG_BREAK"
	WaitingReturn Phase = "WAITING_RETURN"
)

type BreakContext string

const (
	BreakContextNone         BreakContext = "NONE"
	BreakContextStudyBound   BreakContext = "STUDY_BOUND"
	BreakContextReminderOnly BreakContext = "REMINDER_ONLY"
)

type Action string

const (
	StartShortBreak Action = "START_SHORT_BREAK"
	StartLongBreak  Action = "START_LONG_BREAK"
	Snooze          Action = "SNOOZE"
	Skip            Action = "SKIP"
	FinishEarly     Action = "FINISH_EARLY"
	ResumeStudy     Action = "RESUME_STUDY"
	Dismiss         Action = "DISMISS"
)

type Status struct {
	Enabled                    bool                        `json:"enabled"`
	CountingBasis              config.EyeCareCountingBasis `json:"counting_basis"`
	Phase                      Phase                       `json:"phase"`
	BreakContext               BreakContext                `json:"break_context"`
	LocalDate                  string                      `json:"local_date"`
	FocusSegmentSeconds        int64                       `json:"focus_segment_seconds"`
	FocusSinceLongBreakSeconds int64                       `json:"focus_since_long_break_seconds"`
	BreakStartedAt             *time.Time                  `json:"break_started_at,omitempty"`
	PlannedBreakEndAt          *time.Time                  `json:"planned_break_end_at,omitempty"`
	DueAt                      *time.Time                  `json:"due_at,omitempty"`
	SnoozeUntil                *time.Time                  `json:"snooze_until,omitempty"`
	SnoozeCount                int                         `json:"snooze_count"`
	CompletedShortBreaks       int                         `json:"completed_short_breaks"`
	CompletedLongBreaks        int                         `json:"completed_long_breaks"`
	RetryFocusAfterSeconds     int64                       `json:"retry_focus_after_seconds"`
	DueGeneration              int64                       `json:"due_generation"`
	NotifiedGeneration         int64                       `json:"notified_generation"`
	NotifiedAt                 *time.Time                  `json:"notified_at,omitempty"`
	Revision                   int64                       `json:"revision"`
	UpdatedAt                  time.Time                   `json:"updated_at"`
	NotificationSuppressed     bool                        `json:"notification_suppressed"`
	StorageDegraded            bool                        `json:"storage_degraded"`
	StorageErrorKind           string                      `json:"storage_error_kind,omitempty"`
}

type Notification struct {
	Kind    string
	Title   string
	Message string
}

type ActionRequest struct {
	Action           Action `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
	RequestID        string `json:"request_id"`
}

type ModeController interface {
	SetModeEyeCareBreak(long bool) error
	ResumeEyeCareStudy() error
	GetStatus() state.SystemStatus
}

type eyeCareBreakRestorer interface {
	RestoreEyeCareBreak(long bool) error
}

// ReminderSettingsProvider exposes the live canonical reminder settings. The
// reminder engine owns synchronization and returns a defensive copy.
type ReminderSettingsProvider interface {
	GetSettings() config.ReminderConfig
}

type staticReminderSettingsProvider struct {
	settings config.ReminderConfig
}

func (p staticReminderSettingsProvider) GetSettings() config.ReminderConfig {
	settings := p.settings
	settings.QuietPeriods = append([]config.QuietPeriodConfig(nil), p.settings.QuietPeriods...)
	return settings
}

type Store interface {
	GetSetting(context.Context, string) (string, bool, error)
	LoadEyeCareState(context.Context) (storage.EyeCareStateRecord, bool, error)
	SaveEyeCareState(context.Context, storage.EyeCareStateRecord, *storage.EyeCareAuditRecord, string) (bool, error)
	SaveEyeCareSettingsAndState(context.Context, string, string, time.Time, storage.EyeCareStateRecord) error
	ClaimEyeCareNotification(context.Context, int64, time.Time) (bool, error)
	BeginEyeCareRequest(context.Context, storage.EyeCareRequestRecord) (storage.EyeCareRequestRecord, bool, error)
	GetEyeCareRequest(context.Context, string) (storage.EyeCareRequestRecord, bool, error)
	CompleteEyeCareRequest(context.Context, string, string, storage.EyeCareStateRecord, *storage.EyeCareAuditRecord, string, time.Time) error
	FailEyeCareRequest(context.Context, string, string, string, string, int64, time.Time) error
	ListPendingEyeCareRequests(context.Context) ([]storage.EyeCareRequestRecord, error)
	PruneEyeCareRequests(context.Context, time.Time) error
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Service struct {
	mu                         sync.Mutex
	cfg                        config.EyeCareConfig
	reminderSettings           ReminderSettingsProvider
	store                      Store
	modes                      ModeController
	now                        func() time.Time
	status                     Status
	unpersistedCreditedSeconds int64
}

func New(cfg config.EyeCareConfig, reminder config.ReminderConfig, store Store, modes ModeController) (*Service, error) {
	return NewWithClock(cfg, reminder, store, modes, time.Now)
}

func NewWithClock(cfg config.EyeCareConfig, reminder config.ReminderConfig, store Store, modes ModeController, now func() time.Time) (*Service, error) {
	return NewWithReminderSettingsProviderAndClock(cfg, staticReminderSettingsProvider{settings: reminder}, store, modes, now)
}

func NewWithReminderSettingsProvider(cfg config.EyeCareConfig, reminderSettings ReminderSettingsProvider, store Store, modes ModeController) (*Service, error) {
	return NewWithReminderSettingsProviderAndClock(cfg, reminderSettings, store, modes, time.Now)
}

func NewWithReminderSettingsProviderAndClock(cfg config.EyeCareConfig, reminderSettings ReminderSettingsProvider, store Store, modes ModeController, now func() time.Time) (*Service, error) {
	if now == nil {
		now = time.Now
	}
	if reminderSettings == nil {
		reminderSettings = staticReminderSettingsProvider{settings: config.DefaultConfig().Reminder}
	}
	configHolder := config.Config{EyeCare: cfg}
	config.NormalizeEyeCareConfig(&configHolder)
	cfg = configHolder.EyeCare
	if err := config.ValidateEyeCareConfig(cfg); err != nil {
		return nil, err
	}
	service := &Service{cfg: cfg, reminderSettings: reminderSettings, store: store, modes: modes, now: now}
	if store != nil {
		if raw, found, err := store.GetSetting(context.Background(), SettingsKey); err != nil {
			return nil, fmt.Errorf("read eye-care settings: %w", err)
		} else if found {
			var persisted config.EyeCareConfig
			if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
				return nil, fmt.Errorf("decode eye-care settings: %w", err)
			}
			persistedHolder := config.Config{EyeCare: persisted}
			config.NormalizeEyeCareConfig(&persistedHolder)
			persisted = persistedHolder.EyeCare
			if err := config.ValidateEyeCareConfig(persisted); err != nil {
				return nil, fmt.Errorf("invalid persisted eye-care settings: %w", err)
			}
			service.cfg = persisted
		}
	}
	current := now()
	service.status = freshStatus(service.cfg, current)
	if store != nil {
		record, found, err := store.LoadEyeCareState(context.Background())
		if err != nil {
			return nil, fmt.Errorf("restore eye-care state: %w", err)
		}
		if found {
			service.status = statusFromRecord(service.cfg, record)
		} else {
			if err := service.persistCandidateLocked(service.status, current, "", "", "", ""); err != nil {
				service.setStorageFailure(err)
			}
		}
		if err := service.reconcilePendingRequests(current); err != nil {
			service.setStorageFailure(err)
		}
		if err := service.advanceLocked(current, service.currentMode()); err != nil {
			service.setStorageFailure(err)
		}
		if service.status.Phase == ShortBreak || service.status.Phase == LongBreak || service.status.Phase == WaitingReturn {
			next := cloneStatus(service.status)
			next.Revision++
			next.UpdatedAt = current
			if err := service.persistCandidateLocked(next, current, "RESTORED_AFTER_RESTART", "", "", ""); err != nil {
				service.setStorageFailure(err)
			} else {
				service.status = next
			}
		}
		if err := store.PruneEyeCareRequests(context.Background(), current); err != nil {
			service.setStorageFailure(err)
		}
	}
	return service, nil
}

func freshStatus(cfg config.EyeCareConfig, now time.Time) Status {
	phase := Focusing
	if !cfg.Enabled {
		phase = Disabled
	}
	return Status{Enabled: cfg.Enabled, CountingBasis: cfg.CountingBasis, Phase: phase, BreakContext: BreakContextNone, LocalDate: localDate(now), UpdatedAt: now}
}

func statusFromRecord(cfg config.EyeCareConfig, record storage.EyeCareStateRecord) Status {
	return Status{
		Enabled: cfg.Enabled, CountingBasis: cfg.CountingBasis, Phase: Phase(record.Phase), BreakContext: normalizeBreakContext(record.BreakContext, Phase(record.Phase)), LocalDate: record.LocalDate,
		FocusSegmentSeconds:        record.FocusSegmentSeconds,
		FocusSinceLongBreakSeconds: record.FocusSinceLongBreakSeconds,
		BreakStartedAt:             cloneTime(record.BreakStartedAt), PlannedBreakEndAt: cloneTime(record.PlannedBreakEndAt),
		DueAt: cloneTime(record.DueAt), SnoozeUntil: cloneTime(record.SnoozeUntil),
		SnoozeCount: record.SnoozeCount, CompletedShortBreaks: record.CompletedShortBreaks,
		CompletedLongBreaks: record.CompletedLongBreaks, RetryFocusAfterSeconds: record.RetryFocusAfterSeconds,
		DueGeneration: record.DueGeneration, NotifiedGeneration: record.NotifiedGeneration,
		NotifiedAt: cloneTime(record.NotifiedAt),
		Revision:   record.Revision, UpdatedAt: record.UpdatedAt,
	}
}

func (s *Service) Settings() config.EyeCareConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) SaveSettings(value config.EyeCareConfig) (config.EyeCareConfig, error) {
	normalized := config.Config{EyeCare: value}
	config.NormalizeEyeCareConfig(&normalized)
	value = normalized.EyeCare
	if err := config.ValidateEyeCareConfig(value); err != nil {
		return config.EyeCareConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !value.Enabled && isBreakPhase(s.status.Phase) {
		return config.EyeCareConfig{}, errors.New("finish the active eye-care break before disabling")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return config.EyeCareConfig{}, err
	}
	now := s.now()
	next := cloneStatus(s.status)
	next.Enabled = value.Enabled
	next.CountingBasis = value.CountingBasis
	if !value.Enabled {
		next.Phase = Disabled
		next.DueAt, next.SnoozeUntil = nil, nil
		next.DueGeneration = next.NotifiedGeneration
	} else if next.Phase == Disabled {
		next.Phase = Focusing
	}
	next.Revision++
	next.UpdatedAt = now
	if s.store != nil {
		if err := s.store.SaveEyeCareSettingsAndState(context.Background(), SettingsKey, string(encoded), now, stateRecord(next, now)); err != nil {
			s.setStorageFailure(err)
			return config.EyeCareConfig{}, storageFailure(err)
		}
	}
	s.cfg = value
	next.StorageDegraded, next.StorageErrorKind = false, ""
	s.status = next
	return s.cfg, nil
}

// RecordCredits routes both canonical credit streams through the same
// transactional eye-care state machine. Effective-focus callers pass the
// amount accepted by Motivation; computer-use callers pass the bounded credit
// produced by the current Supervisor tick.
func (s *Service) RecordCredits(creditedFocusSeconds, activeUseSeconds int64, out state.TickOutcome, canonical state.SystemStatus) error {
	s.mu.Lock()
	basis := s.cfg.CountingBasis
	s.mu.Unlock()
	if basis == config.EyeCareCountingBasisComputerUsage {
		return s.recordComputerUse(activeUseSeconds, out, canonical)
	}
	return s.RecordCreditedFocus(creditedFocusSeconds, out, canonical)
}

// RecordCreditedFocus consumes only the seconds the motivation ledger
// successfully accepted. No wall-clock or mode-duration estimate is used.
func (s *Service) RecordCreditedFocus(seconds int64, out state.TickOutcome, canonical state.SystemStatus) error {
	if s == nil || seconds <= 0 || out.UserMode != state.UserModeStudy || out.Locked || !out.ActivityValid {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := out.Now
	if now.IsZero() {
		now = s.now()
	}
	if err := s.advanceLocked(now, canonical); err != nil {
		s.unpersistedCreditedSeconds = addBounded(s.unpersistedCreditedSeconds, seconds)
		return err
	}
	if !s.cfg.Enabled || canonical.UserMode != state.UserModeStudy || isBreakPhase(s.status.Phase) {
		return nil
	}
	next := cloneStatus(s.status)
	credited := addBounded(seconds, s.unpersistedCreditedSeconds)
	next.FocusSegmentSeconds = addBounded(next.FocusSegmentSeconds, credited)
	next.FocusSinceLongBreakSeconds = addBounded(next.FocusSinceLongBreakSeconds, credited)
	next.UpdatedAt = now
	longDue := int64(s.cfg.LongBreakAfterFocusMinutes) * 60
	shortDue := int64(s.cfg.FocusMinutes) * 60
	event := ""
	if next.FocusSinceLongBreakSeconds >= longDue && next.FocusSinceLongBreakSeconds >= next.RetryFocusAfterSeconds && next.Phase != LongBreakDue {
		next.Phase = LongBreakDue
		next.DueAt = timePointer(now)
		next.SnoozeUntil = nil
		next.SnoozeCount = 0
		next.RetryFocusAfterSeconds = 0
		next.DueGeneration++
		event = "DUE"
	} else if next.Phase == Focusing && next.FocusSegmentSeconds >= shortDue && next.FocusSinceLongBreakSeconds >= next.RetryFocusAfterSeconds {
		next.Phase = ShortBreakDue
		next.DueAt = timePointer(now)
		next.SnoozeUntil = nil
		next.SnoozeCount = 0
		next.DueGeneration++
		event = "DUE"
	}
	if event != "" {
		next.Revision++
	}
	if err := s.persistCandidateLocked(next, now, event, "", "", ""); err != nil {
		s.setStorageFailure(err)
		s.unpersistedCreditedSeconds = addBounded(s.unpersistedCreditedSeconds, seconds)
		return err
	}
	s.unpersistedCreditedSeconds = 0
	next.StorageDegraded, next.StorageErrorKind = false, ""
	s.status = next
	return nil
}

func (s *Service) recordComputerUse(seconds int64, out state.TickOutcome, canonical state.SystemStatus) error {
	if s == nil || seconds <= 0 || !out.ActivityValid || out.Locked || out.Interaction != state.InteractionActive || out.UserMode == state.UserModeOff && out.AfkSeconds > 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := out.Now
	if now.IsZero() {
		now = s.now()
	}
	if err := s.advanceLocked(now, canonical); err != nil {
		s.unpersistedCreditedSeconds = addBounded(s.unpersistedCreditedSeconds, seconds)
		return err
	}
	if !s.cfg.Enabled || isDuePhase(s.status.Phase) || isBreakPhase(s.status.Phase) || canonical.ModeOrigin == state.ModeOriginEyeCare {
		return nil
	}
	next := cloneStatus(s.status)
	credited := addBounded(seconds, s.unpersistedCreditedSeconds)
	next.FocusSegmentSeconds = addBounded(next.FocusSegmentSeconds, credited)
	next.FocusSinceLongBreakSeconds = addBounded(next.FocusSinceLongBreakSeconds, credited)
	next.UpdatedAt = now
	longDue := int64(s.cfg.LongBreakAfterFocusMinutes) * 60
	shortDue := int64(s.cfg.FocusMinutes) * 60
	event := ""
	if next.FocusSinceLongBreakSeconds >= longDue && next.FocusSinceLongBreakSeconds >= next.RetryFocusAfterSeconds {
		next.Phase = LongBreakDue
		next.DueAt = timePointer(now)
		next.SnoozeUntil = nil
		next.SnoozeCount = 0
		next.RetryFocusAfterSeconds = 0
		next.DueGeneration++
		event = "DUE"
	} else if next.Phase == Focusing && next.FocusSegmentSeconds >= shortDue && next.FocusSinceLongBreakSeconds >= next.RetryFocusAfterSeconds {
		next.Phase = ShortBreakDue
		next.DueAt = timePointer(now)
		next.SnoozeUntil = nil
		next.SnoozeCount = 0
		next.DueGeneration++
		event = "DUE"
	}
	if event != "" {
		next.Revision++
	}
	if err := s.persistCandidateLocked(next, now, event, "", "", ""); err != nil {
		s.setStorageFailure(err)
		s.unpersistedCreditedSeconds = addBounded(s.unpersistedCreditedSeconds, seconds)
		return err
	}
	s.unpersistedCreditedSeconds = 0
	next.StorageDegraded, next.StorageErrorKind = false, ""
	s.status = next
	return nil
}

func (s *Service) Observe(now time.Time, canonical state.SystemStatus) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = s.now()
	}
	return s.advanceLocked(now, canonical)
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if err := s.advanceLocked(now, s.currentMode()); err != nil {
		s.setStorageFailure(err)
	}
	result := cloneStatus(s.status)
	quiet, quietErr := s.inQuietHours(now)
	result.NotificationSuppressed = quietErr != nil || quiet || (result.SnoozeUntil != nil && now.Before(*result.SnoozeUntil))
	return result
}

// TakePendingNotification durably claims one due generation. Toast delivery
// must happen after this method returns, outside the service mutex.
func (s *Service) TakePendingNotification(now time.Time) (*Notification, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = s.now()
	}
	if err := s.advanceLocked(now, s.currentMode()); err != nil {
		return nil, err
	}
	quiet, quietErr := s.inQuietHours(now)
	if quietErr != nil {
		return nil, errors.New("reminder settings unavailable")
	}
	if !isDuePhase(s.status.Phase) || (s.cfg.CountingBasis == config.EyeCareCountingBasisEffectiveFocus && s.currentMode().UserMode != state.UserModeStudy) || quiet || s.status.SnoozeUntil != nil && now.Before(*s.status.SnoozeUntil) || s.status.DueGeneration <= s.status.NotifiedGeneration {
		return nil, nil
	}
	if s.store != nil {
		claimed, err := s.store.ClaimEyeCareNotification(context.Background(), s.status.DueGeneration, now)
		if err != nil {
			s.setStorageFailure(err)
			return nil, storageFailure(err)
		}
		if !claimed {
			return nil, nil
		}
	}
	next := cloneStatus(s.status)
	next.NotifiedGeneration = next.DueGeneration
	next.NotifiedAt = timePointer(now)
	next.StorageDegraded, next.StorageErrorKind = false, ""
	s.status = next
	if next.SnoozeCount > 0 {
		return &Notification{Kind: "SNOOZE_EXPIRED", Title: "StudyGuardian 护眼提醒", Message: "延后时间到了，该让眼睛休息一下了。"}, nil
	}
	if s.cfg.CountingBasis == config.EyeCareCountingBasisComputerUsage {
		if next.Phase == LongBreakDue {
			return &Notification{Kind: "LONG_BREAK_DUE", Title: "StudyGuardian 完整休息", Message: fmt.Sprintf("已经连续使用电脑 %s。出去走走，看看远处，让眼睛真正离开近距离屏幕。", focusDurationLabel(s.cfg.LongBreakAfterFocusMinutes))}, nil
		}
		return &Notification{Kind: "SHORT_BREAK_DUE", Title: "StudyGuardian 护眼提醒", Message: fmt.Sprintf("已经连续使用电脑 %d 分钟。出去走走，看看远处，让眼睛真正离开近距离屏幕。", s.cfg.FocusMinutes)}, nil
	}
	if next.Phase == LongBreakDue {
		return &Notification{Kind: "LONG_BREAK_DUE", Title: "StudyGuardian 完整休息", Message: fmt.Sprintf("已累计有效专注 %s，建议安排一次 %d 分钟完整休息。", focusDurationLabel(s.cfg.LongBreakAfterFocusMinutes), s.cfg.LongBreakMinutes)}, nil
	}
	return &Notification{Kind: "SHORT_BREAK_DUE", Title: "StudyGuardian 护眼提醒", Message: fmt.Sprintf("已有效专注 %d 分钟，起来走动一下，看看远处吧。", s.cfg.FocusMinutes)}, nil
}

func focusDurationLabel(minutes int) string {
	if minutes >= 60 && minutes%60 == 0 {
		return fmt.Sprintf("%d 小时", minutes/60)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}

func (s *Service) currentMode() state.SystemStatus {
	if s.modes == nil {
		return state.SystemStatus{}
	}
	return s.modes.GetStatus()
}

func (s *Service) setStorageFailure(err error) {
	if err == nil {
		return
	}
	s.status.StorageDegraded = true
	s.status.StorageErrorKind = "storage_unavailable"
}

func (s *Service) PruneRequests(now time.Time) error {
	if s == nil || s.store == nil {
		return nil
	}
	if now.IsZero() {
		now = s.now()
	}
	if err := s.store.PruneEyeCareRequests(context.Background(), now); err != nil {
		s.mu.Lock()
		s.setStorageFailure(err)
		s.mu.Unlock()
		return storageFailure(err)
	}
	return nil
}

func (s *Service) Act(request ActionRequest) (Status, error) {
	if !requestIDPattern.MatchString(request.RequestID) || request.ExpectedRevision < 0 {
		return Status{}, errors.New("invalid eye-care action envelope")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	if s.store != nil {
		existing, found, err := s.store.GetEyeCareRequest(ctx, request.RequestID)
		if err != nil {
			s.setStorageFailure(err)
			return Status{}, storageFailure(err)
		}
		if found {
			if existing.Action != string(request.Action) {
				return s.status, errors.New("eye-care request id reused for different action")
			}
			if existing.Status == "APPLIED" || existing.Status == "FAILED" {
				var replay Status
				if err := json.Unmarshal([]byte(existing.ResultJSON), &replay); err != nil {
					return s.status, errors.New("eye-care request result unavailable")
				}
				if existing.Status == "FAILED" {
					return replay, errors.New(existing.ErrorKind)
				}
				return replay, nil
			}
			return s.status, errors.New("eye-care request pending reconciliation")
		}
	}
	now := s.now()
	canonical := s.currentMode()
	if err := s.advanceLocked(now, canonical); err != nil {
		return cloneStatus(s.status), err
	}
	if request.ExpectedRevision != s.status.Revision {
		return cloneStatus(s.status), errors.New("stale eye-care revision")
	}
	if !s.cfg.Enabled {
		return cloneStatus(s.status), errors.New("eye-care is disabled")
	}
	next := cloneStatus(s.status)
	event := ""
	long := false
	modeAction := ""
	switch request.Action {
	case StartShortBreak:
		if next.Phase != ShortBreakDue {
			return cloneStatus(s.status), errors.New("short break is not due")
		}
		if s.cfg.CountingBasis == config.EyeCareCountingBasisEffectiveFocus && canonical.UserMode != state.UserModeStudy {
			return cloneStatus(s.status), errors.New("eye-care break requires study mode")
		}
		next.BreakContext = BreakContextReminderOnly
		if canonical.UserMode == state.UserModeStudy {
			modeAction = "START"
			next.BreakContext = BreakContextStudyBound
		}
		next.Phase = ShortBreak
		next.BreakStartedAt = timePointer(now)
		next.PlannedBreakEndAt = timePointer(now.Add(time.Duration(s.cfg.ShortBreakMinutes) * time.Minute))
		next.SnoozeUntil = nil
		event = "BREAK_STARTED"
	case StartLongBreak:
		if next.Phase != LongBreakDue {
			return cloneStatus(s.status), errors.New("long break is not due")
		}
		if s.cfg.CountingBasis == config.EyeCareCountingBasisEffectiveFocus && canonical.UserMode != state.UserModeStudy {
			return cloneStatus(s.status), errors.New("eye-care break requires study mode")
		}
		long = true
		next.BreakContext = BreakContextReminderOnly
		if canonical.UserMode == state.UserModeStudy {
			modeAction = "START"
			next.BreakContext = BreakContextStudyBound
		}
		next.Phase = LongBreak
		next.BreakStartedAt = timePointer(now)
		next.PlannedBreakEndAt = timePointer(now.Add(time.Duration(s.cfg.LongBreakMinutes) * time.Minute))
		next.SnoozeUntil = nil
		event = "BREAK_STARTED"
	case Snooze:
		if !isDuePhase(next.Phase) || next.SnoozeCount >= s.cfg.MaxSnoozes {
			return cloneStatus(s.status), errors.New("eye-care reminder cannot be snoozed")
		}
		next.SnoozeCount++
		next.SnoozeUntil = timePointer(now.Add(time.Duration(s.cfg.SnoozeMinutes) * time.Minute))
		event = "SNOOZED"
	case Skip, Dismiss:
		if !isDuePhase(next.Phase) {
			return cloneStatus(s.status), errors.New("no due eye-care reminder")
		}
		next.Phase = Focusing
		next.BreakContext = BreakContextNone
		next.RetryFocusAfterSeconds = next.FocusSinceLongBreakSeconds + 10*60
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "SKIPPED"
	case FinishEarly:
		if next.Phase != ShortBreak && next.Phase != LongBreak {
			return cloneStatus(s.status), errors.New("no active eye-care break")
		}
		if next.BreakContext == BreakContextStudyBound && (canonical.UserMode != state.UserModeBreak || canonical.ModeOrigin != state.ModeOriginEyeCare || !isEyeCarePauseReason(canonical.PauseReason)) {
			return cloneStatus(s.status), errors.New("reconciliation_required: canonical eye-care break unavailable")
		}
		if next.BreakContext == BreakContextStudyBound {
			modeAction = "RESUME"
		}
		next.Phase = Focusing
		next.BreakContext = BreakContextNone
		next.RetryFocusAfterSeconds = next.FocusSinceLongBreakSeconds + 10*60
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "ABORTED"
	case ResumeStudy:
		if next.Phase != WaitingReturn {
			return cloneStatus(s.status), errors.New("eye-care rest timer is not complete")
		}
		if next.BreakContext == BreakContextStudyBound && (canonical.UserMode != state.UserModeBreak || canonical.ModeOrigin != state.ModeOriginEyeCare || !isEyeCarePauseReason(canonical.PauseReason)) {
			return cloneStatus(s.status), errors.New("reconciliation_required: canonical eye-care break unavailable")
		}
		if next.BreakContext == BreakContextStudyBound {
			modeAction = "RESUME"
		}
		next.Phase = Focusing
		next.BreakContext = BreakContextNone
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "RESUMED"
	default:
		return cloneStatus(s.status), errors.New("unknown eye-care action")
	}
	compensationLong := long
	if modeAction == "RESUME" {
		compensationLong = canonical.PauseReason == state.PauseReasonEyeCareLong
	}
	next.Revision++
	next.UpdatedAt = now
	next.StorageDegraded, next.StorageErrorKind = false, ""
	resultJSON, err := json.Marshal(next)
	if err != nil {
		return cloneStatus(s.status), err
	}
	if s.store != nil {
		ledger, created, err := s.store.BeginEyeCareRequest(ctx, storage.EyeCareRequestRecord{
			RequestID: request.RequestID, Action: string(request.Action), ResultJSON: string(resultJSON),
			ResultRevision: next.Revision, CreatedAt: now,
		})
		if err != nil {
			s.setStorageFailure(err)
			return cloneStatus(s.status), storageFailure(err)
		}
		if !created {
			if ledger.Action != string(request.Action) {
				return cloneStatus(s.status), errors.New("eye-care request id reused for different action")
			}
			if ledger.Status == "APPLIED" || ledger.Status == "FAILED" {
				var replay Status
				if decodeErr := json.Unmarshal([]byte(ledger.ResultJSON), &replay); decodeErr == nil {
					if ledger.Status == "FAILED" {
						return replay, errors.New(ledger.ErrorKind)
					}
					return replay, nil
				}
			}
			return cloneStatus(s.status), errors.New("eye-care request pending reconciliation")
		}
	}
	if modeAction == "START" {
		if s.modes == nil {
			return cloneStatus(s.status), s.failRequestLocked(request, next, errors.New("mode controller unavailable"), now)
		}
		if err := s.modes.SetModeEyeCareBreak(long); err != nil {
			return cloneStatus(s.status), s.failRequestLocked(request, next, err, now)
		}
	} else if modeAction == "RESUME" {
		if s.modes == nil {
			return cloneStatus(s.status), s.failRequestLocked(request, next, errors.New("mode controller unavailable"), now)
		}
		if err := s.modes.ResumeEyeCareStudy(); err != nil {
			return cloneStatus(s.status), s.failRequestLocked(request, next, err, now)
		}
	}
	var audit *storage.EyeCareAuditRecord
	if event != "" {
		audit = &storage.EyeCareAuditRecord{LocalDate: next.LocalDate, EventType: event, Phase: string(next.Phase), CreatedAt: now}
	}
	if s.store != nil {
		if err := s.store.CompleteEyeCareRequest(ctx, request.RequestID, string(request.Action), stateRecord(next, now), audit, string(resultJSON), now); err != nil {
			s.setStorageFailure(err)
			err = storageFailure(err)
			if modeAction != "" {
				if compensationErr := s.compensateModeLocked(modeAction, compensationLong); compensationErr != nil {
					s.setStorageFailure(fmt.Errorf("eye-care state commit failed and mode compensation failed: %w", compensationErr))
					return cloneStatus(s.status), errors.New("reconciliation_required")
				}
			}
			return cloneStatus(s.status), s.failRequestLocked(request, next, err, now)
		}
	} else if err := s.persistCandidateLocked(next, now, event, "", "", ""); err != nil {
		if modeAction != "" {
			if compensationErr := s.compensateModeLocked(modeAction, compensationLong); compensationErr != nil {
				s.setStorageFailure(compensationErr)
				return cloneStatus(s.status), errors.New("reconciliation_required")
			}
		}
		s.setStorageFailure(err)
		return cloneStatus(s.status), err
	}
	s.status = next
	return cloneStatus(s.status), nil
}

func (s *Service) advanceLocked(now time.Time, canonical state.SystemStatus) error {
	next := cloneStatus(s.status)
	event := ""
	changed := false
	if next.LocalDate != localDate(now) {
		phase := next.Phase
		if phase != ShortBreak && phase != LongBreak && phase != WaitingReturn {
			phase = Focusing
		}
		next.LocalDate = localDate(now)
		next.Phase = phase
		next.FocusSegmentSeconds = 0
		next.FocusSinceLongBreakSeconds = 0
		next.RetryFocusAfterSeconds = 0
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		if !s.cfg.Enabled && phase == Focusing {
			next.Phase = Disabled
		}
		changed = true
	}
	if s.cfg.CountingBasis == config.EyeCareCountingBasisEffectiveFocus && canonical.UserMode == state.UserModeOff && next.Phase != Disabled && next.Phase != Focusing {
		next.Phase = Focusing
		next.BreakContext = BreakContextNone
		if !s.cfg.Enabled {
			next.Phase = Disabled
		}
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		next.DueGeneration = next.NotifiedGeneration
		event = "CANCELED"
		changed = true
	} else if isBreakPhase(next.Phase) && next.BreakContext == BreakContextStudyBound && (canonical.UserMode != state.UserModeBreak || canonical.ModeOrigin != state.ModeOriginEyeCare) {
		wasWaiting := next.Phase == WaitingReturn
		wasOff := canonical.UserMode == state.UserModeOff
		next.Phase = Focusing
		next.BreakContext = BreakContextNone
		if !wasWaiting && !wasOff {
			next.RetryFocusAfterSeconds = next.FocusSinceLongBreakSeconds + 10*60
		}
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		if !s.cfg.Enabled {
			next.Phase = Disabled
		}
		event = "ABORTED"
		if wasWaiting {
			event = "RESUMED"
		}
		if wasOff {
			event = "CANCELED"
		}
		changed = true
	} else if (next.Phase == ShortBreak || next.Phase == LongBreak) && next.PlannedBreakEndAt != nil && !now.Before(*next.PlannedBreakEndAt) {
		if next.Phase == ShortBreak {
			next.CompletedShortBreaks++
			next.FocusSegmentSeconds = 0
		} else {
			next.CompletedLongBreaks++
			next.FocusSegmentSeconds = 0
			next.FocusSinceLongBreakSeconds = 0
			next.RetryFocusAfterSeconds = 0
		}
		next.Phase = WaitingReturn
		event = "COMPLETED"
		changed = true
	}
	if next.SnoozeUntil != nil && !now.Before(*next.SnoozeUntil) && isDuePhase(next.Phase) {
		next.SnoozeUntil = nil
		next.DueAt = timePointer(now)
		next.DueGeneration++
		event = "SNOOZE_EXPIRED"
		changed = true
	}
	if !changed {
		return nil
	}
	next.Revision++
	next.UpdatedAt = now
	if err := s.persistCandidateLocked(next, now, event, "", "", ""); err != nil {
		s.setStorageFailure(err)
		return err
	}
	next.StorageDegraded, next.StorageErrorKind = false, ""
	s.status = next
	return nil
}

func (s *Service) persistCandidateLocked(value Status, now time.Time, event, requestID, action, resultJSON string) error {
	if s.store == nil {
		return nil
	}
	var audit *storage.EyeCareAuditRecord
	if event != "" {
		audit = &storage.EyeCareAuditRecord{LocalDate: value.LocalDate, EventType: event, Phase: string(value.Phase), CreatedAt: now}
	}
	if requestID != "" && action != "" {
		return storageFailure(s.store.CompleteEyeCareRequest(context.Background(), requestID, action, stateRecord(value, now), audit, resultJSON, now))
	}
	_, err := s.store.SaveEyeCareState(context.Background(), stateRecord(value, now), audit, "")
	return storageFailure(err)
}

func stateRecord(value Status, now time.Time) storage.EyeCareStateRecord {
	return storage.EyeCareStateRecord{
		LocalDate: value.LocalDate, Phase: string(value.Phase), BreakContext: string(normalizeBreakContext(string(value.BreakContext), value.Phase)), FocusSegmentSeconds: value.FocusSegmentSeconds,
		FocusSinceLongBreakSeconds: value.FocusSinceLongBreakSeconds,
		BreakStartedAt:             cloneTime(value.BreakStartedAt), PlannedBreakEndAt: cloneTime(value.PlannedBreakEndAt),
		DueAt: cloneTime(value.DueAt), SnoozeUntil: cloneTime(value.SnoozeUntil), SnoozeCount: value.SnoozeCount,
		CompletedShortBreaks: value.CompletedShortBreaks, CompletedLongBreaks: value.CompletedLongBreaks,
		RetryFocusAfterSeconds: value.RetryFocusAfterSeconds, DueGeneration: value.DueGeneration,
		NotifiedGeneration: value.NotifiedGeneration, NotifiedAt: cloneTime(value.NotifiedAt),
		Revision: value.Revision, UpdatedAt: now,
	}
}

func (s *Service) failRequestLocked(request ActionRequest, planned Status, cause error, now time.Time) error {
	kind := "rejected"
	if strings.Contains(strings.ToLower(cause.Error()), "storage") || strings.Contains(strings.ToLower(cause.Error()), "database") {
		kind = "storage_unavailable"
		s.setStorageFailure(cause)
	}
	if s.store != nil {
		encoded, _ := json.Marshal(s.status)
		if err := s.store.FailEyeCareRequest(context.Background(), request.RequestID, string(request.Action), kind, string(encoded), s.status.Revision, now); err != nil {
			s.setStorageFailure(err)
			return storageFailure(errors.New("eye-care request outcome could not be persisted"))
		}
	}
	return cause
}

func (s *Service) compensateModeLocked(action string, long bool) error {
	if s.modes == nil {
		return errors.New("mode controller unavailable for compensation")
	}
	if action == "START" {
		return s.modes.ResumeEyeCareStudy()
	}
	if restorer, ok := s.modes.(eyeCareBreakRestorer); ok {
		return restorer.RestoreEyeCareBreak(long)
	}
	return s.modes.SetModeEyeCareBreak(long)
}

func (s *Service) reconcilePendingRequests(now time.Time) error {
	if s.store == nil {
		return nil
	}
	pending, err := s.store.ListPendingEyeCareRequests(context.Background())
	if err != nil {
		return err
	}
	canonical := s.currentMode()
	for _, request := range pending {
		var planned Status
		if err := json.Unmarshal([]byte(request.ResultJSON), &planned); err != nil {
			if failErr := s.store.FailEyeCareRequest(context.Background(), request.RequestID, request.Action, "reconciliation_required", "{}", s.status.Revision, now); failErr != nil {
				return failErr
			}
			continue
		}
		switch Action(request.Action) {
		case StartShortBreak, StartLongBreak:
			if planned.BreakContext == BreakContextStudyBound && (canonical.UserMode != state.UserModeBreak || canonical.ModeOrigin != state.ModeOriginEyeCare) {
				if failErr := s.store.FailEyeCareRequest(context.Background(), request.RequestID, request.Action, "reconciliation_required", "{}", s.status.Revision, now); failErr != nil {
					return failErr
				}
				continue
			}
		case FinishEarly, ResumeStudy:
			if canonical.ModeOrigin == state.ModeOriginEyeCare || canonical.UserMode == state.UserModeBreak && isEyeCarePauseReason(canonical.PauseReason) {
				if failErr := s.store.FailEyeCareRequest(context.Background(), request.RequestID, request.Action, "reconciliation_required", "{}", s.status.Revision, now); failErr != nil {
					return failErr
				}
				continue
			}
		}
		planned.StorageDegraded, planned.StorageErrorKind = false, ""
		resultJSON, err := json.Marshal(planned)
		if err != nil {
			return err
		}
		event := map[Action]string{
			StartShortBreak: "BREAK_STARTED_RECOVERED", StartLongBreak: "BREAK_STARTED_RECOVERED",
			Snooze: "SNOOZED_RECOVERED", Skip: "SKIPPED_RECOVERED", Dismiss: "SKIPPED_RECOVERED",
			FinishEarly: "ABORTED_RECOVERED", ResumeStudy: "RESUMED_RECOVERED",
		}[Action(request.Action)]
		var audit *storage.EyeCareAuditRecord
		if event != "" {
			audit = &storage.EyeCareAuditRecord{LocalDate: planned.LocalDate, EventType: event, Phase: string(planned.Phase), CreatedAt: now}
		}
		if err := s.store.CompleteEyeCareRequest(context.Background(), request.RequestID, request.Action, stateRecord(planned, now), audit, string(resultJSON), now); err != nil {
			return err
		}
		s.status = planned
	}
	return nil
}

func cloneStatus(value Status) Status {
	value.BreakStartedAt = cloneTime(value.BreakStartedAt)
	value.PlannedBreakEndAt = cloneTime(value.PlannedBreakEndAt)
	value.DueAt = cloneTime(value.DueAt)
	value.SnoozeUntil = cloneTime(value.SnoozeUntil)
	value.NotifiedAt = cloneTime(value.NotifiedAt)
	return value
}

func (s *Service) inQuietHours(now time.Time) (bool, error) {
	if s.reminderSettings == nil {
		return false, nil
	}
	settings := s.reminderSettings.GetSettings()
	periods, err := config.ParseQuietPeriods(settings.QuietPeriods)
	if err != nil {
		return true, err
	}
	return config.IsQuietTime(now.Hour()*60+now.Minute(), periods), nil
}

func localDate(now time.Time) string { return now.In(time.Local).Format("2006-01-02") }

func normalizeBreakContext(raw string, phase Phase) BreakContext {
	value := BreakContext(raw)
	if value == BreakContextStudyBound || value == BreakContextReminderOnly {
		return value
	}
	// Before break context was persisted every active eye-care break was
	// necessarily tied to STUDY. Treat legacy rows that are in a break phase as
	// study-bound so an upgrade cannot silently loosen the old behavior.
	if isBreakPhase(phase) {
		return BreakContextStudyBound
	}
	return BreakContextNone
}

func addBounded(current, delta int64) int64 {
	if delta > int64(^uint64(0)>>1)-current {
		return int64(^uint64(0) >> 1)
	}
	return current + delta
}
func isDuePhase(value Phase) bool { return value == ShortBreakDue || value == LongBreakDue }
func isEyeCarePauseReason(value state.PauseReason) bool {
	return value == state.PauseReasonEyeCareShort || value == state.PauseReasonEyeCareLong
}
func isBreakPhase(value Phase) bool {
	return value == ShortBreak || value == LongBreak || value == WaitingReturn
}
func timePointer(value time.Time) *time.Time { return &value }
func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (s *Service) ValidateActionRequest(request ActionRequest) error {
	request.RequestID = strings.TrimSpace(request.RequestID)
	if !requestIDPattern.MatchString(request.RequestID) {
		return errors.New("invalid request_id")
	}
	return nil
}

// ErrorKind returns a bounded public category; storage paths and driver error
// text are intentionally never exposed to the UI.
func ErrorKind(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "reconciliation_required"):
		return "reconciliation_required"
	case strings.Contains(message, "stale eye-care revision"):
		return "stale_revision"
	case strings.Contains(message, "request id reused"):
		return "request_conflict"
	case strings.Contains(message, "pending reconciliation"):
		return "request_pending"
	case strings.Contains(message, "storage") || strings.Contains(message, "database") || strings.Contains(message, "sqlite"):
		return "storage_unavailable"
	default:
		return "rejected"
	}
}

func storageFailure(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("eye-care storage unavailable: %w", err)
}
