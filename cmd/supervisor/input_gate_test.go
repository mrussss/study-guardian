package main

import "testing"

func TestInputGateStopsAfterThirtySecondsWithoutInput(t *testing.T) {
	for _, c := range []struct {
		watcher        bool
		idle           int64
		available, afk bool
	}{
		{false, 29, true, false}, {false, 30, true, true}, {false, 3600, true, true},
		{true, 0, true, true}, {false, 0, true, false}, {false, 3600, false, false},
	} {
		if got := inputIsAFK(c.watcher, c.idle, c.available); got != c.afk {
			t.Fatalf("%+v: %v", c, got)
		}
	}
}
