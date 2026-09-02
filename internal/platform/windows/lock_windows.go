//go:build windows

package windows

import (
	"syscall"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	openInputDesktop = user32.NewProc("OpenInputDesktop")
	closeDesktop = user32.NewProc("CloseDesktop")
)

func IsLocked() bool {
	hDesk, _, _ := openInputDesktop.Call(0, 0, 0x0100)
	if hDesk == 0 {
		return true
	}
	closeDesktop.Call(hDesk)
	return false
}
