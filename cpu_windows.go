//go:build windows

package gobale

import (
	"syscall"
	"unsafe"
)

// getOSProcessCPUTicks parses process execution ticks specifically on Windows systems
func getOSProcessCPUTicks() int64 {
	mod := syscall.NewLazyDLL("kernel32.dll")
	proc := mod.NewProc("GetProcessTimes")
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var c, e, k, u syscall.Filetime
	r1, _, _ := proc.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&c)),
		uintptr(unsafe.Pointer(&e)),
		uintptr(unsafe.Pointer(&k)),
		uintptr(unsafe.Pointer(&u)),
	)
	if r1 == 0 {
		return 0
	}
	kTime := int64(k.HighDateTime)<<32 + int64(k.LowDateTime)
	uTime := int64(u.HighDateTime)<<32 + int64(u.LowDateTime)
	return (kTime + uTime) / 10
}
