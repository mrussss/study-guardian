//go:build !windows

package windows

func IsLocked() bool {
	return false
}
