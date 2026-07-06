# Advanced Security & Threat Mitigation Suite

The GoBale Security Shield is a suite of defensive middlewares designed to protect your bot, groups, and users from malicious client-crashing payloads, Zalgo text, invisible character bombs, and typosquatting/phishing impersonation attempts. 

---

## 1. Core Security Components

### I. Multi-Dimensional Threat Shield (`bot.AntiCrash()`)
The `AntiCrash` engine provides a multi-layered Unicode analyzer:
* **Combining Marks Density (Global Density):** Legitimate Persian/Arabic texts rarely exceed 5% combining mark density. The shield calculates the ratio of combining marks to total runes. If the total combining marks count is greater than 3 and the ratio exceeds **10%**, the message is flagged.
* **Symbol Enclosing Limit:** Normal texts contain zero enclosing symbols (such as combining circles `⃝`). If a message contains more than **2** enclosing circle/square diacritics (Unicode range `U+20D0` to `U+20FF`), it is instantly flagged.
* **Foreign Script Quarantine:** Mongolian, Telugu, Bengali, Tamil, Kannada, Malayalam, and Sinhala scripts are commonly used in complex ligature rendering exploits. If a message contains more than **2** characters from these blocks, it is flagged immediately.
* **Bidi/Invisible Control limit:** Directional formatting and invisible markers (e.g., `\u200E`, `\u200F`, `\u202A`–`\u202E`, ZWJ, ZWNJ) are restricted to a maximum of **4** per message.
* **Low-Entropy Alternating Pattern Rule:** repititive sequences of alternating characters (e.g., `ᡃ⃝ᡃ⃝`) are mathematically intercepted. If a message length exceeds 12, and the unique characters (excluding spaces and punctuation) represent less than **20%** of the total characters, it is flagged.
* **Homoglyph & Typosquatting Phishing Shield:** Resolves visually identical confusable characters into standard ASCII skeletons, detects mixed-scripts, and decodes Punycode/IDN domains natively without external dependencies. 

### II. Proactive Group History Pruner (`bot.GroupHistoryPruner()`)
Bale/Telegram servers often suppress `edited_message` updates during Polling. To bypass this platform limitation in group chats, the `GroupHistoryPruner` automatically deletes the previous message of any non-admin user as soon as they send a new one. This ensures that no old messages remain active to be edited into client-crashing payloads.

### III. PV Clutter & Attack Shield (`bot.PVClutterShield()`)
To completely eliminate the ability of users to execute edit-based attacks inside direct messages (PV), the `PVClutterShield` immediately deletes the user's incoming message after the bot receives and processes it. This leaves no messages for the user to edit, resulting in a pristine, menu-only user experience.

---

## 2. Fluent API Reference

* `(*Bot).AntiCrash()`: Opens the fluent security configuration pipeline.
* `(*AntiCrashChain).ZalgoLimit(limit int)`: Configures maximum allowed consecutive combining marks (Default is 3).
* `(*AntiCrashChain).MaxRepeat(limit int)`: Blocks excessive runs of identical characters (Default is 5).
* `(*AntiCrashChain).MaxLength(limit int)`: Sets the maximum character limit allowed per message.
* `(*AntiCrashChain).Homoglyph(v bool)`: Enables or disables confusable skeleton analysis.
* `(*AntiCrashChain).Protect(words ...string)`: Safeguards specific keywords or brands. Passing the special keyword `"all"` automatically populates the shield with an exhaustive list of default high-risk words (e.g., `admin`, `support`, `paypal`).
* `(*AntiCrashChain).WarnEngine(engine *WarnEngine)`: Integrates with your bot's dynamic warning system to automatically trigger graduated punishments.
* `(*AntiCrashChain).OnViolation(fn func(c *Ctx, results []DetectionResult))`: Registers a custom callback to handle flagged security threats.

---

## 3. Configuration & Registration Example

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	_ = gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Workers(4).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

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
		// Prune group message history to prevent edit-obfuscation bypasses
		bot.GroupHistoryPruner(),
		// Instantly delete user inputs in DM to eliminate edit exploits
		bot.PVClutterShield(),
	)

	log.Println("Bot is running with Advanced Threat Shield...")
	bot.Run().Polling().Go()
}
```
