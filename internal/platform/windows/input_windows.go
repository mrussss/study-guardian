//go:build windows

package windows

import (
	"syscall"
	"unsafe"
)

var getLastInputInfo = user32.NewProc("GetLastInputInfo")
var getTickCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetTickCount")

func InputIdleSeconds() (int64, bool) {
	info := struct {
		Size uint32
		Tick uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ := getLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0, false
	}
	tick, _, _ := getTickCount.Call()
	// DWORD subtraction deliberately handles the 49-day tick counter wrap.
	return int64(uint32(tick)-info.Tick) / 1000, true
}
