package storage

import (
	"fmt"
	"strings"
	"time"
)

const localDateLayout = "2006-01-02"

// canonicalDBTime removes Go's monotonic clock component and stores one
// unambiguous instant. SQLite receives UTC values; the calendar date used for
// daily accounting is computed separately from the original instant.
func canonicalDBTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.Round(0).UTC()
}

// LocalDate is the one canonical conversion used by all daily accounting
// writes. It intentionally uses the application's local timezone instead of
// SQLite's date() function, whose result would depend on the stored offset.
func LocalDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(time.Local).Format(localDateLayout)
}

func parseStoredDBTime(value any) (time.Time, bool) {
	switch raw := value.(type) {
	case time.Time:
		return raw.Round(0), !raw.IsZero()
	case string:
		raw = strings.TrimSpace(raw)
		// Go's time.Time.String includes a process-local monotonic suffix
		// (" m=+..."). It is not portable storage, but old databases may
		// contain it. Drop only that suffix before parsing the wall clock.
		if monotonic := strings.Index(raw, " m="); monotonic >= 0 {
			raw = raw[:monotonic]
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			"2006-01-02 15:04:05.999999999 -0700 MST",
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999Z07:00",
			"2006-01-02",
		} {
			if parsed, err := time.Parse(layout, raw); err == nil {
				return parsed, true
			}
		}
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", raw, time.Local); err == nil {
			return parsed, true
		}
	case []byte:
		return parseStoredDBTime(string(raw))
	}
	return time.Time{}, false
}

func requireLocalDate(value time.Time) (string, error) {
	date := LocalDate(value)
	if date == "" {
		return "", fmt.Errorf("timestamp is required")
	}
	return date, nil
}
