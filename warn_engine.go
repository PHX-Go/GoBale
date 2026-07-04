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
	// PunishWarn sends a self-destroying warning alert
	PunishWarn PunishmentType = "warn"
	// PunishMute temporarily restricts writing permissions
	PunishMute PunishmentType = "mute"
	// PunishKick evicts the user from the chat group
	PunishKick PunishmentType = "kick"
	// PunishBan blocks the user from the chat group permanently
	PunishBan PunishmentType = "ban"
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

// Kick creates a group eviction step
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

// Cooldown sets a duration after which a warning automatically decrements
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

// AutoCommands enables automatic registration of administrative commands
func (we *WarnEngine) AutoCommands() *WarnEngine {
	we.autoCmds = true
	return we
}

// Build finalizes the WarnEngine configuration and registers commands/cron tasks
func (we *WarnEngine) Build() *WarnEngine {
	if we.autoCmds {
		we.RegisterCommands()
	}

	// Register background persistent warnings cleaner (runs every 1 minute)
	if we.cooldown > 0 {
		we.bot.Task().Every(1*time.Minute, func() {
			db := we.bot.dbInstance
			dbConcrete, ok := db.(*Database)
			if !ok || dbConcrete == nil {
				return
			}

			dbConcrete.mu.Lock()
			now := time.Now().UnixNano() // Upgraded to UnixNano
			var keysToDel []string

			// Scan persistent store for expired timestamps safely
			for k, val := range dbConcrete.store {
				if strings.HasPrefix(k, "warn_expires:") {
					if list, okSlice := val.([]int64); okSlice && len(list) > 0 {
						var newList []int64
						expiredCount := 0
						for _, exp := range list {
							if now >= exp {
								expiredCount++
							} else {
								newList = append(newList, exp)
							}
						}

						if expiredCount > 0 {
							parts := strings.Split(k, ":")
							if len(parts) >= 3 {
								chatID, _ := strconv.ParseInt(parts[1], 10, 64)
								userID, _ := strconv.ParseInt(parts[2], 10, 64)

								countKey := fmt.Sprintf("warn_count:%d:%d", chatID, userID)
								if countVal, okCount := dbConcrete.store[countKey]; okCount {
									current := 0
									if iVal, okInt := countVal.(int); okInt {
										current = iVal
									} else if iVal, okInt := countVal.(int64); okInt {
										current = int(iVal)
									}

									newCount := current - expiredCount
									if newCount <= 0 {
										delete(dbConcrete.store, countKey)
										delete(dbConcrete.store, fmt.Sprintf("warn_reasons:%d:%d", chatID, userID))
										dbConcrete.appendWAL(walEntry{Op: walDel, Key: countKey})
										dbConcrete.appendWAL(walEntry{Op: walDel, Key: fmt.Sprintf("warn_reasons:%d:%d", chatID, userID)})
										keysToDel = append(keysToDel, k)
									} else {
										dbConcrete.store[countKey] = newCount
										dbConcrete.store[k] = newList
										dbConcrete.appendWAL(walEntry{Op: walSet, Key: countKey, Val: newCount})
										dbConcrete.appendWAL(walEntry{Op: walSet, Key: k, Val: newList})
									}
								}
							}
						}
					}
				}
			}

			// Delete fully expired keys
			for _, k := range keysToDel {
				delete(dbConcrete.store, k)
				dbConcrete.appendWAL(walEntry{Op: walDel, Key: k})
			}
			dbConcrete.mu.Unlock()
		})
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
	expiresKey := fmt.Sprintf("warn_expires:%d:%d", chatID, userID)
	db := we.bot.dbInstance

	// Log the warning reason with exact timestamp
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

	// Set persistent expiration timestamp in database (Nanosecond precision)
	if we.cooldown > 0 {
		expireTime := time.Now().Add(we.cooldown).UnixNano() // Upgraded to UnixNano
		_ = db.Tx(func(store map[string]any) {
			var list []int64
			if val, ok := store[expiresKey]; ok {
				if slice, okSlice := val.([]int64); okSlice {
					list = slice
				}
			}
			store[expiresKey] = append(list, expireTime)
		})
	}

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
		step = PunishStep{
			Type:    PunishWarn,
			Message: "⚠️ {name} عزیز، شما یک اخطار دریافت کردید.\nعلت: {reason}\n📊 اخطارها: {count} از {max}",
		}
	}

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

	// Reset warnings from DB if max limits are reached
	if newCount >= we.maxWarns {
		_ = db.Del(countKey)
		_ = db.Del(reasonsKey)
		_ = db.Del(expiresKey)
	}

	return nil
}

// RegisterCommands automatically boots group administration commands: /warn, /unwarn, /warns
func (we *WarnEngine) RegisterCommands() {
	we.bot.On().Cmd("warns").Do(func(c *Ctx) {
		_ = c.Del().Go()

		chatID, err := c.ChatID()
		if err != nil {
			return
		}
		targetUserID := c.SenderID()
		targetUser := c.Message.From

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

		_, _ = c.Send().Text(report).Markdown().Temp(15 * time.Second).Go()
	})

	we.bot.On().Cmd("warn").Do(AdminsOnly(), func(c *Ctx) {
		_ = c.Del().Go()

		if c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
			_, _ = c.Send().Text("⚠️ لطفاً این دستور را روی پیام کاربر مورد نظر ریپلای کنید.").Temp(10 * time.Second).Go()
			return
		}

		reason := "توسط مدیریت"
		args, ok := c.Arg().([]string)
		if ok && len(args) > 0 {
			reason = strings.Join(args, " ")
		}

		ctxCopy := &Ctx{
			Bot:     c.Bot,
			Update:  c.Update,
			Message: c.Message.ReplyToMessage,
		}
		_ = we.Warn(ctxCopy, "مدیریت: "+reason)
	})

	we.bot.On().Cmd("unwarn").Do(AdminsOnly(), func(c *Ctx) {
		_ = c.Del().Go()

		if c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
			_, _ = c.Send().Text("⚠️ لطفاً این دستور را روی پیام کاربر مورد نظر ریپلای کنید.").Temp(10 * time.Second).Go()
			return
		}

		chatID, _ := c.ChatID()
		targetUserID := c.Message.ReplyToMessage.From.ID
		targetUser := c.Message.ReplyToMessage.From

		countKey := fmt.Sprintf("warn_count:%d:%d", chatID, targetUserID)
		reasonsKey := fmt.Sprintf("warn_reasons:%d:%d", chatID, targetUserID)
		expiresKey := fmt.Sprintf("warn_expires:%d:%d", chatID, targetUserID)

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

				// Pop the last expiration timestamp
				if valExp, okExp := store[expiresKey]; okExp {
					if list, okSlice := valExp.([]int64); okSlice && len(list) > 0 {
						store[expiresKey] = list[:len(list)-1]
					}
				}
			} else {
				newCount = 0
			}
		})

		if newCount == 0 {
			_ = we.bot.dbInstance.Del(countKey)
			_ = we.bot.dbInstance.Del(reasonsKey)
			_ = we.bot.dbInstance.Del(expiresKey)
			_, _ = c.Send().Text(fmt.Sprintf("✅ تمام اخطارهای کاربر %s بخشیده و صفر شد.", targetUser.Mention())).Markdown().Temp(10 * time.Second).Go()
		} else {
			_, _ = c.Send().Text(fmt.Sprintf("📉 یک اخطار از کاربر %s کسر شد.\n📊 اخطارهای فعلی: %d از %d", targetUser.Mention(), newCount, we.maxWarns)).Markdown().Temp(10 * time.Second).Go()
		}
	})
}
