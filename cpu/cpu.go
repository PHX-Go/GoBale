package cpu

import (
	"runtime"
	"sync"
	"time"
)

var cpuMu sync.Mutex
var lastCPUTime int64
var lastSampleTime time.Time

func init() {
	lastCPUTime = getOSProcessCPUTicks()
	lastSampleTime = time.Now()
}

func GetProcessCPUUsage() float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()

	now := time.Now()
	ticks := getOSProcessCPUTicks()

	elapsedTime := float64(now.Sub(lastSampleTime).Microseconds())
	if elapsedTime <= 0 {
		return 0.0
	}

	cpuDelta := float64(ticks - lastCPUTime)
	lastCPUTime = ticks
	lastSampleTime = now

	rawPercent := (cpuDelta / elapsedTime) * 100.0

	numCPU := float64(runtime.NumCPU())
	percent := rawPercent / numCPU

	if percent < 0 {
		return 0.0
	}
	return percent
}