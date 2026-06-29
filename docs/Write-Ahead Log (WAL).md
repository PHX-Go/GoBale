# In-Memory WAL Database Engine

GoBale provides a custom-built, thread-safe local key-value store designed specifically for state persistence. It solves the performance-versus-durability trade-off of standard file databases by using a **Write-Ahead Log (WAL)** architecture backed by memory-first reads.

---

## 1. Architectural Design

```
                  ┌──────────────────────┐
                  │   Database Write     │ (Set / Del / Tx)
                  └──────────┬───────────┘
                             │
              ┌──────────────┴──────────────┐
              ▼                             ▼
   ┌────────────────────┐            ┌──────────────┐
   │ In-Memory Map      │            │  WAL Append  │ (Sequential OS Disk Write)
   │ (Instant O(1) Read)│            │  (.wal file) │
   └────────────────────┘            └──────┬───────┘
                                            ▼
                                     [Check threshold] (e.g. 200 writes)
                                            ▼
                                     ┌──────────────┐
                                     │ compactLoop  │ (Atomic background Snapshot)
                                     └──────┬───────┘
                                            ▼
                                     ┌──────────────┐
                                     │  data.gob    │ (Truncates .wal file)
                                     └──────────────┘
```

* **High-Speed Reads ($O(1)$):** Reads are served directly from an in-memory dictionary. The disk is never touched during read operations, ensuring lightning-fast performance under heavy request loads.
* **Append-Only Logging (WAL):** Write operations do not rewrite the entire database file (which causes high disk wear and CPU overhead). Instead, writes update the memory state instantly and append a tiny, sequential binary GOB entry to the `.wal` file.
* **Background Compactor & Atomic Snapshots:** A background loop monitors write counts. Once it exceeds the compaction threshold (default: 200 writes) or every 5 minutes, it compiles a full database snapshot into a `.tmp` file, performs an atomic OS rename to replace the `.gob` file, and truncates the WAL log file to zero.
* **Crash Recovery:** On startup, the engine reads the latest stable `.gob` snapshot and replays outstanding WAL transaction logs, restoring the exact state prior to any unexpected crash.

---

## 2. API Reference & Code Examples

All database operations are managed fluently via `bot.DB()` or `c.DB()` chains.

### Basic Reads, Writes, and Deletes

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

	// 1. Thread-safe write (updates memory + appends metadata to WAL log)
	_ = bot.DB().Set("user_status_88888", "premium_member").Go()

	// 2. Thread-safe read (served instantly from memory)
	val, ok := bot.DB().Get("user_status_88888").Go()
	if ok {
		log.Printf("User 88888 status: %v", val)
	}

	// 3. Thread-safe deletion
	_ = bot.DB().Del("user_status_88888").Go()
}
```

### Atomic Transactions (`.Tx`)
When updating interrelated keys or incrementing metrics, always use the `.Tx()` transaction method. It acquires a full lock on the in-memory map, executes your mutation closure, calculates differences, and flushes all changes to the WAL file securely in one pass.

```go
func HandleUserVisit(bot *gobale.Bot, userID int64) {
	// Execute an atomic transaction to prevent concurrent read-write races
	err := bot.DB().Tx(func(store map[string]any) {
		currentHits := 0
		if val, exists := store["total_hits"]; exists {
			if current, ok := val.(int); ok {
				currentHits = current
			}
		}
		
		// Mutate state safely inside the transaction closure
		store["total_hits"] = currentHits + 1
		store["last_visitor_id"] = userID
	}).Go()

	if err != nil {
		log.Printf("Database transaction failed: %v", err)
	}
}
```
