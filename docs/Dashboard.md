# GoBale Monitoring Dashboard & Profiling Guide

GoBale includes a lightweight, built-in real-time monitoring dashboard and performance profiler. It operates as an embedded web server, providing diagnostic metrics and profiling hooks (`pprof`) without requiring external monitoring agents or database dependencies.

---

## 1. Quick Start

The dashboard is managed fluently via the `Dash()` builder. To prevent blocking your bot's polling or webhook loop, always launch the dashboard HTTP server in a separate goroutine.

```go
package main

import (
	"log"

	"github.com/PHX-Go/GoBale"
)

func main() {
	_ = gobale.Env().Go()
	token := gobale.GetEnv[string]("BALE_TOKEN")

	bot, err := gobale.New(token).DryRun().Go()
	if err != nil {
		log.Fatalf("Failed to init bot: %v", err)
	}

	// Launch the dashboard server in a separate background goroutine
	go func() {
		err := bot.Dash().
			Addr(":8080"). // Serve on port 8080 (defaults to :8080)
			Go()
		
		if err != nil {
			log.Printf("Dashboard server stopped: %v", err)
		}
	}()

	log.Println("Bot and Dashboard are running...")
	bot.Run().Polling().Go()
}
```

---

## 2. Real-Time Metrics API (`/api/metrics`)

The dashboard serves a JSON metrics payload at `/api/metrics`. This endpoint can be queried by external monitoring systems (such as Prometheus or custom administrative control panels).

### JSON Response Structure
```json
{
  "cpu_percent": 12.45,
  "alloc_mb": 14.82,
  "sys_mb": 32.11,
  "queue_depth": 0,
  "goroutines": 18,
  "total_updates": 45120,
  "latency_ms": 115.4,
  "active_sessions": 324,
  "db_keys": 1205,
  "num_gc": 45,
  "shield_active": false
}
```

### Metrics Calculation Mechanics

#### A. Cross-Platform CPU Usage (`cpu_percent`)
GoBale calculates the active process CPU load dynamically without calling external system commands:
* **Linux:** Reads the process ticks directly from `/proc/self/stat` (`utime` + `stime`).
* **Windows:** Dynamically loads the `kernel32.dll` library at runtime and invokes the `GetProcessTimes` API using unsafe pointers to extract process thread execution times.
* **macOS / Others:** Falls back to `0` safely without crashing the server.

#### B. Network Latency (`latency_ms`)
GoBale calculates the network latency in nanoseconds (`NetLatencyNs`) for each request sent via the client's `BaseRequest`. The dashboard reads this atomic counter and displays the rolling network delay to Bale API servers in milliseconds.

#### C. Session & DB Keys
* `active_sessions`: Reads the total session record count distributed across the 32 sharded memory partitions.
* `db_keys`: Reads the total unique keys stored inside the in-memory WAL database.

---

## 3. Visual Dashboard Web Interface (`/`)

Exposing the root URL `/` serves an embedded, responsive HTML dashboard page.

```
                      GoBale Admin Dashboard (RTL)
┌────────────────────────────────────────────────────────────────────────┐
│  BALE MONITORING  [Status Badge: Normal / Shield Throttling Active]    │
├────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐         │
│ │  CPU Usage       │ │  Heap Memory     │ │  Sys Memory      │         │
│ │  1.42%           │ │  12.40 MB        │ │  32.11 MB        │         │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘         │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐         │
│ │  Queue Depth     │ │  Live Goroutines │ │  Total Updates   │         │
│ │  0 / 1000        │ │  18              │ │  45,120          │         │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘         │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐         │
│ │  Bale Latency    │ │  Active Sessions │ │  Database Keys   │         │
│ │  115.4 ms        │ │  324             │ │  1,205           │         │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘         │
└────────────────────────────────────────────────────────────────────────┘
```

* **Tailwind & Vazirmatn:** The UI is powered by Tailwind CSS and includes RTL styling with the Vazirmatn font, optimized for Persian administrators.
* **Auto-Polling:** The dashboard client-side script fetches `/api/metrics` and refreshes the metrics cards once every second using vanilla JavaScript `Fetch` APIs.
* **Responsive Layout:** Automatically scales across desktop, tablet, and mobile screens.

---

## 4. Integrated Performance Profiling (`pprof`)

Go's native profiling tool (`pprof`) is automatically mounted and exposed on the dashboard web server. This allows you to diagnose memory leaks, CPU bottlenecks, thread blocks, or scheduler behaviors under load.

### Exposed pprof Routes
* `/debug/pprof/`: Main pprof index interface.
* `/debug/pprof/profile`: Collects a CPU profile.
* `/debug/pprof/heap`: Inspects memory heap allocations.
* `/debug/pprof/goroutine`: Inspects stack traces of all active goroutines.
* `/debug/pprof/trace`: Captures execution trace data.

### Collecting Profiles via CLI
To analyze the bot's performance, run Go's profiling tool on your local machine pointing to the dashboard server port:

#### Collect CPU Profile
```bash
go tool pprof http://localhost:8080/debug/pprof/profile
```

#### Collect Memory Heap Profile
```bash
go tool pprof -alloc_objects http://localhost:8080/debug/pprof/heap
```

---

## 5. Production Security Recommendations

Because the dashboard exposes sensitive diagnostic details and `pprof` execution hooks, **never expose the dashboard port directly to the public internet** without access control. 

#### Recommended Patterns:
1. **Private Binding:** Keep the dashboard bound to `localhost:8080` on your cloud server and access it securely using SSH port forwarding:
   ```bash
   ssh -L 8080:localhost:8080 user@your-cloud-ip
   ```
2. **Reverse Proxy with Basic Authentication:** Configure Nginx as a reverse proxy in front of port `8080` and enforce HTTP Basic Authentication (`htpasswd`) to restrict unauthorized access [1].
