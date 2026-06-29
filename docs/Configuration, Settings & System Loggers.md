# Configuration, Settings & System Loggers

GoBale provides robust systems to manage environment variables, persist runtime toggle variables thread-safely via memory-to-disk mapping, and handle safe system logging with automated size-based file rotation.

---

## Dynamic Settings Registry (`SettingsChain`)

The dynamic settings registry allows mapping configuration keys directly to global boolean pointers (`*bool`). It utilizes a dedicated settings database (`gobale_settings.gob`) to persist changes across restarts.

* **Memory-to-Disk Syncing:** Registering a setting immediately initializes the global pointer to its last persisted database value.
* **Atomic Toggling:** Toggling a configuration atomically flips the mapped boolean pointer value in memory and updates the local settings database.

```go
package main

import (
	"log"

	"github.com/PHX-Go/GoBale"
)

// Declare a global configuration pointer
var MaintenanceMode bool

func InitSettings(bot *gobale.Bot) {
	// Register the setting. It automatically loads its last saved state from disk.
	bot.Settings().Register("maintenance", "Enable Maintenance Mode", &MaintenanceMode)

	log.Printf("Loaded Maintenance Mode status: %t", MaintenanceMode)
}

func ToggleSettings(bot *gobale.Bot) {
	// Toggle the setting. It atomically flips the pointer and saves the change to disk.
	err := bot.Settings().Toggle("maintenance").Go()
	if err != nil {
		log.Printf("Failed to toggle setting: %v", err)
		return
	}

	log.Printf("New Maintenance Mode status: %t", MaintenanceMode)
}
```

---

## Environment Variable Parser (`gobale.Env`)

GoBale includes a native, type-safe environment variable loader and parser. It eliminates the need for manual type assertions, supporting strings, integers, booleans, and durations.

### Supported Type Formats:
* `GetEnv[string](key)`: Retrieves raw string variable.
* `GetEnv[int64](key)`: Parses variable to 64-bit integer.
* `GetEnv[bool](key)`: Parses variable to boolean value.
* `GetEnv[time.Duration](key)`: Parses duration strings (supporting days `d` and weeks `w` formats natively).

```go
func LoadConfig() {
	// Load environment variables from default .env file
	_ = gobale.Env().Path(".env").Go()

	// Safely retrieve parsed configurations natively
	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")
	sessionTTL := gobale.GetEnv[time.Duration]("SESSION_TTL") // e.g., "7d" or "24h"

	log.Printf("Token: %s, Admin: %d, TTL: %v", token, adminID, sessionTTL)
}
```

---

## Size-Based Rotating Logger (`NewLogger`)

GoBale's logger is thread-safe and includes a size-based automatic file rotation mechanism to prevent disk space exhaustion on production servers.

* **Size-Based Trigger:** Rotates the active log once the file size exceeds `maxSize` (configured in megabytes).
* **Backup Management:** Renames old log files sequentially (e.g., `.1`, `.2`) up to the maximum `backups` limit and deletes the oldest copies.

```go
func SetupLog() {
	// Initialize a self-rotating log engine
	logger := gobale.NewLogger(gobale.LevelInfo, "bot.log", true).
		MaxSize(10). // Rotate active file once it reaches 10 Megabytes
		Backups(3)   // Keep maximum 3 backup files on disk

	logger.Log(gobale.LevelInfo, "[Boot]", "Log engine initialized.", nil)
}
```
