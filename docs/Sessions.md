# Sessions: Sharded GOB State Store & Type-Coercive Getters

The `Session` module in GoBale manages isolated, concurrent user states, conversational Finite State Machine (FSM) states, and key-value memories. It uses a sharded, thread-safe memory architecture backed by persistent GOB file storage [session.go].

---

## Architecture Overview

1. **Sharded Memory Isolation:** To minimize lock contention under high-concurrency workloads, the system splits session storage into 32 independent memory shards [session.go].
2. **GOB Persistence (`GOBStore`):** Sessions are periodically auto-saved to `gobale_sessions.gob` every 10 minutes (and flushed gracefully on shutdown) using deep-copied data transfer objects (DTOs) to prevent concurrent write panic conditions [session.go].
3. **Auto-Cleanup Background Loop:** A persistent hourly background task automatically purges idle sessions that have not been accessed for more than 24 hours [session.go].
4. **Type-Coercion Shield:** Reading raw interface values (`any`) via standard type-assertions is prone to runtime panic crashes if GOB decodes numeric values to unexpected sizes (e.g., decoding an integer as `int64` but asserting as `int`). The new typed getters dynamically coerce equivalent structures safely (e.g., converting a numeric string `"24"` or float `24.0` to the requested `int` or `bool` natively) to safeguard the application runtime [session.go].

---

## API Reference

### 1. State Transitions (FSM)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `State(val ...string)` | `*StateChain` | Initiates the conversational state transition. If an argument is provided, it updates the state value [session.go]. |
| `StateChain.Go()` | `(string, error)` | Finalizes the state modification transaction [session.go]. |

### 2. Transactional Key-Value Storage (Sets / Writes)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Data(key string, val ...any)` | `*DataChain` | Prepares a transactional write operation. If `val` is provided, it sets the value [session.go]. |
| `DataChain.Go()` | `(any, error)` | Commits the transactional write or retrieves the raw interface value [session.go]. |

### 3. Safe Type-Coercive Getters (Reads / Gets)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `String(key string, fallback ...string)` | `string` | Retrieves a string. Defaults to empty string `""` or optional fallback value [session.go]. |
| `Int(key string, fallback ...int)` | `int` | Retrieves an integer. Supports safe coercion of string, float, and int64 sizes [session.go]. |
| `Int64(key string, fallback ...int64)` | `int64` | Retrieves an int64. Supports safe coercion of string, float, and standard int sizes [session.go]. |
| `Bool(key string, fallback ...bool)` | `bool` | Retrieves a boolean. Coerces boolean strings (`"true"`/`"false"`) and non-zero integers [session.go]. |
| `Float64(key string, fallback ...float64)` | `float64` | Retrieves a float64. Supports coercion of string, float32, and all integer sizes [session.go]. |

---

## Practical Examples

The following examples demonstrate how to implement user-state tracking, rate counters, and localization configurations within handler contexts.

### Example 1: Multi-Step Conversational State Machine (FSM)

This scenario registers a command to trigger a registration conversation. It safely transitions through intermediate states and retrieves user-input parameters dynamically across multiple incoming message iterations.

```go
// Command to trigger the conversational FSM registration flow
bot.On().Cmd("register").Do(func(c *gobale.Ctx) {
	// Set the user's initial conversational state to "awaiting_age"
	_, _ = c.Session().State("awaiting_age").Go()
	
	_, _ = c.Send().Text("👋 Welcome! To register, please reply with your age (numbers only):").Go()
})

// Step 1: Receives the user age input, saves it, and transitions to Step 2
bot.On().State("awaiting_age").Do(func(c *gobale.Ctx) {
	ageInput := c.Text()

	// Write raw input string directly to GOB session
	_, _ = c.Session().Data("user_age", ageInput).Go()

	// Move user to the next state "awaiting_name"
	_, _ = c.Session().State("awaiting_name").Go()
	_, _ = c.Send().Text("📝 Age saved! Now, please reply with your full name:").Go()
})

// Step 2: Receives the user name, retrieves the saved age, and completes registration
bot.On().State("awaiting_name").Do(func(c *gobale.Ctx) {
	name := c.Text()

	// Retrieve age using safe type-coercion (parses string "18" to int 18 automatically)
	age := c.Session().Int("user_age", 0)

	if age < 18 {
		_, _ = c.Send().Text("❌ Sorry " + name + ", registration is only allowed for users aged 18 and older.").Go()
	} else {
		_, _ = c.Send().Text("🎉 Congratulations " + name + "! Your registration is complete.").Go()
	}

	// Reset the conversational state back to empty
	_, _ = c.Session().State("").Go()
})
```

---

### Example 2: Command Invocation Rate Counter

This scenario counts how many times a user invokes a computationally intensive reporting command, restricting access once they exceed their daily allowance.

```go
bot.On().Cmd("report").Do(func(c *gobale.Ctx) {
	// Retrieve previous call counts safely (defaults to 0 if key does not exist)
	calls := c.Session().Int("report_calls", 0)

	if calls >= 3 {
		_, _ = c.Send().Text("⚠️ Access denied: You have reached your daily limit of 3 reports.").Go()
		return
	}

	// Perform report calculations...

	// Increment and write the counter back to the session GOB store
	_, _ = c.Session().Data("report_calls", calls+1).Go()

	_, _ = c.Send().Text("📊 Report generated successfully! (Requests used today: " + strconv.Itoa(calls+1) + "/3)").Go()
})
```

---

### Example 3: Dual-Variable Bonus & Localization Checks

This scenario evaluates both a boolean flag (`claimed_today`) and a localized language preference string (`lang`) to decide whether to credit a user's daily bonus in their preferred language.

```go
bot.On().Cmd("daily_bonus").Do(func(c *gobale.Ctx) {
	// Retrieve language preference string (defaults to English "en" if empty)
	lang := c.Session().String("lang", "en")

	// Retrieve daily claim boolean flag (defaults to false if empty)
	hasClaimed := c.Session().Bool("claimed_today", false)

	if hasClaimed {
		msg := "❌ Error: You have already claimed your daily bonus today!"
		if lang == "fa" {
			msg = "❌ خطا: شما امروز هدیه روزانه خود را دریافت کرده‌اید!"
		}
		_, _ = c.Send().Text(msg).Go()
		return
	}

	// Process daily bonus transaction...

	// Write the claimed boolean flag to the session store
	_, _ = c.Session().Data("claimed_today", true).Go()

	msg := "✅ Success! Daily bonus credited to your account."
	if lang == "fa" {
		msg = "✅ تبریک! هدیه روزانه به حساب شما واریز شد."
	}
	_, _ = c.Send().Text(msg).Go()
})
```
