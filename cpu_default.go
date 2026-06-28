//go:build !windows && !linux

package gobale

// getOSProcessCPUTicks is a fallback stub for unsupported operating systems
func getOSProcessCPUTicks() int64 {
	return 0
}
