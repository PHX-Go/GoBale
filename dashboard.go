package gobale

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// DashChain manages web monitoring configurations portably
type DashChain struct {
	bot  *Bot
	addr string
}

// Dash opens the visual dashboard configuration dot system
func (b *Bot) Dash() *DashChain {
	return &DashChain{
		bot:  b,
		addr: ":8080",
	}
}

// Addr registers custom dashboard interface listening address
func (d *DashChain) Addr(a string) *DashChain {
	d.addr = a
	return d
}

// openBrowser opens the specified URL in the default browser of the OS cross-platformly
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported operating system")
	}
	if err != nil {
		log.Printf("[Browser Launch Error] Failed to auto-open dashboard: %v", err)
	} else {
		log.Printf("[Browser Launch Success] Auto-opened default browser at %s", url)
	}
}

// Go fires the dashboard monitoring service asynchronously and serves metrics via a REST API
func (d *DashChain) Go() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// API endpoint to serve live metrics dynamically via REST polling [3]
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		activeS := 0
		if d.bot.Sessions != nil {
			activeS = d.bot.Sessions.GetSessionsCount()
		}
		dbKeys := 0
		if db, ok := d.bot.dbInstance.(*Database); ok && db != nil {
			db.mu.RLock()
			dbKeys = len(db.store)
			db.mu.RUnlock()
		}
		activeShield, _ := d.bot.Shield().IsActive().Go()
		payload := map[string]any{
			"cpu_percent":     d.bot.GetCPU(),
			"alloc_mb":        float64(m.Alloc) / (1024 * 1024),
			"sys_mb":          float64(m.Sys) / (1024 * 1024),
			"queue_depth":     len(d.bot.workerChan),
			"goroutines":      runtime.NumGoroutine(),
			"total_updates":   atomic.LoadUint64(&d.bot.totalUpdates),
			"latency_ms":      float64(atomic.LoadInt64(&d.bot.Client.NetLatencyNs)) / 1000000.0,
			"active_sessions": activeS,
			"db_keys":         dbKeys,
			"num_gc":          m.NumGC,
			"shield_active":   activeShield,
		}
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlPage))
	})

	// Async browser auto-opener right before starting the blocking server
	go func() {
		time.Sleep(500 * time.Millisecond)
		port := d.addr
		if strings.HasPrefix(port, ":") {
			port = "http://localhost" + port
		} else if !strings.HasPrefix(port, "http://") && !strings.HasPrefix(port, "https://") {
			port = "http://" + port
		}
		openBrowser(port)
	}()

	log.Printf("dashboard server is running on http://localhost%s", d.addr)
	server := &http.Server{
		Addr:    d.addr,
		Handler: mux,
	}
	d.bot.dashServer = server

	// Fire the HTTP/HTTPS server asynchronously in background (non-blocking for caller)
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("[Dashboard Server Error] %v", err)
		}
	}()

	return nil
}

// htmlPage contains the embedded visual monitoring layout with zero external network dependencies and native AJAX polling
const htmlPage = `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>داشبورد ادمین ربات بله</title>
    <style>
        /* Zero external dependencies. Replaced Tailwind CDN and Google Fonts with native system font stacks for instant render */
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, Tahoma, sans-serif;
            background-color: #09090b;
            color: #f4f4f5;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: space-between;
            line-height: 1.5;
        }
        .container {
            max-width: 1024px;
            margin: 0 auto;
            width: 100%;
            padding: 16px;
            flex-grow: 1;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 1px solid #27272a;
            padding-bottom: 12px;
            margin-bottom: 20px;
            gap: 8px;
        }
        .title {
            font-size: 20px;
            font-weight: 800;
            color: #34d399;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .subtitle {
            font-size: 11px;
            color: #71717a;
            margin-top: 2px;
        }
        .badge {
            padding: 4px 10px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 700;
            border: 1px solid rgba(16, 185, 129, 0.3);
            background-color: rgba(16, 185, 129, 0.1);
            color: #34d399;
            display: flex;
            align-items: center;
            gap: 6px;
        }
        .badge-red {
            border-color: rgba(239, 68, 68, 0.3);
            background-color: rgba(239, 68, 68, 0.1);
            color: #f87171;
        }
        /* Pulsating dot vector */
        .dot {
            position: relative;
            display: flex;
            height: 6px;
            width: 6px;
        }
        .dot::before {
            content: '';
            animation: ping 1s cubic-bezier(0, 0, 0.2, 1) infinite;
            position: absolute;
            display: inline-flex;
            height: 100%;
            width: 100%;
            border-radius: 50%;
            background-color: currentColor;
            opacity: 0.75;
        }
        .dot::after {
            content: '';
            position: relative;
            display: inline-flex;
            border-radius: 50%;
            height: 6px;
            width: 6px;
            background-color: currentColor;
        }
        @keyframes ping {
            75%, 100% { transform: scale(3); opacity: 0; }
        }
        /* Responsive Grid layout */
        main {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 12px;
        }
        @media (max-width: 480px) {
            main {
                grid-template-columns: repeat(2, 1fr);
            }
        }
        /* Card design matching original look */
        .card {
            background-color: rgba(24, 24, 27, 0.4);
            border: 1px solid #18181b;
            padding: 12px;
            border-radius: 12px;
            display: flex;
            flex-direction: column;
            justify-content: space-between;
            min-height: 95px;
        }
        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: start;
            color: #a1a1aa;
            font-size: 11px;
            font-weight: 700;
        }
        .card-body {
            margin-top: 8px;
        }
        .card-value {
            font-size: 20px;
            font-weight: 900;
            color: #f4f4f5;
        }
        .unit {
            font-size: 11px;
            font-weight: 400;
            color: #52525b;
        }
        .cyan-text { color: #22d3ee; }
        .emerald-text { color: #34d399; }
        .amber-text { color: #fbbf24; }
        .indigo-text { color: #818cf8; }
        .blue-text { color: #60a5fa; }
        /* Progress Bar */
        .progress-container {
            width: 100%;
            background-color: #27272a;
            border-radius: 9999px;
            height: 4px;
            margin-top: 8px;
            overflow: hidden;
        }
        .progress-bar {
            height: 100%;
            border-radius: 9999px;
            transition: width 0.5s ease;
        }
        .bg-emerald { background-color: #10b981; }
        .bg-amber { background-color: #f59e0b; }
        .bg-red { background-color: #ef4444; }
        .bg-purple { background-color: #a855f7; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1 class="title">
                    <span class="dot" style="color: #10b981;"></span>
                    مانیتورینگ بله
                </h1>
                <p class="subtitle">وضعیت آنلاین منابع سرور و همروندی</p>
            </div>
            <div id="shield-badge" class="badge">در حال بارگذاری...</div>
        </header>

        <main>
            <!-- CPU Card -->
            <div class="card">
                <div class="card-header"><span>مصرف پردازنده (CPU)</span></div>
                <div class="card-body">
                    <p id="cpu-percent" class="card-value">0.00%</p>
                    <div class="progress-container">
                        <div id="cpu-bar" class="progress-bar bg-emerald" style="width: 0%"></div>
                    </div>
                </div>
            </div>

            <!-- Heap Memory Card -->
            <div class="card">
                <div class="card-header"><span>حافظه فعال (Heap)</span></div>
                <div class="card-body">
                    <p id="alloc-mb" class="card-value">0.00 <span class="unit">MB</span></p>
                </div>
            </div>

            <!-- Reserved Memory Card -->
            <div class="card">
                <div class="card-header"><span>حافظه رزرو (Sys)</span></div>
                <div class="card-body">
                    <p id="sys-mb" class="card-value">0.00 <span class="unit">MB</span></p>
                </div>
            </div>

            <!-- Queue Card -->
            <div class="card">
                <div class="card-header"><span>صف پردازش (Queue)</span></div>
                <div class="card-body">
                    <p id="queue-depth" class="card-value">0 <span class="unit">/ 1000</span></p>
                    <div class="progress-container">
                        <div id="queue-bar" class="progress-bar bg-purple" style="width: 0%"></div>
                    </div>
                </div>
            </div>

            <!-- Goroutines Card -->
            <div class="card">
                <div class="card-header"><span>گوروتین‌های زنده</span></div>
                <div class="card-body">
                    <p id="goroutines" class="card-value cyan-text">0</p>
                </div>
            </div>

            <!-- Updates Card -->
            <div class="card">
                <div class="card-header"><span>پیام‌های دریافتی</span></div>
                <div class="card-body">
                    <p id="total-updates" class="card-value emerald-text">0</p>
                </div>
            </div>

            <!-- Latency Card -->
            <div class="card">
                <div class="card-header"><span>تأخیر شبکه (Latency)</span></div>
                <div class="card-body">
                    <p id="latency-ms" class="card-value amber-text">0.00 <span class="unit">ms</span></p>
                </div>
            </div>

            <!-- Sessions Card -->
            <div class="card">
                <div class="card-header"><span>نشست‌های فعال</span></div>
                <div class="card-body">
                    <p id="active-sessions" class="card-value">0</p>
                </div>
            </div>

            <!-- DB Keys Card -->
            <div class="card">
                <div class="card-header"><span>رکوردهای دیتابیس</span></div>
                <div class="card-body">
                    <p id="db-keys" class="card-value indigo-text">0</p>
                </div>
            </div>

            <!-- GC Card -->
            <div class="card">
                <div class="card-header"><span>دوره‌های زباله‌روب (GC)</span></div>
                <div class="card-body">
                    <p id="num-gc" class="card-value blue-text">0</p>
                </div>
            </div>
        </main>
    </div>

    <script {nonce}>
        // Highly resilient local AJAX Polling mechanism to replace WebSockets completely [3]
        async function fetchMetrics() {
            try {
                const response = await fetch('/api/metrics');
                if (!response.ok) throw new Error('API response error');
                const data = await response.json();
                renderMetrics(data);
                
                const badge = document.getElementById('shield-badge');
                badge.innerText = "متصل به API";
                badge.className = "badge";
                badge.innerHTML = '<span class="dot" style="color: #10b981;"></span> متصل به API';
            } catch (error) {
                console.error("Failed to fetch metrics", error);
                const badge = document.getElementById('shield-badge');
                badge.innerText = "خطا در ارتباط! تلاش مجدد...";
                badge.className = "badge badge-red";
                badge.innerHTML = '<span class="dot" style="color: #ef4444;"></span> خطا در ارتباط! تلاش مجدد...';
            }
        }

        function renderMetrics(data) {
            const cpuVal = data.cpu_percent;
            document.getElementById('cpu-percent').innerText = cpuVal.toFixed(2) + '%';
            const cpuBar = document.getElementById('cpu-bar');
            cpuBar.style.width = Math.min(cpuVal, 100) + '%';

            if (cpuVal > 80) {
                cpuBar.className = 'progress-bar bg-red';
            } else if (cpuVal > 40) {
                cpuBar.className = 'progress-bar bg-amber';
            } else {
                cpuBar.className = 'progress-bar bg-emerald';
            }

            document.getElementById('alloc-mb').innerHTML = data.alloc_mb.toFixed(2) + ' <span class="unit">MB</span>';
            document.getElementById('sys-mb').innerHTML = data.sys_mb.toFixed(2) + ' <span class="unit">MB</span>';
            document.getElementById('goroutines').innerText = data.goroutines.toLocaleString();

            const queueVal = data.queue_depth;
            document.getElementById('queue-depth').innerHTML = queueVal.toLocaleString() + ' <span class="unit">/ 1000</span>';
            const queueBar = document.getElementById('queue-bar');
            queueBar.style.width = Math.min(queueVal / 10, 100) + '%';
            if (queueVal > 800) {
                queueBar.className = 'progress-bar bg-red';
            } else if (queueVal > 300) {
                queueBar.className = 'progress-bar bg-amber';
            } else {
                queueBar.className = 'progress-bar bg-purple';
            }

            const latencyVal = data.latency_ms;
            if (latencyVal > 0) {
                document.getElementById('latency-ms').innerHTML = latencyVal.toFixed(1) + ' <span class="unit">ms</span>';
            } else {
                document.getElementById('latency-ms').innerHTML = '<span class="unit">در انتظار درخواست...</span>';
            }

            document.getElementById('total-updates').innerText = data.total_updates.toLocaleString();
            document.getElementById('active-sessions').innerText = data.active_sessions.toLocaleString();
            document.getElementById('db-keys').innerText = data.db_keys.toLocaleString();
            document.getElementById('num-gc').innerText = data.num_gc.toLocaleString();

            const shieldBadge = document.getElementById('shield-badge');
            // Check shield active status dynamically and update badge
            if (data.shield_active === true) {
                shieldBadge.innerHTML = '<span class="dot" style="color: #ef4444;"></span>  سپر دفاعی فعال';
                shieldBadge.className = 'badge badge-red';
            } else {
                shieldBadge.innerHTML = '<span class="dot" style="color: #10b981;"></span> متصل به API';
                shieldBadge.className = 'badge';
            }
        }

        // Start dynamic REST polling natively every 1 second [3]
        fetchMetrics();
        setInterval(fetchMetrics, 1000);
    </script>
</body>
</html>
`
