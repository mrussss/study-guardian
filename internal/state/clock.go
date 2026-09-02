package state

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

type FakeClock struct {
	currentTime time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{currentTime: t}
}

func (f *FakeClock) Now() time.Time {
	return f.currentTime
}

func (f *FakeClock) Set(t time.Time) {
	f.currentTime = t
}

func (f *FakeClock) Advance(d time.Duration) {
	f.currentTime = f.currentTime.Add(d)
}
