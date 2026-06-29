# Sharded Session Storage (GOBStore)

GoBale manages user conversations and states using a highly optimized, concurrent-safe session engine called `GOBStore`. It solves the massive lock contention overhead associated with standard map-based session managers.

---

## 1. Architectural Design

In typical bot frameworks, session storage relies on a single map guarded by a single mutex lock. This creates a severe bottlenecks because every concurrent update has to wait to acquire that single lock.

```
                         Chat ID Update
                               │
                        [ Hash Chat ID ] (ID % 32)
                               │
         ┌─────────────────────┼─────────────────────┐
         ▼                     ▼                     ▼
   ┌───────────┐         ┌───────────┐         ┌───────────┐
   │ Shard 0   │         │ Shard 1   │         │ Shard 31  │ (32 Independent Shards)
   │ (Mutex 0) │         │ (Mutex 1) │         │ (Mutex 31)│
   └─────┬─────┘         └─────┬─────┘         └─────┬─────┘
         ▼                     ▼                     ▼
   ┌───────────┐         ┌───────────┐         ┌───────────┐
   │Session 100│         │Session 101│         │Session 131│ (Individual Lock/Read)
   └───────────┘         └───────────┘         └───────────┘
```

* **Memory Partitioning (Sharding):** GoBale hashes each incoming `chatID` and routes it to one of **32 independent shards**. Reading or locking a session only locks its specific shard, allowing other worker threads to concurrently write to other shards without waiting.
* **Automatic Inactivity Sweeper:** A background routine runs hourly to clean up inactive sessions. If a session has not been accessed for more than 24 hours, it is swept from memory to prevent RAM exhaustion.
* **Durable Deep-Copy GOB Saves:** When saving states to disk, the store locks each shard sequentially, performs a deep copy of each session's data map under a read lock, and encodes them securely. This protects against race conditions where a worker tries to write to a session while a background auto-save is running.

---

## 2. API Reference & Code Examples

Sessions are fetched using `c.Session()` inside the handler context or `bot.Sessions.Get(chatID)` from the main bot context.

### Standard Usage (FSM States and Custom Data)

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

	bot.On().Msg().Do(func(c *gobale.Ctx) {
		// Retrieve the sharded session belonging to this chat ID
		sess := c.Session()

		// 1. Manage Finite State Machine (FSM) states fluently
		// Set an active state:
		_, _ = sess.State("awaiting_email").Go()

		// Retrieve the active state:
		state, _ := sess.State().Go()
		log.Printf("Current FSM state: %s", state)

		// 2. Store custom keys/values inside the session data map
		// Set custom key-value:
		_, _ = sess.Data("signup_step", 1).Go()

		// Retrieve custom key-value:
		step, _ := sess.Data("signup_step").Go()
		log.Printf("Current signup step: %v", step)
	})
}
```

### Type-Safe Generics Helpers

GoBale provides global generic helper functions (`SessionGet` and `SessionSet`) to write and retrieve custom types from sessions safely without doing tedious interface assertions or type switches.

```go
func HandleUserSession(sess *gobale.Session) {
	// Store custom types safely inside the session
	gobale.SessionSet[string](sess, "user_email", "developer@example.com")
	gobale.SessionSet[int64](sess, "login_timestamp", 1700000000)

	// Retrieve typed values cleanly without doing manual interface assertions
	email, ok := gobale.SessionGet[string](sess, "user_email")
	if ok {
		log.Printf("Type-safe user email: %s", email)
	}

	timestamp, ok := gobale.SessionGet[int64](sess, "login_timestamp")
	if ok {
		log.Printf("Type-safe login timestamp: %d", timestamp)
	}
}
```
