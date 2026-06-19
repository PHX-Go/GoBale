//go:build !linux && !windows

package cpu

func getOSProcessCPUTicks() int64 {
	return 0
}
