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
	Enabled                    bool       `json:"enabled"`
	Phase                      Phase      `json:"phase"`
	LocalDate                  string     `json:"local_date"`
	FocusSegmentSeconds        int64      `json:"focus_segment_seconds"`
	FocusSinceLongBreakSeconds int64      `json:"focus_since_long_break_seconds"`
	BreakStartedAt             *time.Time `json:"break_started_at,omitempty"`
	PlannedBreakEndAt          *time.Time `json:"planned_break_end_at,omitempty"`
	DueAt                      *time.Time `json:"due_at,omitempty"`
	SnoozeUntil                *time.Time `json:"snooze_until,omitempty"`
	SnoozeCount                int        `json:"snooze_count"`
	CompletedShortBreaks       int        `json:"completed_short_breaks"`
	CompletedLongBreaks        int        `json:"completed_long_breaks"`
	RetryFocusAfterSeconds     int64      `json:"retry_focus_after_seconds"`
	Revision                   int64      `json:"revision"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	NotificationSuppressed     bool       `json:"notification_suppressed"`
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

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Service struct {
	mu       sync.Mutex
	cfg      config.EyeCareConfig
	reminder config.ReminderConfig
	store    *storage.Storage
	modes    ModeController
	now      func() time.Time
	status   Status
}

func New(cfg config.EyeCareConfig, reminder config.ReminderConfig, store *storage.Storage, modes ModeController) (*Service, error) {
	return NewWithClock(cfg, reminder, store, modes, time.Now)
}

func NewWithClock(cfg config.EyeCareConfig, reminder config.ReminderConfig, store *storage.Storage, modes ModeController, now func() time.Time) (*Service, error) {
	if now == nil {
		now = time.Now
	}
	configHolder := config.Config{EyeCare: cfg}
	config.NormalizeEyeCareConfig(&configHolder)
	cfg = configHolder.EyeCare
	if err := config.ValidateEyeCareConfig(cfg); err != nil {
		return nil, err
	}
	service := &Service{cfg: cfg, reminder: reminder, store: store, modes: modes, now: now}
	if store != nil {
		if raw, found, err := store.GetSetting(context.Background(), SettingsKey); err != nil {
			return nil, fmt.Errorf("read eye-care settings: %w", err)
		} else if found {
			var persisted config.EyeCareConfig
			if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
				return nil, fmt.Errorf("decode eye-care settings: %w", err)
			}
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
			service.rollDateLocked(current)
			wasExpired := service.finishExpiredBreakLocked(current)
			if wasExpired {
				service.status.Revision++
				service.status.UpdatedAt = current
				_ = service.persistLocked(current, "COMPLETED", "")
			}
			if !wasExpired && (service.status.Phase == ShortBreak || service.status.Phase == LongBreak || service.status.Phase == WaitingReturn) {
				service.status.Revision++
				_ = service.persistLocked(current, "RESTORED_AFTER_RESTART", "")
			}
		} else {
			_ = service.persistLocked(current, "", "")
		}
	}
	return service, nil
}

func freshStatus(cfg config.EyeCareConfig, now time.Time) Status {
	phase := Focusing
	if !cfg.Enabled {
		phase = Disabled
	}
	return Status{Enabled: cfg.Enabled, Phase: phase, LocalDate: localDate(now), UpdatedAt: now}
}

func statusFromRecord(cfg config.EyeCareConfig, record storage.EyeCareStateRecord) Status {
	return Status{
		Enabled: cfg.Enabled, Phase: Phase(record.Phase), LocalDate: record.LocalDate,
		FocusSegmentSeconds:        record.FocusSegmentSeconds,
		FocusSinceLongBreakSeconds: record.FocusSinceLongBreakSeconds,
		BreakStartedAt:             cloneTime(record.BreakStartedAt), PlannedBreakEndAt: cloneTime(record.PlannedBreakEndAt),
		DueAt: cloneTime(record.DueAt), SnoozeUntil: cloneTime(record.SnoozeUntil),
		SnoozeCount: record.SnoozeCount, CompletedShortBreaks: record.CompletedShortBreaks,
		CompletedLongBreaks: record.CompletedLongBreaks, RetryFocusAfterSeconds: record.RetryFocusAfterSeconds,
		Revision: record.Revision, UpdatedAt: record.UpdatedAt,
	}
}

func (s *Service) Settings() config.EyeCareConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) SaveSettings(value config.EyeCareConfig) (config.EyeCareConfig, error) {
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
	if s.store != nil {
		if err := s.store.SetSetting(context.Background(), SettingsKey, string(encoded), now); err != nil {
			return config.EyeCareConfig{}, err
		}
	}
	s.cfg = value
	s.status.Enabled = value.Enabled
	if !value.Enabled {
		s.status.Phase = Disabled
		s.status.DueAt, s.status.SnoozeUntil = nil, nil
	} else if s.status.Phase == Disabled {
		s.status.Phase = Focusing
	}
	s.status.Revision++
	s.status.UpdatedAt = now
	if err := s.persistLocked(now, "", ""); err != nil {
		return config.EyeCareConfig{}, err
	}
	return s.cfg, nil
}

// RecordCreditedFocus consumes only the seconds the motivation ledger
// successfully accepted. No wall-clock or mode-duration estimate is used.
func (s *Service) RecordCreditedFocus(seconds int64, out state.TickOutcome, canonical state.SystemStatus) {
	if s == nil || seconds <= 0 || out.UserMode != state.UserModeStudy || out.Locked || !out.ActivityValid {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := out.Now
	if now.IsZero() {
		now = s.now()
	}
	s.advanceLocked(now, canonical)
	if !s.cfg.Enabled || canonical.UserMode != state.UserModeStudy {
		return
	}
	if isBreakPhase(s.status.Phase) {
		return
	}
	s.status.FocusSegmentSeconds = addBounded(s.status.FocusSegmentSeconds, seconds)
	s.status.FocusSinceLongBreakSeconds = addBounded(s.status.FocusSinceLongBreakSeconds, seconds)
	s.status.UpdatedAt = now
	longDue := int64(s.cfg.LongBreakAfterFocusMinutes) * 60
	shortDue := int64(s.cfg.FocusMinutes) * 60
	if s.status.FocusSinceLongBreakSeconds >= longDue && s.status.FocusSinceLongBreakSeconds >= s.status.RetryFocusAfterSeconds && s.status.Phase != LongBreakDue {
		s.status.Phase = LongBreakDue
		s.status.DueAt = timePointer(now)
		s.status.SnoozeUntil = nil
		s.status.SnoozeCount = 0
		s.status.RetryFocusAfterSeconds = 0
		s.status.Revision++
		_ = s.persistLocked(now, "DUE", "")
		return
	}
	if s.status.Phase == Focusing && s.status.FocusSegmentSeconds >= shortDue && s.status.FocusSinceLongBreakSeconds >= s.status.RetryFocusAfterSeconds {
		s.status.Phase = ShortBreakDue
		s.status.DueAt = timePointer(now)
		s.status.SnoozeUntil = nil
		s.status.SnoozeCount = 0
		s.status.Revision++
		_ = s.persistLocked(now, "DUE", "")
		return
	}
	_ = s.persistLocked(now, "", "")
}

func (s *Service) Observe(now time.Time, canonical state.SystemStatus) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = s.now()
	}
	s.advanceLocked(now, canonical)
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	canonical := state.SystemStatus{}
	if s.modes != nil {
		canonical = s.modes.GetStatus()
	}
	s.advanceLocked(now, canonical)
	result := s.status
	result.BreakStartedAt = cloneTime(result.BreakStartedAt)
	result.PlannedBreakEndAt = cloneTime(result.PlannedBreakEndAt)
	result.DueAt = cloneTime(result.DueAt)
	result.SnoozeUntil = cloneTime(result.SnoozeUntil)
	result.NotificationSuppressed = s.inQuietHours(now)
	return result
}

func (s *Service) Act(request ActionRequest) (Status, error) {
	if !requestIDPattern.MatchString(request.RequestID) || request.ExpectedRevision < 0 {
		return Status{}, errors.New("invalid eye-care action envelope")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	if s.store != nil {
		seen, err := s.store.HasEyeCareRequest(ctx, request.RequestID)
		if err != nil {
			return Status{}, err
		}
		if seen {
			return s.status, nil
		}
	}
	now := s.now()
	canonical := state.SystemStatus{}
	if s.modes != nil {
		canonical = s.modes.GetStatus()
	}
	s.advanceLocked(now, canonical)
	if request.ExpectedRevision != s.status.Revision {
		return s.status, errors.New("stale eye-care revision")
	}
	if !s.cfg.Enabled {
		return s.status, errors.New("eye-care is disabled")
	}
	previous := s.status
	next := s.status
	event := ""
	long := false
	modeAction := ""
	switch request.Action {
	case StartShortBreak:
		if next.Phase != ShortBreakDue {
			return s.status, errors.New("short break is not due")
		}
		modeAction = "START"
		next.Phase = ShortBreak
		next.BreakStartedAt = timePointer(now)
		next.PlannedBreakEndAt = timePointer(now.Add(time.Duration(s.cfg.ShortBreakMinutes) * time.Minute))
		next.SnoozeUntil = nil
		event = "BREAK_STARTED"
	case StartLongBreak:
		if next.Phase != LongBreakDue {
			return s.status, errors.New("long break is not due")
		}
		modeAction, long = "START", true
		next.Phase = LongBreak
		next.BreakStartedAt = timePointer(now)
		next.PlannedBreakEndAt = timePointer(now.Add(time.Duration(s.cfg.LongBreakMinutes) * time.Minute))
		next.SnoozeUntil = nil
		event = "BREAK_STARTED"
	case Snooze:
		if !isDuePhase(next.Phase) || next.SnoozeCount >= s.cfg.MaxSnoozes {
			return s.status, errors.New("eye-care reminder cannot be snoozed")
		}
		next.SnoozeCount++
		next.SnoozeUntil = timePointer(now.Add(time.Duration(s.cfg.SnoozeMinutes) * time.Minute))
		event = "SNOOZED"
	case Skip, Dismiss:
		if !isDuePhase(next.Phase) {
			return s.status, errors.New("no due eye-care reminder")
		}
		next.Phase = Focusing
		next.RetryFocusAfterSeconds = next.FocusSinceLongBreakSeconds + 10*60
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "SKIPPED"
	case FinishEarly:
		if next.Phase != ShortBreak && next.Phase != LongBreak {
			return s.status, errors.New("no active eye-care break")
		}
		modeAction = "RESUME"
		next.Phase = Focusing
		next.RetryFocusAfterSeconds = next.FocusSinceLongBreakSeconds + 10*60
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "ABORTED"
	case ResumeStudy:
		if next.Phase != WaitingReturn {
			return s.status, errors.New("eye-care rest timer is not complete")
		}
		modeAction = "RESUME"
		next.Phase = Focusing
		next.BreakStartedAt, next.PlannedBreakEndAt = nil, nil
		next.DueAt, next.SnoozeUntil = nil, nil
		next.SnoozeCount = 0
		event = "RESUMED"
	default:
		return s.status, errors.New("unknown eye-care action")
	}
	next.Revision++
	next.UpdatedAt = now
	if modeAction == "START" {
		if s.modes == nil {
			return s.status, errors.New("mode controller unavailable")
		}
		if err := s.modes.SetModeEyeCareBreak(long); err != nil {
			return s.status, err
		}
	} else if modeAction == "RESUME" {
		if s.modes == nil {
			return s.status, errors.New("mode controller unavailable")
		}
		if err := s.modes.ResumeEyeCareStudy(); err != nil {
			return s.status, err
		}
	}
	s.status = next
	if err := s.persistLocked(now, event, request.RequestID); err != nil {
		// The database owns idempotency. Roll the visible state back; mode
		// transitions remain safe because the manager rejects repeated actions.
		s.status = previous
		return s.status, err
	}
	return s.status, nil
}

func (s *Service) advanceLocked(now time.Time, canonical state.SystemStatus) {
	s.rollDateLocked(now)
	if isBreakPhase(s.status.Phase) && (canonical.UserMode != state.UserModeBreak || canonical.ModeOrigin != state.ModeOriginEyeCare) {
		s.status.Phase = Focusing
		s.status.RetryFocusAfterSeconds = s.status.FocusSinceLongBreakSeconds + 10*60
		s.status.BreakStartedAt, s.status.PlannedBreakEndAt = nil, nil
		s.status.DueAt, s.status.SnoozeUntil = nil, nil
		s.status.Revision++
		s.status.UpdatedAt = now
		_ = s.persistLocked(now, "ABORTED", "")
		return
	}
	if s.finishExpiredBreakLocked(now) {
		s.status.Revision++
		s.status.UpdatedAt = now
		_ = s.persistLocked(now, "COMPLETED", "")
	}
	if s.status.SnoozeUntil != nil && !now.Before(*s.status.SnoozeUntil) {
		s.status.SnoozeUntil = nil
		s.status.UpdatedAt = now
		_ = s.persistLocked(now, "", "")
	}
}

func (s *Service) rollDateLocked(now time.Time) {
	date := localDate(now)
	if s.status.LocalDate == date {
		return
	}
	phase := s.status.Phase
	if phase != ShortBreak && phase != LongBreak && phase != WaitingReturn {
		phase = Focusing
	}
	s.status.LocalDate = date
	s.status.Phase = phase
	s.status.FocusSegmentSeconds = 0
	s.status.FocusSinceLongBreakSeconds = 0
	s.status.RetryFocusAfterSeconds = 0
	s.status.DueAt = nil
	s.status.SnoozeUntil = nil
	s.status.SnoozeCount = 0
	if !s.cfg.Enabled && phase == Focusing {
		s.status.Phase = Disabled
	}
	s.status.Revision++
	s.status.UpdatedAt = now
	_ = s.persistLocked(now, "", "")
}

func (s *Service) finishExpiredBreakLocked(now time.Time) bool {
	if (s.status.Phase != ShortBreak && s.status.Phase != LongBreak) || s.status.PlannedBreakEndAt == nil || now.Before(*s.status.PlannedBreakEndAt) {
		return false
	}
	if s.status.Phase == ShortBreak {
		s.status.CompletedShortBreaks++
		s.status.FocusSegmentSeconds = 0
	} else {
		s.status.CompletedLongBreaks++
		s.status.FocusSegmentSeconds = 0
		s.status.FocusSinceLongBreakSeconds = 0
		s.status.RetryFocusAfterSeconds = 0
	}
	s.status.Phase = WaitingReturn
	return true
}

func (s *Service) persistLocked(now time.Time, event, requestID string) error {
	if s.store == nil {
		return nil
	}
	value := s.status
	value.UpdatedAt = now
	record := storage.EyeCareStateRecord{
		LocalDate: value.LocalDate, Phase: string(value.Phase), FocusSegmentSeconds: value.FocusSegmentSeconds,
		FocusSinceLongBreakSeconds: value.FocusSinceLongBreakSeconds,
		BreakStartedAt:             cloneTime(value.BreakStartedAt), PlannedBreakEndAt: cloneTime(value.PlannedBreakEndAt),
		DueAt: cloneTime(value.DueAt), SnoozeUntil: cloneTime(value.SnoozeUntil), SnoozeCount: value.SnoozeCount,
		CompletedShortBreaks: value.CompletedShortBreaks, CompletedLongBreaks: value.CompletedLongBreaks,
		RetryFocusAfterSeconds: value.RetryFocusAfterSeconds, Revision: value.Revision, UpdatedAt: now,
	}
	var audit *storage.EyeCareAuditRecord
	if event != "" {
		audit = &storage.EyeCareAuditRecord{LocalDate: value.LocalDate, EventType: event, Phase: string(value.Phase), CreatedAt: now}
	}
	_, err := s.store.SaveEyeCareState(context.Background(), record, audit, requestID)
	return err
}

func (s *Service) inQuietHours(now time.Time) bool {
	periods, err := config.ParseQuietPeriods(s.reminder.QuietPeriods)
	if err != nil {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	for _, period := range periods {
		if minute >= period.Start && minute < period.End {
			return true
		}
	}
	return false
}

func localDate(now time.Time) string { return now.In(time.Local).Format("2006-01-02") }
func addBounded(current, delta int64) int64 {
	if delta > int64(^uint64(0)>>1)-current {
		return int64(^uint64(0) >> 1)
	}
	return current + delta
}
func isDuePhase(value Phase) bool { return value == ShortBreakDue || value == LongBreakDue }
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
