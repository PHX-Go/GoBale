package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"github.com/PHX-Go/GoBale"
	"github.com/PHX-Go/GoBale/cpu"
	"github.com/PHX-Go/GoBale/db"
	"github.com/PHX-Go/GoBale/session"
)

func Run(b *gobale.Bot, addr string, localDB *db.LocalDB) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		stats := b.GetMemoryStats()
		metrics := b.GetMetrics()
		queueDepth := len(b.GetWorkerChan()) 

		shieldActive := false
		if b.Shield != nil {
			shieldActive = b.Shield.IsActive()
		}

		activeSessions := 0
		if b.Sessions != nil {
			if store, ok := b.Sessions.(*session.GOBStore); ok {
				activeSessions = store.GetSessionsCount()
			}
		}

		dbKeys := 0
		if localDB != nil {
			dbKeys = localDB.GetKeysCount()
		}

		payload := map[string]any{
			"alloc_mb":        stats.AllocMegabytes,
			"sys_mb":          stats.SysMegabytes,
			"num_gc":          stats.NumGC,
			"total_updates":   metrics.TotalUpdates,
			"processed_msgs":  metrics.ProcessedMessages,
			"failed_reqs":     metrics.FailedRequests,
			"latency_ms":      float64(metrics.NetworkLatencyNs) / 1000000.0,
			"queue_depth":     queueDepth,
			"shield_active":   shieldActive,
			"goroutines":      runtime.NumGoroutine(),
			"active_sessions": activeSessions,
			"db_keys":         dbKeys,
			"cpu_percent":     cpu.GetProcessCPUUsage(),
		}

		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardHTML))
	})

	log.Printf("📊 [Dashboard] Server is running on http://localhost%s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  35 * time.Second, 
		WriteTimeout: 35 * time.Second,
	}
	return server.ListenAndServe()
}

const dashboardHTML = `
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

        <main class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
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
                    <span class="text-[11px] text-zinc-400 font-bold">تأخیر بله (Latency)</span>
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
        async function updateMetrics() {
            try {
                const res = await fetch('/api/metrics');
                const data = await res.json();

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
                
                const qVal = data.queue_depth;
                document.getElementById('queue-depth').innerHTML = qVal + ' <span class="text-[10px] font-normal text-zinc-500">/ 1000</span>';
                document.getElementById('queue-bar').style.width = Math.min((qVal / 1000) * 100, 100) + '%';

                document.getElementById('total-updates').innerText = data.total_updates.toLocaleString();
                document.getElementById('latency-ms').innerHTML = data.latency_ms.toFixed(1) + ' <span class="text-[10px] font-normal text-zinc-500">ms</span>';
                document.getElementById('num-gc').innerText = data.num_gc.toLocaleString();
                document.getElementById('goroutines').innerText = data.goroutines.toLocaleString();
                document.getElementById('active-sessions').innerText = data.active_sessions.toLocaleString();
                document.getElementById('db-keys').innerText = data.db_keys.toLocaleString();

                const shieldBadge = document.getElementById('shield-badge');
                if (data.shield_active) {
                    shieldBadge.className = 'px-2 py-0.5 rounded text-[10px] md:text-xs font-bold bg-red-950/40 text-red-400 border border-red-900/30 flex items-center gap-1';
                    shieldBadge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-red-500"></span></span> سپر دفاعی فعال';
                } else {
                    shieldBadge.className = 'px-2 py-0.5 rounded text-[10px] md:text-xs font-bold bg-emerald-950/40 text-emerald-400 border border-emerald-900/30 flex items-center gap-1';
                    shieldBadge.innerHTML = '<span class="relative flex h-1.5 w-1.5"><span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span></span> وضعیت عادی';
                }
            } catch (e) {
                console.error("Failed to fetch metrics", e);
            }
        }

        updateMetrics();
        setInterval(updateMetrics, 1000);
    </script>
</body>
</html>
`