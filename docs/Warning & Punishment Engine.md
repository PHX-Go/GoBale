# Integrated Warning & Punishment Engine

The **Warning & Punishment Engine** (implemented via `warn_engine.go` with native integrations inside `middleware.go`) is a highly modular, state-persisted subsystem designed for group moderation. All native punitive middlewares in the framework accept the `WarnEngine` as an optional first parameter. If provided, they automatically trigger graduated punishments (warnings, mutes, kicks, bans) tracked in the GOB WAL database; if `nil`, they seamlessly fall back to traditional temporary static warning alerts.

---

## 1. Core Architectural Capabilities

1. **Pluggable Middleware Integration:** Every native punitive middleware in `middleware.go` (e.g., `AntiLink`, `AntiSpam`, `AntiProfanity`) accepts `engine *WarnEngine` as its first parameter. This architecture preserves the **SOLID Single Responsibility Principle** by letting middlewares handle *violation detection* while the `WarnEngine` handles *punishment execution and state tracking*.
2. **Backward Compatibility:** Passing `nil` as the first argument to any upgraded middleware reverts its behavior to traditional static, self-destroying warnings, preventing any breaking changes in existing codebases.
3. **Hierarchical Step Builders:** Simplifies configuration by providing concise, out-of-the-box constructors for different punishment levels: `gobale.Warn()`, `gobale.Mute()`, `gobale.Kick()`, and `gobale.Ban()`.
4. **Warning Cooldown (Auto-Expire):** Allows configuring a cooldown duration. If a user remains well-behaved, their warning count automatically decrements by `1` after the configured duration (e.g., 24 hours), managed by safe background tasks.
5. **Timestamped History Logger:** Appends and persists the exact reason and time of each infraction (`warn_reasons:<chatID>:<userID>`) in the GOB WAL database, allowing admins to inspect details later.
6. **One-Liner Command Auto-Registry (`.AutoCommands()`):** Automatically boots, configures, and registers group administrative commands:
   * `/warns` (Replying on a user displays their exact warning counts and full timestamped violation history; sending normally displays the sender's own warnings).
   * `/warn [reason]` (Admins manually warn the replied user with a custom reason).
   * `/unwarn` (Admins decrement the replied user's warnings by 1 and clean up database keys if they reach 0).
7. **Chat-Cleaning Self-Destruction:** All admin-initiated commands (`/warn`, `/unwarn`, `/warns`) automatically delete the incoming command message (`c.Del().Go()`), and their text responses automatically delete themselves using the fluent `.Temp(...)` API after a short period.

---

## 2. Integrated Middleware Signatures

The following native middlewares in `middleware.go` have been upgraded to support the optional `*WarnEngine` integration:

```go
// AntiLink checks for web links and warns/punishes users dynamically
gobale.AntiLink(engine *WarnEngine, warnDuration time.Duration, customMsg string, customTLDs ...string)

// AntiSpam prevents message floods and warns/punishes users dynamically
gobale.AntiSpam(engine *WarnEngine, limit int, window time.Duration, warnMsg ...string)

// AntiProfanity filters banned words and warns/punishes users dynamically
gobale.AntiProfanity(engine *WarnEngine, warnDuration time.Duration, bannedWords []string, customMsg ...string)

// AntiForward restricts forwarded messages and warns/punishes users dynamically
gobale.AntiForward(engine *WarnEngine, warnDuration time.Duration, customMsg ...string)

// AntiNight restricts night hours messaging and warns/punishes users dynamically
gobale.AntiNight(engine *WarnEngine, startHour, endHour int, warnDuration time.Duration, customMsg ...string)

// AntiCaps restricts consecutive uppercase English letters and warns/punishes users dynamically
gobale.AntiCaps(engine *WarnEngine, thresholdPercent float64, minLength int, warnDuration time.Duration)

// AntiMedia restricts specific media types and warns/punishes users dynamically
gobale.AntiMedia(engine *WarnEngine, warnDuration time.Duration, blockedTypes ...MediaType)

// AntiRepeat restricts sequential identical messages and warns/punishes users dynamically
gobale.AntiRepeat(engine *WarnEngine, warnDuration time.Duration, customMsg ...string)

// MandatoryAddGuard restricts chatting until a minimum number of users are invited
gobale.MandatoryAddGuard(engine *WarnEngine, defaultLimit int)
```

---

## 3. Fluent API Reference (`WarnEngine`)

### `bot.Warns()`
* Initiates the fluent `WarnEngine` configuration pipeline.

### Fluent Methods on `WarnEngine`:
* `.Limit(n int)`: Sets the maximum warning threshold before final punishment is executed (defaults to `3`).
* `.Cooldown(d time.Duration)`: Optional. Configures the warning expiration cooldown duration.
* `.On(count int, step PunishStep)`: Maps a warning count directly to a custom `PunishStep` (e.g., `gobale.Warn()`, `gobale.Mute()`).
* `.OnFinal(step PunishStep)`: Sets the ultimate punishment when warning limits are exceeded.
* `.AutoCommands()`: Enables automatic registration of self-destructing `/warn`, `/unwarn`, and `/warns` commands.
* `.Build()`: Finalizes the WarnEngine configuration and registers the commands if `.AutoCommands()` was called.
* `.Warn(c *Ctx, reason string) error`: Main trigger method. Increments the user's warning count in GOB DB, appends the reason with a timestamp, executes the corresponding stepped punishment, and schedules cooldowns.

---

### 3.1 Shorthand Step Constructors:
* `gobale.Warn(msg)`: Sends a temporary warning message.
* `gobale.Mute(duration, msg)`: Temporarily restricts write permissions for the specified duration.
* `gobale.Kick(msg)`: Evicts the user from the group (unbans immediately).
* `gobale.Ban(msg)`: Blocks the user from the group permanently.

---

## 4. Production Implementation Example

The following `main.go` code demonstrates configuring the `WarnEngine` and seamlessly passing it to your native framework middlewares (`AntiLink` and `AntiSpam`):

```go
package main

import (
	"log"
	"time"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Gzip().Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// 1. Configure the advanced WarnEngine
	warnEngine := bot.Warns().
		Limit(3).                 // Max 3 warnings allowed before Ban
		Cooldown(24 * time.Hour). // Decrement 1 warning every 24 hours of good behavior
		On(1, gobale.Warn("⚠️ کاربر {name} عزیز، شما اخطار اول را دریافت کردید.\nعلت: {reason}\n📊 اخطارها: {count}/{max}")).
		On(2, gobale.Mute(2*time.Hour, "🔇 کاربر {name} به دلیل دریافت اخطار دوم به مدت ۲ ساعت بی‌صدا شد.\nعلت خطای دوم: {reason}")).
		OnFinal(gobale.Ban("🚫 کاربر {name} به دلیل دریافت اخطار نهایی ({count}/{max}) از گروه مسدود شد.\nآخرین خطا: {reason}")).
		AutoCommands(). // Instantly boots /warn, /unwarn, and /warns automatically
		Build()

	// 2. Register native middlewares and connect them directly to the WarnEngine!
	bot.On().Use(
		// Pass "warnEngine" to enable stepped warnings, OR pass "nil" to use the old static warnings!
		gobale.AntiLink(warnEngine, 5*time.Second, "تبلیغات غیرمجاز در گروه"),
		gobale.AntiSpam(warnEngine, 5, 2*time.Second, "ارسال پیام‌های متوالی و اسپم"),
	)

	log.Println("Bot is running with Integrated WarnEngine...")
	bot.Run().Polling().Go()
}
```
