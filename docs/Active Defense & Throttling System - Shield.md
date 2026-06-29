# Active Defense & Throttling System (Shield)

GoBale implements a proactive active defense system to protect your bot from sudden traffic spikes, flooding, and denial-of-service (DoS) attempts. It constantly monitors queue depths and update rates, applying dynamic throttling to preserve system resources under high-concurrency conditions.

---

## Architectural Design

Under standard conditions, GoBale processes updates at its configured rate. However, extreme traffic spikes can saturate memory or exceed platform limits, leading to crashes.

```
                    [ Incoming Updates Stream ]
                                │
                                ▼
                       [ workerChan Queue ] (Capacity: 1000)
                                │
               ┌────────────────┴────────────────┐
               ▼                                 ▼
       [ Normal Traffic ]               [ Massive Spike ]
     (Queue < 100, UPS < 10)         (Queue > 800 or UPS > 150)
               │                                 │
               ▼                                 ▼
     ┌──────────────────┐               ┌──────────────────┐
     │  Standard Rate   │               │  Shield Active   │ (Throttles Rate Limiter)
     │ (30 reqs/second) │               │  (10 reqs/second)│
     └──────────────────┘               └──────────────────┘
```

* **Automated Sentinel Goroutine:** A background sentinel monitors the bot’s operation state every 10 seconds. It tracks the Updates-Per-Second (UPS) rate and the `workerChan` queue depth [GoBale_v3.txt].
* **Proactive Throttling (Shield Activation):** If the queue depth exceeds 800 pending updates or the update rate exceeds 150 UPS, GoBale dynamically activates `Shield` mode [GoBale_v3.txt]. When active, the bot's central rate limiter `rateLimit` is throttled down from `30 requests/second` to `10 requests/second` to stabilize resources and prevent memory overflows [GoBale_v3.txt].
* **Graceful Restoration:** Once the traffic subside, the queue depth drops below 100, and the update rate falls under 10 UPS, the sentinel automatically deactivates `Shield` mode, restoring standard rate-limiting metrics [GoBale_v3.txt].

---

## API Reference & Code Examples

The defense system can be queried or manually overridden from both the main `Bot` context and the handler `Ctx` context [GoBale_v3.txt].

### Querying and Overriding Defense States

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

	bot.On().Cmd("shield").Do(func(c *gobale.Ctx) {
		// Check if the automated defense shield is currently active
		isActive, err := c.Shield().IsActive().Go()
		if err != nil {
			log.Printf("Failed to check shield status: %v", err)
			return
		}

		if isActive {
			_, _ = c.Send().Text("🚨 Automated active defense is currently active!").Go()
			return
		}

		_, _ = c.Send().Text("🟢 Traffic levels are normal. Shield is inactive.").Go()
	})

	bot.On().Cmd("lockdown").Do(func(c *gobale.Ctx) {
		// Manually activate the throttling shield to limit incoming traffic rates
		err := c.Shield().Activate().Go()
		if err != nil {
			log.Printf("Failed to activate shield: %v", err)
			return
		}

		_, _ = c.Send().Text("🔒 Emergency lockdown activated. Throttling is now in place.").Go()
	})
}
```
