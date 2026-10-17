//go:build !windows

package windows

func InputIdleSeconds() (int64, bool) { return 0, false }
