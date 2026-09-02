package motivation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
	"study-guardian/internal/storage"
)

type Event struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Status struct {
	TodayCreditedFocusMinutes int64   `json:"today_credited_focus_minutes"`
	TotalCreditedFocusMinutes int64   `json:"total_credited_focus_minutes"`
	TodayEarnedAPMilli        int64   `json:"today_earned_ap_milli"`
	TodaySpentAPMilli         int64   `json:"today_spent_ap_milli"`
	BalanceAPMilli            int64   `json:"balance_ap_milli"`
	CheckinCompleted          bool    `json:"checkin_completed"`
	DailyTargetMinutes        int     `json:"daily_target_minutes"`
	TargetProgress            float64 `json:"target_progress"`
	StreakDays                int     `json:"streak_days"`
	LastEvent                 *Event  `json:"last_event,omitempty"`
}
type Settings struct {
	DailyTargetMinutes int       `json:"daily_target_minutes"`
	UpdatedAt          time.Time `json:"updated_at"`
}
type HistoryDay struct {
	Date             string `json:"date"`
	FocusMinutes     int64  `json:"focus_minutes"`
	TargetMinutes    int64  `json:"target_minutes"`
	CheckinCompleted bool   `json:"checkin_completed"`
	TargetCompleted  bool   `json:"target_completed"`
}
type AchievementDefinition struct {
	ID          string     `json:"achievement_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Progress    float64    `json:"progress"`
	Unlocked    bool       `json:"unlocked"`
	UnlockedAt  *time.Time `json:"unlocked_at,omitempty"`
}

var definitions = []struct{ id, name, description string }{
	{"FIRST_30", "初次专注", "累计有效专注 30 分钟"}, {"DAILY_120", "今日达标", "单日有效专注 120 分钟"}, {"STREAK_3", "三日坚持", "连续打卡 3 天"}, {"STREAK_7", "一周坚持", "连续打卡 7 天"}, {"STREAK_30", "月度坚持", "连续打卡 30 天"}, {"WEEK_600", "周专注", "7 天累计有效专注 600 分钟"}, {"MISSION_10", "任务达人", "累计完成 10 个 Mission"}, {"COMEBACK", "重新出发", "分心提醒后重新累计有效专注 30 分钟"},
}

type Service struct {
	cfg       *config.Config
	store     *storage.Storage
	clock     state.Clock
	mu        sync.Mutex
	lastEvent *Event
}

func NewService(cfg *config.Config, store *storage.Storage) *Service {
	return NewServiceWithClock(cfg, store, state.RealClock{})
}

func NewServiceWithClock(cfg *config.Config, store *storage.Storage, clock state.Clock) *Service {
	if clock == nil {
		clock = state.RealClock{}
	}
	return &Service{cfg: cfg, store: store, clock: clock}
}
func (s *Service) enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Motivation.Enabled && s.store != nil
}
func (s *Service) RecordTick(out state.TickOutcome) {
	if !s.enabled() || out.Now.IsZero() || out.DeltaSeconds <= 0 {
		return
	}
	ctx := context.Background()
	if out.UserMode == state.UserModeStudy && out.Relation == state.RelationDistracted {
		_ = s.store.MarkComebackDistraction(ctx, out.Now)
		return
	}
	if out.UserMode != state.UserModeStudy || out.Locked || !out.ActivityValid || !creditEligible(out, s.cfg.Motivation.IdleStaticCreditGraceSeconds) {
		_ = s.store.ResetComebackFocus(ctx, out.Now)
		return
	}

	now := out.Now
	date := now.Format("2006-01-02")
	target, err := s.store.GetMotivationTarget(ctx, int64(s.cfg.Motivation.DefaultDailyTargetMinutes*60), now)
	if err != nil {
		return
	}
	checkin := int64(s.cfg.Motivation.CheckinThresholdMinutes * 60)
	newCheckin, newTarget, err := s.store.RecordCreditedFocus(ctx, date, out.DeltaSeconds, target, checkin, now)
	if err != nil {
		return
	}
	_, _ = s.store.AddComebackFocus(ctx, out.DeltaSeconds, now)
	if newCheckin {
		s.emit("CHECKIN_COMPLETED", "完成今日打卡，继续保持专注", now)
	}
	if newTarget {
		s.emit("DAILY_TARGET_COMPLETED", "今日有效专注目标完成", now)
	}
	s.evaluateAchievements(now)
}

func creditEligible(out state.TickOutcome, graceSeconds int) bool {
	switch out.Interaction {
	case state.InteractionActive, state.InteractionIdleDynamic:
		return true
	case state.InteractionIdleStatic:
		return out.Relation == state.RelationFocused && graceSeconds > 0 && out.IdleStaticSeconds <= int64(graceSeconds)
	default:
		return false
	}
}
func (s *Service) emit(kind, msg string, now time.Time) {
	var id int64
	if s.store != nil {
		if event, err := s.store.RecordUIEvent(context.Background(), kind, msg, "{}", now); err == nil {
			id = event.ID
			_ = s.store.PruneUIEvents(context.Background(), now.AddDate(0, 0, -90), 1000)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEvent = &Event{ID: id, Type: kind, Message: msg, CreatedAt: now}
}
func (s *Service) consumeEvent() *Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastEvent == nil {
		return nil
	}
	e := *s.lastEvent
	return &e
}

func (s *Service) GetStatus(ctx context.Context, now time.Time) (Status, error) {
	if !s.enabled() {
		return Status{DailyTargetMinutes: s.cfg.Motivation.DefaultDailyTargetMinutes}, nil
	}
	date := now.Format("2006-01-02")
	defaultTarget := int64(s.cfg.Motivation.DefaultDailyTargetMinutes * 60)
	target, err := s.store.GetMotivationTarget(ctx, defaultTarget, now)
	if err != nil {
		return Status{}, err
	}
	d, err := s.store.GetMotivationDaily(ctx, date)
	if err != nil {
		d = storage.MotivationDaily{Date: date, DailyTargetSeconds: target}
	}
	total, err := s.store.TotalCreditedFocus(ctx)
	if err != nil {
		return Status{}, err
	}
	ledger, err := s.store.SumAPLedger(ctx)
	if err != nil {
		return Status{}, err
	}
	if d.DailyTargetSeconds <= 0 {
		d.DailyTargetSeconds = target
	}
	target = d.DailyTargetSeconds
	if target <= 0 {
		target = int64(s.cfg.Motivation.DefaultDailyTargetMinutes * 60)
	}
	progress := 0.0
	if target > 0 {
		progress = float64(d.CreditedFocusSeconds) / float64(target)
		if progress > 1 {
			progress = 1
		}
	}
	rate := s.cfg.Motivation.APPerFocusHourMilli
	if rate <= 0 {
		rate = 1000
	}
	todayEarned, todaySpent, err := s.store.APSummaryForDate(ctx, date, rate)
	if err != nil {
		return Status{}, err
	}
	return Status{TodayCreditedFocusMinutes: d.CreditedFocusSeconds / 60, TotalCreditedFocusMinutes: total / 60, TodayEarnedAPMilli: todayEarned, TodaySpentAPMilli: todaySpent, BalanceAPMilli: max64(0, total*rate/3600+ledger), CheckinCompleted: d.CheckinCompleted, DailyTargetMinutes: int(target / 60), TargetProgress: progress, StreakDays: s.streak(ctx, date), LastEvent: s.consumeEvent()}, nil
}

func (s *Service) GetSettings(ctx context.Context, now time.Time) (Settings, error) {
	defaultSeconds := int64(s.cfg.Motivation.DefaultDailyTargetMinutes * 60)
	target, err := s.store.GetMotivationTarget(ctx, defaultSeconds, now)
	if err != nil {
		return Settings{}, err
	}
	return Settings{DailyTargetMinutes: int(target / 60), UpdatedAt: now}, nil
}

func (s *Service) SetDailyTarget(ctx context.Context, minutes int, now time.Time) (Settings, error) {
	if minutes < 1 || minutes > 1440 {
		return Settings{}, fmt.Errorf("daily target must be between 1 and 1440 minutes")
	}
	if err := s.store.SetMotivationTarget(ctx, int64(minutes*60), now.Format("2006-01-02"), now); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(ctx, now)
}

func (s *Service) Events(ctx context.Context, afterID int64, limit int) ([]storage.UIEvent, error) {
	return s.store.ListUIEvents(ctx, afterID, limit)
}
func (s *Service) GetHistory(ctx context.Context, days int, now time.Time) ([]HistoryDay, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	from := now.AddDate(0, 0, -days+1).Format("2006-01-02")
	rows, err := s.store.ListMotivationDaily(ctx, from)
	if err != nil {
		return nil, err
	}
	byDate := map[string]storage.MotivationDaily{}
	for _, d := range rows {
		byDate[d.Date] = d
	}
	out := make([]HistoryDay, 0, days)
	defaultTarget, err := s.store.GetMotivationTarget(ctx, int64(s.cfg.Motivation.DefaultDailyTargetMinutes*60), now)
	if err != nil {
		return nil, err
	}
	for i := 0; i < days; i++ {
		t := now.AddDate(0, 0, -i)
		key := t.Format("2006-01-02")
		d := byDate[key]
		dayTarget := d.DailyTargetSeconds
		if dayTarget == 0 {
			dayTarget = defaultTarget
		}
		out = append(out, HistoryDay{Date: key, FocusMinutes: d.CreditedFocusSeconds / 60, TargetMinutes: dayTarget / 60, CheckinCompleted: d.CheckinCompleted, TargetCompleted: d.TargetCompleted})
	}
	return out, nil
}
func (s *Service) streak(ctx context.Context, today string) int {
	dates, err := s.store.CheckinDates(ctx)
	if err != nil {
		return 0
	}
	set := map[string]bool{}
	for _, d := range dates {
		set[d] = true
	}
	start := today
	if !set[today] {
		if t, err := time.Parse("2006-01-02", today); err == nil {
			start = t.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	n := 0
	t, err := time.Parse("2006-01-02", start)
	if err != nil {
		return 0
	}
	for set[t.Format("2006-01-02")] {
		n++
		t = t.AddDate(0, 0, -1)
	}
	return n
}

func (s *Service) evaluateAchievements(now time.Time) {
	ctx := context.Background()
	total, _ := s.store.TotalCreditedFocus(ctx)
	daily, _ := s.store.GetMotivationDaily(ctx, now.Format("2006-01-02"))
	streak := s.streak(ctx, now.Format("2006-01-02"))
	missions, _ := s.store.MissionCount(ctx)
	history, _ := s.GetHistory(ctx, 7, now)
	week := int64(0)
	for _, d := range history {
		week += d.FocusMinutes
	}
	comebackFocus, _ := s.store.ComebackFocusSeconds(ctx)
	checks := map[string]bool{"FIRST_30": total >= 1800, "DAILY_120": daily.CreditedFocusSeconds >= 7200, "STREAK_3": streak >= 3, "STREAK_7": streak >= 7, "STREAK_30": streak >= 30, "WEEK_600": week >= 600, "MISSION_10": missions >= 10, "COMEBACK": comebackFocus >= 1800}
	for id, ok := range checks {
		if ok {
			if unlocked, _ := s.store.UnlockAchievement(ctx, id, now, "{}"); unlocked {
				s.emit("ACHIEVEMENT_UNLOCKED", "解锁成就："+id, now)
			}
		}
	}
}
func (s *Service) Achievements(ctx context.Context, now time.Time) ([]AchievementDefinition, error) {
	unlocked, err := s.store.ListAchievements(ctx)
	if err != nil {
		return nil, err
	}
	by := map[string]storage.Achievement{}
	for _, a := range unlocked {
		by[a.ID] = a
	}
	total, _ := s.store.TotalCreditedFocus(ctx)
	daily, _ := s.store.GetMotivationDaily(ctx, now.Format("2006-01-02"))
	streak := s.streak(ctx, now.Format("2006-01-02"))
	missions, _ := s.store.MissionCount(ctx)
	history, _ := s.GetHistory(ctx, 7, now)
	week := int64(0)
	for _, d := range history {
		week += d.FocusMinutes
	}
	dailyTarget := int64(7200)
	comebackProgress := 0.0
	comebackFocus, _ := s.store.ComebackFocusSeconds(ctx)
	if comebackFocus > 0 {
		comebackProgress = float64(comebackFocus) / 1800
	}
	vals := map[string]float64{"FIRST_30": float64(total) / 1800, "DAILY_120": float64(daily.CreditedFocusSeconds) / float64(dailyTarget), "STREAK_3": float64(streak) / 3, "STREAK_7": float64(streak) / 7, "STREAK_30": float64(streak) / 30, "WEEK_600": float64(week) / 600, "MISSION_10": float64(missions) / 10, "COMEBACK": comebackProgress}
	out := make([]AchievementDefinition, 0, len(definitions))
	for _, def := range definitions {
		p := vals[def.id]
		if p > 1 {
			p = 1
		}
		item := AchievementDefinition{ID: def.id, Name: def.name, Description: def.description, Progress: p, Unlocked: p >= 1}
		if a, ok := by[def.id]; ok {
			item.Unlocked = true
			t := a.UnlockedAt
			item.UnlockedAt = &t
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) Missions(ctx context.Context) ([]storage.Mission, error) {
	return s.store.ListMissions(ctx)
}
func (s *Service) CreateMission(ctx context.Context, title, description string, reward int64, due *string) (storage.Mission, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return storage.Mission{}, fmt.Errorf("title is required")
	}
	now := s.clock.Now()
	m := storage.Mission{ID: fmt.Sprintf("mission-%d", now.UnixNano()), Title: title, Description: description, RewardMilliAP: reward, DueDate: due, CreatedAt: now}
	return m, s.store.CreateMission(ctx, m)
}
func (s *Service) CompleteMission(ctx context.Context, id string) (storage.Mission, bool, error) {
	now := s.clock.Now()
	m, done, err := s.store.CompleteMission(ctx, id, now)
	if err == nil && done {
		s.emit("MISSION_COMPLETED", "完成任务："+m.Title, now)
		s.evaluateAchievements(now)
	}
	return m, done, err
}
func (s *Service) CancelMission(ctx context.Context, id string) error {
	return s.store.CancelMission(ctx, id)
}
func (s *Service) Rewards(ctx context.Context) ([]storage.Reward, error) {
	return s.store.ListRewards(ctx)
}
func (s *Service) RedeemReward(ctx context.Context, id string) (storage.Redemption, error) {
	rate := s.cfg.Motivation.APPerFocusHourMilli
	if rate <= 0 {
		rate = 1000
	}
	now := s.clock.Now()
	r, err := s.store.RedeemReward(ctx, id, now, rate)
	if err == nil {
		s.emit("REWARD_REDEEMED", "已兑换："+r.RewardName, now)
	}
	return r, err
}
func (s *Service) EventStatus() *Event { return s.consumeEvent() }
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
