//go:build windows

package cpu

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessTimes = modkernel32.NewProc("GetProcessTimes")
)

func getOSProcessCPUTicks() int64 {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}

	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	r1, _, _ := procGetProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if r1 == 0 {
		return 0
	}

	kTime := int64(kernelTime.HighDateTime)<<32 + int64(kernelTime.LowDateTime)
	uTime := int64(userTime.HighDateTime)<<32 + int64(userTime.LowDateTime)
	return (kTime + uTime) / 10
}