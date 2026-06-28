//go:build linux

package gobale

import (
	"os"
	"strconv"
	"strings"
)

// getOSProcessCPUTicks parses process execution ticks specifically on Linux systems via procfs
func getOSProcessCPUTicks() int64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0
	}
	utime, _ := strconv.ParseInt(fields[13], 10, 64)
	stime, _ := strconv.ParseInt(fields[14], 10, 64)
	return (utime + stime) * 10000
}
