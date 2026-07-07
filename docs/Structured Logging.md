# Structured Logging

GoBale features a highly optimized, structured logging engine built directly on top of Go's standard library `log/slog` package. It provides dual-destination support (simultaneous rotating file and console output), native Jalali calendar date formatting, and a lightweight, zero-allocation fluent dot-system API.

---

## 1. Fluent Builder Configuration

When constructing the bot instance, you can configure the central logging engine directly on the `BotBuilder` using fluent methods.

### `.LogText(path string, level ...slog.Level)`
Configures the logger to write standard key-value text records to the console and to an optional rotating log file.
* **`path`**: File destination path (e.g., `"data/bot.log"`). Leave as `""` to output to the console only.
* **`level`**: Minimum severity level to log (defaults to `slog.LevelInfo`).

### `.LogJSON(path string, level ...slog.Level)`
Configures the logger to write structured, indexable JSON records to the console and to an optional rotating log file. Useful for production analytics and system monitoring.
* **`path`**: File destination path. Leave as `""` for console-only JSON output.
* **`level`**: Minimum severity level to log (defaults to `slog.LevelInfo`).

### `.LogLadder(path string, level ...slog.Level)`
Configures the logger to format terminal console output into a human-readable, vertical ladder box-drawing layout (`┌`, `├`, `└`) while automatically translating Gregorian timestamps to native Jalali calendar dates (e.g. `1405/4/16`). If a file path is provided, standard structured records are simultaneously mirrored to the rotating file.
* **`path`**: File destination path. Leave as `""` for shamsi console-only output.
* **`level`**: Minimum severity level to log (defaults to `slog.LevelInfo`).

---

## 2. Fluent Log Event API

You can trigger structured logging events using the `.Log()` method available on both the global `Bot` pointer and the request execution `Ctx` pointer. 

### Event Severity
Initialize the log event by choosing the severity method:
* **`.Debug(format string, args ...any)`**
* **`.Info(format string, args ...any)`**
* **`.Warn(format string, args ...any)`**
* **`.Error(format string, args ...any)`**

### Chained Attributes
Chain highly optimized, typed structured attributes to the event:
* **`.Str(key, val string)`**: Appends a string attribute.
* **`.Int(key string, val int)`**: Appends an integer attribute.
* **`.Int64(key string, val int64)`**: Appends an int64 attribute.
* **`.Bool(key string, val bool)`**: Appends a boolean attribute.
* **`.Float(key string, val float64)`**: Appends a float64 attribute.
* **`.Any(key string, val any)`**: Appends any dynamic interface attribute.
* **`.Err(err error)`**: Automatically appends a structured error attribute with key `"error"` if the error is non-nil.
* **`.Group(name string, attrs ...slog.Attr)`**: Appends nested grouped attributes.
* **`.Context(ctx context.Context)`**: Registers a custom context to associate with the log entry.

### Terminal Method
Every log chain must terminate with the `.Go()` method to execute and flush the logging pipeline:
* **`.Go()`**

---

## 3. Complete Fluent Integration Example

Below is a complete, standalone program illustrating environment initialization, shamsi ladder console configuration, and fluent structured logging inside bot handlers.

```go
package main

import (
	"errors"
	"log"
	"log/slog"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// Load environment variables from local .env file
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize bot, admin, workers, and shamsi ladder logger in one single fluent chain
	bot, err := gobale.New(token).
		Admin(adminID).
		Workers(4).
		LogLadder("data/bot.log", slog.LevelInfo).
		Go()
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}

	// Register start command handler
	bot.On().Cmd("start").Do(func(c *gobale.Ctx) {
		// Log start action structurally
		c.Log().Info("Start command triggered").Go()

		_, _ = c.Send().Text("Welcome to the Bot!").Go()
	})

	// Register a simple message handler
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		text := c.Text()

		errDb := mockDatabaseCall(text)
		if errDb != nil {
			// Log database error structurally with error attributes
			c.Log().Error("Database transaction failed").
				Err(errDb).
				Str("input_value", text).
				Go()

			_, _ = c.Send().Text("An error occurred.").Go()
			return
		}

		// Log processed message structurally
		c.Log().Info("Message processed successfully").
			Int("chars_count", len([]rune(text))).
			Go()

		_, _ = c.Send().Text("Processed: " + text).Go()
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}

// mockDatabaseCall simulates a database transaction that fails on empty texts
func mockDatabaseCall(text string) error {
	if text == "" {
		return errors.New("empty text payload is invalid")
	}
	return nil
}
```
