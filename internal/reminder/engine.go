package reminder

import (
	"fmt"
	"sync"
	"time"

	"study-guardian/internal/config"
	"study-guardian/internal/state"
)

type Engine struct {
	mu           sync.Mutex
	cfg          *config.Config
	cooldowns    map[string]time.Time
	lastEventID  int64
	quietPeriods []config.MinutePeriod
	wasQuiet     bool
	baseline     reminderBaseline
}

type reminderBaseline struct {
	active, distracted, idleStatic, breakSeconds int64
}

func NewEngine(cfg *config.Config) *Engine {
	periods, _ := config.ParseQuietPeriods(cfg.Reminder.QuietPeriods)
	return &Engine{
		cfg:          cfg,
		cooldowns:    make(map[string]time.Time),
		quietPeriods: periods,
	}
}

func (e *Engine) captureBaseline(input state.ReminderDecisionInput) {
	e.baseline = reminderBaseline{active: input.ActiveSeconds, distracted: input.DistractedSeconds, idleStatic: input.IdleStaticSeconds, breakSeconds: input.BreakSeconds}
}

func elapsedAfterBaseline(current int64, baseline *int64) int64 {
	if current < *baseline {
		*baseline = 0
	}
	return current - *baseline
}

func (e *Engine) isQuiet(now time.Time) bool {
	minute := now.Hour()*60 + now.Minute()
	for _, period := range e.quietPeriods {
		if minute >= period.Start && minute < period.End {
			return true
		}
	}
	return false
}

func (e *Engine) GetSettings() config.ReminderConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	settings := e.cfg.Reminder
	settings.QuietPeriods = append([]config.QuietPeriodConfig(nil), settings.QuietPeriods...)
	return settings
}

func (e *Engine) SetSettings(settings config.ReminderConfig) error {
	periods, err := config.ParseQuietPeriods(settings.QuietPeriods)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.Reminder = settings
	e.cfg.Reminder.QuietPeriods = append([]config.QuietPeriodConfig(nil), settings.QuietPeriods...)
	e.quietPeriods = periods
	e.wasQuiet = false
	e.baseline = reminderBaseline{}
	return nil
}

func (e *Engine) Evaluate(input state.ReminderDecisionInput) *state.ReminderEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	if input.UserMode == state.UserModeOff {
		return nil
	}
	if e.isQuiet(input.Now) {
		e.wasQuiet = true
		e.captureBaseline(input)
		return nil
	}
	if e.wasQuiet {
		e.wasQuiet = false
		e.captureBaseline(input)
		return nil
	}
	effectiveActive := elapsedAfterBaseline(input.ActiveSeconds, &e.baseline.active)
	effectiveDistracted := elapsedAfterBaseline(input.DistractedSeconds, &e.baseline.distracted)
	effectiveIdleStatic := elapsedAfterBaseline(input.IdleStaticSeconds, &e.baseline.idleStatic)
	effectiveBreak := elapsedAfterBaseline(input.BreakSeconds, &e.baseline.breakSeconds)

	cooldownDuration := time.Duration(e.cfg.Reminder.CooldownMinutes) * time.Minute
	if cooldownDuration <= 0 {
		cooldownDuration = 10 * time.Minute
	}

	switch input.UserMode {
	case state.UserModeStandby:
		// Standby reminder rule
		thresholdSec := int64(e.cfg.Standby.FirstStudyActiveMinutes * 60)
		if thresholdSec <= 0 {
			thresholdSec = 3600
		}
		if effectiveActive >= thresholdSec && input.StudySeconds == 0 {
			category := "standby_prompt"
			if e.isCoolingDown(category, input.Now) {
				return nil
			}
			repeatMins := e.cfg.Standby.RepeatReminderMinutes
			if repeatMins <= 0 {
				repeatMins = 30
			}
			e.setCooldown(category, input.Now, time.Duration(repeatMins)*time.Minute)
			return e.createEvent(state.ReminderLevelBubble,
				fmt.Sprintf("今天电脑已活跃 %d 分钟，是否开始今天的学习？", effectiveActive/60),
				"STANDBY_ACTIVE_PROMPT", input.Now)
		}

	case state.UserModeStudy:
		// 1. Distraction check
		strongDistractSec := int64(e.cfg.Study.DistractionStrongMinutes * 60)
		warnDistractSec := int64(e.cfg.Study.DistractionWarnMinutes * 60)
		if strongDistractSec <= 0 {
			strongDistractSec = 900
		}
		if warnDistractSec <= 0 {
			warnDistractSec = 480
		}

		if input.Relation == state.RelationDistracted {
			if effectiveDistracted >= strongDistractSec {
				category := "study_distraction_strong"
				if !e.isCoolingDown(category, input.Now) {
					e.setCooldown(category, input.Now, cooldownDuration)
					msg := fmt.Sprintf("你已经偏离学习任务 %d 分钟。先回到当前任务，再决定是否进入休息。", effectiveDistracted/60)
					return e.createEvent(state.ReminderLevelToast, msg, "DISTRACTION_STRONG", input.Now)
				}
			} else if effectiveDistracted >= warnDistractSec {
				category := "study_distraction_warn"
				if !e.isCoolingDown(category, input.Now) {
					e.setCooldown(category, input.Now, cooldownDuration)
					taskInfo := ""
					if input.Task != "" {
						taskInfo = fmt.Sprintf("当前任务：%s", input.Task)
					}
					msg := fmt.Sprintf("你已经偏离当前任务 %d 分钟。%s", effectiveDistracted/60, taskInfo)
					return e.createEvent(state.ReminderLevelBubble, msg, "DISTRACTION_WARN", input.Now)
				}
			}
		}

		// 2. Idle static check (no keyboard + static screen)
		strongIdleSec := int64(e.cfg.Study.IdleStaticStrongMinutes * 60)
		warnIdleSec := int64(e.cfg.Study.IdleStaticWarnMinutes * 60)
		if strongIdleSec <= 0 {
			strongIdleSec = 1800
		}
		if warnIdleSec <= 0 {
			warnIdleSec = 1200
		}

		if input.Interaction == state.InteractionIdleStatic {
			if effectiveIdleStatic >= strongIdleSec {
				category := "study_idle_static_strong"
				if !e.isCoolingDown(category, input.Now) {
					e.setCooldown(category, input.Now, cooldownDuration)
					return e.createEvent(state.ReminderLevelToast, "学习状态已长时间无输入且屏幕无变化，学习链条是否已中断？", "IDLE_STATIC_STRONG", input.Now)
				}
			} else if effectiveIdleStatic >= warnIdleSec {
				category := "study_idle_static_warn"
				if !e.isCoolingDown(category, input.Now) {
					e.setCooldown(category, input.Now, cooldownDuration)
					return e.createEvent(state.ReminderLevelBubble, "已较长时间没有操作，是否需要继续专注？", "IDLE_STATIC_WARN", input.Now)
				}
			}
		}

	case state.UserModeBreak:
		strongBreakSec := int64(e.cfg.Break.StrongMinutes * 60)
		warnBreakSec := int64(e.cfg.Break.WarnMinutes * 60)
		if strongBreakSec <= 0 {
			strongBreakSec = 1800
		}
		if warnBreakSec <= 0 {
			warnBreakSec = 1200
		}

		if effectiveBreak >= strongBreakSec {
			category := "break_strong"
			if !e.isCoolingDown(category, input.Now) {
				repeatMins := e.cfg.Break.RepeatMinutes
				if repeatMins <= 0 {
					repeatMins = 15
				}
				e.setCooldown(category, input.Now, time.Duration(repeatMins)*time.Minute)
				return e.createEvent(state.ReminderLevelToast, fmt.Sprintf("休息已满 %d 分钟，准备回到学习了吗？", effectiveBreak/60), "BREAK_TOO_LONG_STRONG", input.Now)
			}
		} else if effectiveBreak >= warnBreakSec {
			category := "break_warn"
			if !e.isCoolingDown(category, input.Now) {
				e.setCooldown(category, input.Now, cooldownDuration)
				return e.createEvent(state.ReminderLevelBubble, fmt.Sprintf("休息已满 %d 分钟，轻松一下后继续加油吧。", effectiveBreak/60), "BREAK_WARN", input.Now)
			}
		}
	}

	return nil
}

func (e *Engine) isCoolingDown(category string, now time.Time) bool {
	until, exists := e.cooldowns[category]
	if !exists {
		return false
	}
	return now.Before(until)
}

func (e *Engine) setCooldown(category string, now time.Time, duration time.Duration) {
	e.cooldowns[category] = now.Add(duration)
}

func (e *Engine) createEvent(level state.ReminderLevel, message, reason string, now time.Time) *state.ReminderEvent {
	e.lastEventID++
	return &state.ReminderEvent{
		ID:        fmt.Sprintf("rem-%d-%d", now.Unix(), e.lastEventID),
		Level:     level,
		Message:   message,
		Reason:    reason,
		CreatedAt: now,
	}
}
