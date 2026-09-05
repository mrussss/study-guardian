package config

import "testing"

func TestParseQuietPeriodsAllowsOnlyEndSentinel2400(t *testing.T) {
	periods, err := ParseQuietPeriods([]QuietPeriodConfig{{Start: "21:00", End: "24:00"}})
	if err != nil || len(periods) != 1 || periods[0].Start != 1260 || periods[0].End != 1440 {
		t.Fatalf("periods=%+v err=%v", periods, err)
	}
	invalid := [][]QuietPeriodConfig{
		{{Start: "24:00", End: "24:00"}},
		{{Start: "21:00", End: "24:01"}},
		{{Start: "14:00", End: "12:00"}},
		{{Start: "12:00", End: "14:00"}, {Start: "13:59", End: "15:00"}},
		{{Start: "12:00", End: "14:00"}, {Start: "12:00", End: "14:00"}},
	}
	for _, value := range invalid {
		if _, err := ParseQuietPeriods(value); err == nil {
			t.Fatalf("expected rejection: %+v", value)
		}
	}
}

func TestDefaultQuietPeriodsMatchProductRequirement(t *testing.T) {
	got := DefaultConfig().Reminder.QuietPeriods
	if len(got) != 3 || got[0].Start != "12:00" || got[1].Start != "17:30" || got[2].End != "24:00" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}
