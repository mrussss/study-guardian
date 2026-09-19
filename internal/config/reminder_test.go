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
		{{Start: "12:00", End: "12:00"}},
		{{Start: "12:00", End: "14:00"}, {Start: "13:59", End: "15:00"}},
		{{Start: "12:00", End: "14:00"}, {Start: "12:00", End: "14:00"}},
	}
	for _, value := range invalid {
		if _, err := ParseQuietPeriods(value); err == nil {
			t.Fatalf("expected rejection: %+v", value)
		}
	}
}

func TestCrossMidnightQuietPeriodsParseWithoutOverlappingNeighbors(t *testing.T) {
	periods, err := ParseQuietPeriods([]QuietPeriodConfig{{Start: "22:00", End: "02:00"}, {Start: "02:00", End: "03:00"}})
	if err != nil || len(periods) != 2 {
		t.Fatalf("periods=%+v err=%v", periods, err)
	}
	if _, err := ParseQuietPeriods([]QuietPeriodConfig{{Start: "22:00", End: "02:00"}, {Start: "01:00", End: "03:00"}}); err == nil {
		t.Fatal("overlapping cross-midnight periods were accepted")
	}
	period := periods[0]
	for minute, want := range map[int]bool{1319: false, 1320: true, 1439: true, 0: true, 119: true, 120: false, 720: false} {
		if got := IsQuietTime(minute, []MinutePeriod{period}); got != want {
			t.Errorf("minute %d quiet=%v want=%v", minute, got, want)
		}
	}
}

func TestDefaultQuietPeriodsMatchProductRequirement(t *testing.T) {
	got := DefaultConfig().Reminder.QuietPeriods
	if len(got) != 3 || got[0].Start != "12:00" || got[1].Start != "17:30" || got[2].End != "24:00" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}
