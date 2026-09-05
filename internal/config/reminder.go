package config

import (
	"fmt"
	"strconv"
	"strings"
)

type QuietPeriodConfig struct {
	Start string `yaml:"start" json:"start"`
	End   string `yaml:"end" json:"end"`
}

type MinutePeriod struct {
	Start int
	End   int
}

func ParseClockMinute(value string, end bool) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("time must use HH:MM")
	}
	hour, errHour := strconv.Atoi(parts[0])
	minute, errMinute := strconv.Atoi(parts[1])
	if errHour != nil || errMinute != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	if hour == 24 && minute == 0 && end {
		return 1440, nil
	}
	if hour < 0 || hour > 23 {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	return hour*60 + minute, nil
}

func ParseQuietPeriods(periods []QuietPeriodConfig) ([]MinutePeriod, error) {
	parsed := make([]MinutePeriod, 0, len(periods))
	for _, period := range periods {
		start, err := ParseClockMinute(period.Start, false)
		if err != nil {
			return nil, fmt.Errorf("quiet period start: %w", err)
		}
		end, err := ParseClockMinute(period.End, true)
		if err != nil {
			return nil, fmt.Errorf("quiet period end: %w", err)
		}
		if end < 1 || start >= end {
			return nil, fmt.Errorf("quiet period start must be before end")
		}
		parsed = append(parsed, MinutePeriod{Start: start, End: end})
	}
	for i := range parsed {
		for j := i + 1; j < len(parsed); j++ {
			if parsed[i].Start < parsed[j].End && parsed[j].Start < parsed[i].End {
				return nil, fmt.Errorf("quiet periods must not overlap")
			}
		}
	}
	return parsed, nil
}

func ValidateReminderConfig(cfg *Config) error {
	if _, err := ParseQuietPeriods(cfg.Reminder.QuietPeriods); err != nil {
		return err
	}
	return nil
}
