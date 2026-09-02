package activitywatch

import (
	"context"
	"sync"
	"time"
)

type FakeActivitySource struct {
	mu       sync.RWMutex
	snapshot *ActivitySnapshot
	isOK     bool
}

func NewFakeActivitySource(initial *ActivitySnapshot) *FakeActivitySource {
	if initial == nil {
		initial = &ActivitySnapshot{
			Timestamp: time.Now(),
			App:       "Code.exe",
			Title:     "main.go - study-guardian",
			IsAFK:     false,
		}
	}
	return &FakeActivitySource{
		snapshot: initial,
		isOK:     true,
	}
}

func (f *FakeActivitySource) Health(ctx context.Context) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.isOK
}

func (f *FakeActivitySource) SetHealth(ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isOK = ok
}

func (f *FakeActivitySource) SetActivity(snap *ActivitySnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshot = snap
}

func (f *FakeActivitySource) GetLatestActivity(ctx context.Context) (*ActivitySnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.snapshot, nil
}
