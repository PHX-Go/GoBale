# Group Protection & Antispam Middlewares

GoBale includes a suite of highly configurable group protection middlewares designed to mitigate spam, filter advertisements, detect malicious payload crashes, and stop automated self-bot raids inside group chats.

---

## Middlewares Reference

These middlewares are registered on routing groups or globally. They automatically inspect incoming updates, execute actions (such as deleting messages, muting users, or kicking bots), and abort the pipeline when violations are identified [GoBale_v3.txt].

* **`AntiSpam(limit, window, warnMsg...)`**: Tracks message frequency per user. If a user exceeds the limit within the timeframe, their messages are deleted, and a self-destroying warning is sent [GoBale_v3.txt].
* **`AntiLink(warnDuration, customMsg, customTLDs...)`**: Scans text for URLs and automatically deletes matching links from non-admin users [GoBale_v3.txt].
* **`AntiRepeat(warnDuration)`**: Detects and deletes duplicate identical messages sent sequentially by the same user within 1 minute [GoBale_v3.txt].
* **`AntiForward(warnDuration)`**: Automatically deletes forwarded messages sent by non-admin members [GoBale_v3.txt].
* **`AntiCaps(threshold, minLength, warnDuration)`**: Detects and deletes messages containing excessive uppercase letters (shouting) [GoBale_v3.txt].
* **`AntiCharLimit(limit, warnDuration)`**: Deletes messages exceeding the specified character limit (to block massive advertising banners) [GoBale_v3.txt].
* **`AntiNightMedia(startHour, endHour, warnDuration)`**: Restricts non-admin members from posting media files (photos, videos, voice notes, stickers, etc.) during specified night hours [GoBale_v3.txt].
* **`AntiSpamProfile(banOnMatch, bannedKeywords)`**: Inspects newly joined members' profiles (names/usernames) and bans them instantly on keyword matches [GoBale_v3.txt].
* **`AntiSelfBot(minInterval)`**: Tracks member join timestamps and bans automated user accounts (self-bots) that post faster than the allowed threshold [GoBale_v3.txt].
* **`MandatoryAddGuard(defaultLimit)`**: Restricts non-admin members from sending messages unless they have invited a minimum number of users to the group [GoBale_v3.txt].
* **`GroupLockGuard()`**: Standard gateway that deletes messages from muted/unverified users or when the group is locally locked in the database [GoBale_v3.txt].
* **`AntiRaid(limit, window)`**: Monitors join frequency; if joins exceed the limit, it automatically locks the group in the WAL database and alerts the chat [GoBale_v3.txt].
* **`AntiProfanity(warnDuration, bannedWords, customMsg...)`**: Automatically deletes messages containing specified banned words [GoBale_v3.txt].
* **`AdminsOnly()`**: Restricts execution of subsequent handlers to group administrators only [GoBale_v3.txt].
* **`AdminOnly(adminID, customMsg...)`**: Restricts execution of subsequent handlers to the global bot owner [GoBale_v3.txt].
* **`SuperGroupOnly(alert)`**: Restricts execution of subsequent handlers to supergroups only [GoBale_v3.txt].

---

## Architectural Example Pipeline

The following example demonstrates how to set up a secure group pipeline using GoBale's built-in antispam and protection middlewares:

```go
package main

import (
	"log"
	"time"

	"github.com/PHX-Go/GoBale"
)

func main() {
	_ = gobale.Env().Go()
	token := gobale.GetEnv[string]("BALE_TOKEN")

	bot, err := gobale.New(token).DryRun().Go()
	if err != nil {
		log.Fatalf("Failed to init bot: %v", err)
	}

	// Apply global anti-crash and panic recovery to the entire pipeline
	bot.On().Use(gobale.Recovery())
	bot.On().Use(gobale.AntiCrash())

	// Build a highly secured group pipeline for group chats
	securedGroup := bot.On().Group(
		gobale.AntiSpam(5, 3*time.Second),                       // Allow max 5 messages per 3 seconds
		gobale.AntiLink(5*time.Second, "No links allowed here!"), // Block and delete links
		gobale.AntiRepeat(5*time.Second),                       // Block duplicate identical texts
		gobale.GroupLockGuard(),                                 // Delete messages if group is locked or user is muted
	)

	// This command executes only if the message passes all secured group filters
	securedGroup.Cmd("rules").Do(func(c *gobale.Ctx) {
		_, _ = c.Send().
			Text("Group Rules:\n1. No spamming\n2. No advertising links\n3. Respect other members.").
			Go()
	})
}
```
