# Anti-Crash & Phishing Threat Shield

The `AntiCrash` middleware is an advanced, multi-dimensional security shield designed to protect your bot, groups, and users from malicious client-crashing payloads, Zalgo text, invisible character bombs, and typosquatting/phishing impersonation attempts.

## Features

1. **Multi-Dimensional Zalgo Shield:** Monitors consecutive combining marks (Unicode category `Mn`, `Me`, `Mc`) alongside hardcoded ranges to bypass standard library limitations.
2. **Invisible/Bidi Control Bomb Protection:** Strictly limits the usage of bidirectional and invisible formatting marks (such as `\u200E`, `\u200F`, `\u202A`–`\u202E`) to a maximum of **4 characters** to completely neutralize "Black Dot" freeze attacks.
3. **Foreign Script Quarantine:** Automatically quarantines complex ligatures from foreign scripts (e.g., Mongolian, Telugu, Malayalam, Bengali) when combined with any combining diacritics.
4. **Low-Entropy Alternating Pattern Rule:** Tracks repetitive alternating sequences (e.g., `ᡃ⃝ᡃ⃝`) with a mathematical character diversity ratio (under 20% unique characters flags a threat).
5. **Stateful Homoglyph & Phishing Shield:** Resolves visually identical confusable characters into standard ASCII skeletons, detects mixed-scripts, and decodes Punycode/IDN domains natively without external dependencies.
6. **Brand Protection & Typosquatting Safeguard:** Computes Levenshtein edit-distances against registered sensitive keywords to block spoofing attempts (e.g., preventing `аdmin` with Cyrillic `а` from impersonating the actual `admin` keyword).

---

## API Reference

* `(*Bot).AntiCrash()`: Opens the fluent security configuration pipeline.
* `(*AntiCrashChain).ZalgoLimit(limit int)`: Configures maximum allowed consecutive combining marks (Default is 3).
* `(*AntiCrashChain).MaxRepeat(limit int)`: Blocks excessive runs of identical characters (Default is 5).
* `(*AntiCrashChain).MaxLength(limit int)`: Sets the maximum character limit allowed per message.
* `(*AntiCrashChain).Homoglyph(v bool)`: Enables or disables confusable skeleton analysis.
* `(*AntiCrashChain).Protect(words ...string)`: Safeguards specific keywords or brands. Passing the special keyword `"all"` automatically populates the shield with an exhaustive list of default high-risk words (e.g., `admin`, `support`, `paypal`).
* `(*AntiCrashChain).WarnEngine(engine *WarnEngine)`: Integrates with your bot's dynamic warning system to automatically trigger graduated punishments (mute, kick, ban).
* `(*AntiCrashChain).OnViolation(fn func(c *Ctx, results []DetectionResult))`: Registers a custom callback to handle flagged security threats.

---

## Configuration Example

```go
// Register the advanced Anti-Crash and Phishing Shield globally
bot.On().Use(bot.AntiCrash().
	// Enforce consecutive combining marks limit
	ZalgoLimit(3).
	// Block excessive character repetitions
	MaxRepeat(5).
	// Set maximum character limit to prevent client overflows
	MaxLength(2048).
	// Enable visually identical confusable detection
	Homoglyph(true).
	// Safeguard default high-risk keywords and selective custom domains
	Protect("all", "custom_brand.com", "channel_support").
	// Register custom security violation handler
	OnViolation(func(c *gobale.Ctx, results []gobale.DetectionResult) {
		// Delete the malicious message immediately
		_ = c.Del().Go()

		// Extract the exact reason behind the violation
		reason := results[0].Reason
		// Send a temporary 5-second warning to the user
		warn := fmt.Sprintf("⚠️ Dear %s, your message was filtered due to a security violation.\nReason: %s", c.Message.From.Mention(), reason)
		_, _ = c.Send().Text(warn).Temp(5 * time.Second).Go()

		// Halt the message processing pipeline immediately
		c.Abort()
	}).
	// Compile security rules and return middleware handler
	Go(),
)
```
