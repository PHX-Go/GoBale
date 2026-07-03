package gobale

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PunishmentType defines the severity and type of restriction applied to a user
type PunishmentType string

const (
	PunishWarn PunishmentType = "warn" // Sends a self-destroying warning alert
	PunishMute PunishmentType = "mute" // Temporarily restricts writing permissions
	PunishKick PunishmentType = "kick" // Evicts the user from the chat group
	PunishBan  PunishmentType = "ban"  // Blocks the user from the chat group permanently
)

// PunishStep represents a graduation warning step configuration
type PunishStep struct {
	Type     PunishmentType
	Duration time.Duration // Duration used for temporary mute/ban restrictions
	Message  string        // Custom Farsi message template for this step
}

// Warn creates a simple warning alert step
func Warn(messageTemplate string) PunishStep {
	return PunishStep{Type: PunishWarn, Message: messageTemplate}
}

// Mute creates a temporary mute step
func Mute(duration time.Duration, messageTemplate string) PunishStep {
	return PunishStep{Type: PunishMute, Duration: duration, Message: messageTemplate}
}

// Kick creates a group eviction step (kicks and immediately unbans)
func Kick(messageTemplate string) PunishStep {
	return PunishStep{Type: PunishKick, Message: messageTemplate}
}

// Ban creates a permanent group block step
func Ban(messageTemplate string) PunishStep {
	return PunishStep{Type: PunishBan, Message: messageTemplate}
}

// WarnEngine coordinates stepped punishments and warning limits globally
type WarnEngine struct {
	bot       *Bot
	steps     map[int]PunishStep
	maxWarns  int
	finalStep PunishStep
	cooldown  time.Duration
	autoCmds  bool
}

// Warns opens the fluent WarnEngine configuration pipeline on Bot
func (b *Bot) Warns() *WarnEngine {
	return &WarnEngine{
		bot:      b,
		steps:    make(map[int]PunishStep),
		maxWarns: 3, // Default threshold of 3 warnings
		finalStep: PunishStep{
			Type:    PunishBan,
			Message: "🚫 کاربر {name} به دلیل دریافت اخطار نهایی ({count}/{max}) از گروه مسدود شد.",
		},
	}
}

// Limit configures the maximum warning threshold before final punishment is executed
func (we *WarnEngine) Limit(n int) *WarnEngine {
	if n > 0 {
		we.maxWarns = n
	}
	return we
}

// Cooldown sets a duration after which a warning automatically decrements (auto-expire)
func (we *WarnEngine) Cooldown(d time.Duration) *WarnEngine {
	we.cooldown = d
	return we
}

// On maps a warning count directly to a custom PunishmentStep
func (we *WarnEngine) On(count int, step PunishStep) *WarnEngine {
	if count > 0 {
		we.steps[count] = step
	}
	return we
}

// OnFinal sets the ultimate punishment when warning limits are exceeded
func (we *WarnEngine) OnFinal(step PunishStep) *WarnEngine {
	we.finalStep = step
	return we
}

// AutoCommands enables automatic registration of administrative commands (/warn, /unwarn, /warns)
func (we *WarnEngine) AutoCommands() *WarnEngine {
	we.autoCmds = true
	return we
}

// Build finalizes the WarnEngine configuration and registers commands if requested
func (we *WarnEngine) Build() *WarnEngine {
	if we.autoCmds {
		we.RegisterCommands()
	}
	return we
}

// Warn issues a warning to a user, increments DB count, logs reason, and triggers punishment
func (we *WarnEngine) Warn(c *Ctx, reason string) error {
	chatID, err := c.ChatID()
	if err != nil {
		return err
	}
	userID := c.SenderID()
	if userID == 0 {
		return errors.New("cannot resolve sender ID")
	}

	// Bypass group administrators and owner
	isAdmin, errAdmin := c.Chat().IsAdmin().Go()
	if errAdmin == nil && isAdmin {
		return nil
	}
	we.bot.mu.RLock()
	isOwner := userID == we.bot.MaintenanceAdminID
	we.bot.mu.RUnlock()
	if isOwner {
		return nil
	}

	countKey := fmt.Sprintf("warn_count:%d:%d", chatID, userID)
	reasonsKey := fmt.Sprintf("warn_reasons:%d:%d", chatID, userID)
	db := we.bot.dbInstance

	// Log the warning reason with exact timestamp into GOB database
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	newReason := fmt.Sprintf("📌 [%s] %s", nowStr, reason)
	_ = db.Tx(func(store map[string]any) {
		var list []string
		if val, ok := store[reasonsKey]; ok {
			if slice, okSlice := val.([]string); okSlice {
				list = slice
			}
		}
		store[reasonsKey] = append(list, newReason)
	})

	// Read and increment count atomically inside database transaction
	var newCount int
	errTx := db.Tx(func(store map[string]any) {
		current := 0
		if val, ok := store[countKey]; ok {
			if iVal, okInt := val.(int); okInt {
				current = iVal
			} else if iVal, okInt := val.(int64); okInt {
				current = int(iVal)
			}
		}
		newCount = current + 1
		store[countKey] = newCount
	})
	if errTx != nil {
		return errTx
	}

	// Resolve user's mention name safely
	userName := ""
	if c.Message != nil && c.Message.From != nil {
		userName = c.Message.From.Mention()
	} else if c.Update != nil && c.Update.CallbackQuery != nil {
		userName = c.Update.CallbackQuery.From.Mention()
	} else {
		userName = fmt.Sprintf("User %d", userID)
	}

	// Retrieve corresponding step punishment
	step, hasStep := we.steps[newCount]
	if !hasStep && newCount >= we.maxWarns {
		step = we.finalStep
		hasStep = true
	}

	if !hasStep {
		// Fallback warning template
		step = PunishStep{
			Type:    PunishWarn,
			Message: "⚠️ {name} عزیز، شما یک اخطار دریافت کردید.\nعلت: {reason}\n📊 اخطارها: {count} از {max}",
		}
	}

	// Format custom variables inside message template
	msgText := step.Message
	if msgText == "" {
		msgText = "⚠️ {name} عزیز، شما اخطار شماره {count} را دریافت کردید."
	}
	msgText = strings.ReplaceAll(msgText, "{name}", userName)
	msgText = strings.ReplaceAll(msgText, "{reason}", reason)
	msgText = strings.ReplaceAll(msgText, "{count}", strconv.Itoa(newCount))
	msgText = strings.ReplaceAll(msgText, "{max}", strconv.Itoa(we.maxWarns))

	// Execute corresponding stepped punishment
	switch step.Type {
	case PunishWarn:
		_, _ = c.Send().Text(msgText).Markdown().Temp(10 * time.Second).Go()

	case PunishMute:
		_ = c.Chat().Mute(userID).For(step.Duration).Go()
		_, _ = c.Send().Text(msgText).Markdown().Temp(15 * time.Second).Go()

	case PunishKick:
		_ = c.Chat().Ban(userID).Go()
		_ = c.Chat().Unban(userID).OnlyIfBanned(true).Go()
		_, _ = c.Send().Text(msgText).Markdown().Temp(15 * time.Second).Go()

	case PunishBan:
		_ = c.Chat().Ban(userID).Go()
		_, _ = c.Send().Text(msgText).Markdown().Temp(15 * time.Second).Go()
	}

	// Reset warnings from DB if max limits are reached and final step is executed
	if newCount >= we.maxWarns {
		_ = db.Del(countKey)
		_ = db.Del(reasonsKey)
	} else if we.cooldown > 0 {
		// Schedule automatic warning expiration (cooldown decrement)
		botInstance := we.bot
		botInstance.Task().In(we.cooldown, func() {
			_ = db.Tx(func(store map[string]any) {
				if val, ok := store[countKey]; ok {
					current := 0
					if iVal, okInt := val.(int); okInt {
						current = iVal
					} else if iVal, okInt := val.(int64); okInt {
						current = int(iVal)
					}
					if current > 0 {
						// Decrement warnings safely over time
						store[countKey] = current - 1
					}
				}
			})
		})
	}

	return nil
}

// RegisterCommands automatically boots group administration commands: /warn, /unwarn, /warns with self-destructing alerts
func (we *WarnEngine) RegisterCommands() {
	// Register /warns command to check warning history (self-destructs after 15 seconds)
	we.bot.On().Cmd("warns").Do(func(c *Ctx) {
		_ = c.Del().Go() // Instantly delete the incoming command message to keep the chat tidy

		chatID, err := c.ChatID()
		if err != nil {
			return
		}
		targetUserID := c.SenderID()
		targetUser := c.Message.From

		// Inspect replied user's warnings if replying to a message
		if c.Message.ReplyToMessage != nil && c.Message.ReplyToMessage.From != nil {
			targetUserID = c.Message.ReplyToMessage.From.ID
			targetUser = c.Message.ReplyToMessage.From
		}

		countKey := fmt.Sprintf("warn_count:%d:%d", chatID, targetUserID)
		reasonsKey := fmt.Sprintf("warn_reasons:%d:%d", chatID, targetUserID)

		count := 0
		if val, ok := we.bot.dbInstance.Get(countKey); ok {
			if iVal, okInt := val.(int); okInt {
				count = iVal
			} else if iVal, okInt := val.(int64); okInt {
				count = int(iVal)
			}
		}

		var reasons []string
		if val, ok := we.bot.dbInstance.Get(reasonsKey); ok {
			if rSlice, okSlice := val.([]string); okSlice {
				reasons = rSlice
			}
		}

		if count == 0 {
			// Sends a temporary 10-second alert
			_, _ = c.Send().Text(fmt.Sprintf("🟢 کاربر %s هیچ اخطاری در این گروه ندارد.", targetUser.Mention())).Markdown().Temp(10 * time.Second).Go()
			return
		}

		history := "علتی ثبت نشده است."
		if len(reasons) > 0 {
			history = strings.Join(reasons, "\n")
		}

		report := Text().
			Line("📊 *وضعیت اخطارهای کاربر:* {name}").
			Line().
			Line("⚠️ تعداد اخطارها: *{count} از {max}*").
			Line().
			Line("📝 *تاریخچه علت اخطارها:*").
			Line("{history}").
			Bind("name", targetUser.Mention()).
			Bind("count", count).
			Bind("max", we.maxWarns).
			Bind("history", history).
			Go()

		// Sends a temporary 15-second detailed report
		_, _ = c.Send().Text(report).Markdown().Temp(15 * time.Second).Go()
	})

	// Register /warn command for manual admin warning (errors self-destruct after 10s)
	we.bot.On().Cmd("warn").Do(AdminsOnly(), func(c *Ctx) {
		_ = c.Del().Go() // Instantly delete the incoming command message to keep the chat tidy

		if c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
			_, _ = c.Send().Text("⚠️ لطفاً این دستور را روی پیام کاربر مورد نظر ریپلای کنید.").Temp(10 * time.Second).Go()
			return
		}

		reason := "توسط مدیریت"
		args, ok := c.Arg().([]string)
		if ok && len(args) > 0 {
			reason = strings.Join(args, " ")
		}

		// Create a clean, un-recycled temporary context to prevent sync.Pool collisions
		ctxCopy := &Ctx{
			Bot:     c.Bot,
			Update:  c.Update,
			Message: c.Message.ReplyToMessage,
		}
		_ = we.Warn(ctxCopy, "مدیریت: "+reason)
	})

	// Register /unwarn command to decrement warnings (all replies self-destruct after 10s)
	we.bot.On().Cmd("unwarn").Do(AdminsOnly(), func(c *Ctx) {
		_ = c.Del().Go() // Instantly delete the incoming command message to keep the chat tidy

		if c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
			_, _ = c.Send().Text("⚠️ لطفاً این دستور را روی پیام کاربر مورد نظر ریپلای کنید.").Temp(10 * time.Second).Go()
			return
		}

		chatID, _ := c.ChatID()
		targetUserID := c.Message.ReplyToMessage.From.ID
		targetUser := c.Message.ReplyToMessage.From

		countKey := fmt.Sprintf("warn_count:%d:%d", chatID, targetUserID)
		reasonsKey := fmt.Sprintf("warn_reasons:%d:%d", chatID, targetUserID)

		var newCount int
		_ = we.bot.dbInstance.Tx(func(store map[string]any) {
			current := 0
			if val, ok := store[countKey]; ok {
				if iVal, okInt := val.(int); okInt {
					current = iVal
				} else if iVal, okInt := val.(int64); okInt {
					current = int(iVal)
				}
			}
			if current > 0 {
				newCount = current - 1
				store[countKey] = newCount
			} else {
				newCount = 0
			}
		})

		if newCount == 0 {
			_ = we.bot.dbInstance.Del(countKey)
			_ = we.bot.dbInstance.Del(reasonsKey)
			_, _ = c.Send().Text(fmt.Sprintf("✅ تمام اخطارهای کاربر %s بخشیده و صفر شد.", targetUser.Mention())).Markdown().Temp(10 * time.Second).Go()
		} else {
			_, _ = c.Send().Text(fmt.Sprintf("📉 یک اخطار از کاربر %s کسر شد.\n📊 اخطارهای فعلی: %d از %d", targetUser.Mention(), newCount, we.maxWarns)).Markdown().Temp(10 * time.Second).Go()
		}
	})
}
