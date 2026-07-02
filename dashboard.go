package gobale

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/pprof"
	"runtime"
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

// Go fires the dashboard monitoring service and starts unified WebSocket streaming
func (d *DashChain) Go() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Legacy fallback API endpoint
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
		if db, ok := d.bot.dbInstance.(*Database); ok {
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

	// Unified WebSocket endpoint hosted directly on the same dashboard port (e.g. :8080/ws)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Upgrade connection using the native SocketServer upgrade engine
		d.bot.Socket().ServeHTTP(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlPage))
	})

	// Periodically stream real-time metrics over WebSocket to all active dashboard clients
	d.bot.Task().Every(1*time.Second, func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		activeS := 0
		if d.bot.Sessions != nil {
			activeS = d.bot.Sessions.GetSessionsCount()
		}
		dbKeys := 0
		if db, ok := d.bot.dbInstance.(*Database); ok {
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
		d.bot.Socket().BroadcastJSON("metrics", payload)
	})

	log.Printf("dashboard server is running on http://localhost%s", d.addr)
	server := &http.Server{
		Addr:         d.addr,
		Handler:      mux,
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 35 * time.Second,
	}
	d.bot.dashServer = server
	return server.ListenAndServe()
}

// htmlPage contains the embedded visual monitoring layout with resilient, vector-only WS client
const htmlPage = `
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>داشبورد ادمین ربات بله</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;500;700;800&display=swap');
        body { font-family: 'Vazirmatn', sans-serif; -webkit-tap-highlight-color: transparent; }
    </style>
</head>
<body class="bg-zinc-950 text-zinc-100 min-h-screen flex flex-col justify-between selection:bg-emerald-500 selection:text-zinc-950">
    <div class="max-w-5xl mx-auto w-full px-3 py-4 md:py-6 flex-grow">
        <header class="flex flex-row justify-between items-center border-b border-zinc-900 pb-3 mb-5 gap-2">
            <div>
                <h1 class="text-lg md:text-2xl font-extrabold text-emerald-400 tracking-tight flex items-center gap-1.5">
                    <span class="relative flex h-2.5 w-2.5">
                        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                        <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
                    </span>
                    مانیتورینگ بله
                </h1>
                <p class="text-[10px] md:text-xs text-zinc-500 mt-0.5">وضعیت آنلاین منابع سرور و همروندی</p>
            </div>
            <div id="shield-badge" class="px-2.5 py-1 rounded-md text-[10px] md:text-xs font-bold transition-all duration-300">
                در حال بارگذاری...
            </div>
        </header>

        <main class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">مصرف پردازنده (CPU)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m14 0h2M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2zM9 9h6v6H9V9z"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="cpu-percent" class="text-2xl font-black text-zinc-100">0.00%</p>
                    <div class="w-full bg-zinc-800 rounded-full h-1 mt-2 overflow-hidden">
                        <div id="cpu-bar" class="bg-emerald-500 h-1 rounded-full transition-all duration-500" style="width: 0%"></div>
                    </div>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">حافظه فعال (Heap)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="alloc-mb" class="text-2xl font-black text-zinc-100">0.00 <span class="text-xs font-normal text-zinc-500">MB</span></p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">حافظه رزرو (Sys)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="sys-mb" class="text-2xl font-black text-zinc-100">0.00 <span class="text-xs font-normal text-zinc-500">MB</span></p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">صف پردازش (Queue)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="queue-depth" class="text-2xl font-black text-zinc-100">0 <span class="text-xs font-normal text-zinc-500">/ 1000</span></p>
                    <div class="w-full bg-zinc-800 rounded-full h-1 mt-2 overflow-hidden">
                        <div id="queue-bar" class="bg-purple-500 h-1 rounded-full transition-all duration-500" style="width: 0%"></div>
                    </div>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">گوروتین‌های زنده</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="goroutines" class="text-2xl font-black text-cyan-400">0</p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">پیام‌های دریافتی</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="total-updates" class="text-2xl font-black text-emerald-400">0</p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">تأخیر شبکه (Latency)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="latency-ms" class="text-2xl font-black text-amber-400">0.00 <span class="text-xs font-normal text-zinc-500">ms</span></p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">نشست‌های فعال</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="active-sessions" class="text-2xl font-black text-zinc-100">0</p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">رکوردهای دیتابیس</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="db-keys" class="text-2xl font-black text-indigo-400">0</p>
                </div>
            </div>

            <div class="bg-zinc-900/40 border border-zinc-900 p-3 rounded-xl flex flex-col justify-between min-h-[95px]">
                <div class="flex justify-between items-start">
                    <span class="text-[11px] text-zinc-400 font-bold">دوره‌های زباله‌روب (GC)</span>
                    <svg class="w-4 h-4 text-zinc-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 00-1 1v3M4 7h16"></path></svg>
                </div>
                <div class="mt-2">
                    <p id="num-gc" class="text-2xl font-black text-blue-400">0</p>
                </div>
            </div>
        </main>
    </div>

    <script>
        // Resilient WebSocket client that automatically connects to the same port on /ws
        const wsUri = "ws://" + window.location.host + "/ws";
        let websocket;

        function initWebSocket() {
            websocket = new WebSocket(wsUri);
            
            websocket.onopen = function() {
                const badge = document.getElementById('shield-badge');
                badge.innerText = "متصل به WebSocket Stream";
                badge.className = "px-2.5 py-1 rounded-md text-[10px] md:text-xs font-bold transition-all duration-300 bg-emerald-950/40 text-emerald-400 border border-emerald-900/30 flex items-center gap-1.5";
                badge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span></span> متصل به WebSocket Stream';
            };

            websocket.onmessage = function(evt) {
                try {
                    const data = JSON.parse(evt.data);
                    if (data.action === "metrics") {
                        renderMetrics(data.payload);
                    }
                } catch (e) {
                    console.error("Failed to parse WS packet", e);
                }
            };

            websocket.onclose = function() {
                const badge = document.getElementById('shield-badge');
                badge.innerText = "قطع ارتباط! تلاش برای اتصال مجدد...";
                badge.className = "px-2.5 py-1 rounded-md text-[10px] md:text-xs font-bold transition-all duration-300 bg-red-950/40 text-red-400 border border-red-900/30 flex items-center gap-1.5";
                badge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-red-500"></span></span> قطع ارتباط! تلاش برای اتصال مجدد...';
                
                // Resilient auto-reconnect loop
                setTimeout(initWebSocket, 2000);
            };

            websocket.onerror = function(err) {
                websocket.close();
            };
        }

        function renderMetrics(data) {
            const cpuVal = data.cpu_percent;
            document.getElementById('cpu-percent').innerText = cpuVal.toFixed(2) + '%';
            const cpuBar = document.getElementById('cpu-bar');
            cpuBar.style.width = Math.min(cpuVal, 100) + '%';
            
            if (cpuVal > 80) {
                cpuBar.className = 'bg-red-500 h-1 rounded-full transition-all duration-500';
            } else if (cpuVal > 40) {
                cpuBar.className = 'bg-amber-500 h-1 rounded-full transition-all duration-500';
            } else {
                cpuBar.className = 'bg-emerald-500 h-1 rounded-full transition-all duration-500';
            }

            document.getElementById('alloc-mb').innerHTML = data.alloc_mb.toFixed(2) + ' <span class="text-[10px] font-normal text-zinc-500">MB</span>';
            document.getElementById('sys-mb').innerHTML = data.sys_mb.toFixed(2) + ' <span class="text-[10px] font-normal text-zinc-500">MB</span>';
            document.getElementById('goroutines').innerText = data.goroutines.toLocaleString();
            
            const latencyVal = data.latency_ms;
            if (latencyVal > 0) {
                document.getElementById('latency-ms').innerHTML = latencyVal.toFixed(1) + ' <span class="text-[10px] font-normal text-zinc-500">ms</span>';
            } else {
                document.getElementById('latency-ms').innerHTML = '<span class="text-xs font-normal text-zinc-500">در انتظار درخواست...</span>';
            }
            
            document.getElementById('total-updates').innerText = data.total_updates.toLocaleString();
            document.getElementById('active-sessions').innerText = data.active_sessions.toLocaleString();
            document.getElementById('db-keys').innerText = data.db_keys.toLocaleString();

            const shieldBadge = document.getElementById('shield-badge');
            // Check shield active status dynamically and update badge with pure vectors
            if (data.shield_active === true) {
                shieldBadge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-red-500"></span></span>  سپر دفاعی فعال';
                shieldBadge.className = 'px-2.5 py-1 rounded text-[10px] md:text-xs font-bold bg-red-950/40 text-red-400 border border-red-900/30 flex items-center gap-1.5';
            } else {
                shieldBadge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span></span> متصل به WebSocket Stream';
                shieldBadge.className = 'px-2.5 py-1 rounded-md text-[10px] md:text-xs font-bold transition-all duration-300 bg-emerald-950/40 text-emerald-400 border border-emerald-900/30 flex items-center gap-1.5';
            }
        }

        initWebSocket();
    </script>
</body>
</html>
`
