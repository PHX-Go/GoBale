package gobale

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MediaAll defines restriction for all media types simultaneously
const MediaAll MediaType = "all"

// MediaGuardChain manages configuration of media guard middleware and commands
type MediaGuardChain struct {
	on           *OnChain
	warnDuration time.Duration
	customMsg    string
	useCommands  bool
	delCmds      bool
	silent       bool
}

// MediaGuard initiates the media guard configuration chain
func (o *OnChain) MediaGuard() *MediaGuardChain {
	return &MediaGuardChain{
		on:           o,
		warnDuration: 5 * time.Second, // Default self-destroying TTL
	}
}

// Warn sets the TTL of temporary warning alerts
func (m *MediaGuardChain) Warn(d time.Duration) *MediaGuardChain {
	m.warnDuration = d
	m.silent = false
	return m
}

// Msg overrides the default warning alert text
func (m *MediaGuardChain) Msg(text string) *MediaGuardChain {
	m.customMsg = text
	return m
}

// Silent disables warning alerts entirely for silent restriction
func (m *MediaGuardChain) Silent() *MediaGuardChain {
	m.silent = true
	m.warnDuration = 0
	return m
}

// Commands enables automatic registration of admin restrict/unrestrict commands
func (m *MediaGuardChain) Commands() *MediaGuardChain {
	m.useCommands = true
	return m
}

// DelCmds enables automatic deletion of incoming command messages
func (m *MediaGuardChain) DelCmds() *MediaGuardChain {
	m.delCmds = true
	return m
}

// Go completes building the chain, registering unified ChatGuard and optional commands
func (m *MediaGuardChain) Go() *OnChain {
	m.on.bot.mu.Lock()
	// Registers the unified multi-purpose ChatGuard middleware
	m.on.bot.middlewares = append(m.on.bot.middlewares, ChatGuard(m.warnDuration, m.customMsg, m.silent))
	m.on.bot.mu.Unlock()

	if m.useCommands {
		m.on.Cmd("restrict").Do(AdminsOnly(), RestrictMediaHandler(m.delCmds))
		m.on.Cmd("unrestrict").Do(AdminsOnly(), UnrestrictMediaHandler(m.delCmds))
	}

	return m.on
}

// MediaRestrictChain handles user/group media restriction database transactions
type MediaRestrictChain struct {
	bot     *Bot
	ctx     context.Context
	user    int64
	chat    any
	toAdd   []MediaType
	toDel   []MediaType
	clear   bool
	isGroup bool
}

// RestrictMedia initiates media restriction transaction from Bot context
func (b *Bot) RestrictMedia(userID int64) *MediaRestrictChain {
	return &MediaRestrictChain{
		bot:  b,
		ctx:  context.Background(),
		user: userID,
	}
}

// RestrictMedia initiates media restriction transaction from Ctx context resolving chatID automatically
func (c *Ctx) RestrictMedia(userID int64) *MediaRestrictChain {
	id, _ := c.ChatID()
	return &MediaRestrictChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		user: userID,
		chat: id,
	}
}

// Group configures the transaction to apply to the entire group instead of a single user
func (r *MediaRestrictChain) Group() *MediaRestrictChain {
	r.isGroup = true
	return r
}

// Chat sets the target chat reference
func (r *MediaRestrictChain) Chat(chatID any) *MediaRestrictChain {
	r.chat = chatID
	return r
}

// Block adds media categories to the restriction list
func (r *MediaRestrictChain) Block(types ...MediaType) *MediaRestrictChain {
	r.toAdd = append(r.toAdd, types...)
	return r
}

// Allow removes media categories from the restriction list
func (r *MediaRestrictChain) Allow(types ...MediaType) *MediaRestrictChain {
	r.toDel = append(r.toDel, types...)
	return r
}

// Clear clears all active media restrictions
func (r *MediaRestrictChain) Clear() *MediaRestrictChain {
	r.clear = true
	return r
}

// Go executes the restriction database transaction using bot's direct storage
func (r *MediaRestrictChain) Go() error {
	if r.chat == nil {
		return fmt.Errorf("missing chat ID for restriction chain")
	}

	resolved := r.bot.ResolveChatID(r.chat)
	var key string
	if r.isGroup {
		// Group-wide restriction key structure
		key = fmt.Sprintf("blocked_media_group_%v", resolved)
	} else {
		// Individual user restriction key structure
		key = fmt.Sprintf("blocked_media_%v_%d", resolved, r.user)
	}

	db := r.bot.dbInstance

	if r.clear {
		return db.Del(key)
	}

	val, ok := db.Get(key)
	var current []string
	if ok {
		if slice, okSlice := val.([]string); okSlice {
			current = slice
		}
	}

	m := make(map[string]bool)
	for _, b := range current {
		m[b] = true
	}
	for _, t := range r.toAdd {
		m[string(t)] = true
	}
	for _, t := range r.toDel {
		delete(m, string(t)) // Fixed Go map delete syntax
	}

	if len(m) == 0 {
		return db.Del(key)
	}

	var updated []string
	for k := range m {
		updated = append(updated, k)
	}
	return db.Set(key, updated)
}

// UserMediaGuard checks active group/user media restrictions and deletes matching messages
func UserMediaGuard(warnDuration time.Duration, customMsg string, silent bool) Handler {
	defaultWarn := "⚠️ کاربر عزیز {name}، شما اجازه ارسال رسانه از نوع [{type}] را در این چت ندارید!"
	if customMsg != "" {
		defaultWarn = customMsg
	}

	return func(c *Ctx) {
		// Ignore callback queries and service messages
		if c.Message == nil || c.Message.From == nil || len(c.Message.NewChatMembers) > 0 || c.Message.LeftChatMember != nil {
			c.Next()
			return
		}

		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		senderID := c.SenderID()

		// Bypass global administrators and owner
		c.Bot.mu.RLock()
		isOwner := senderID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()
		if isOwner {
			c.Next()
			return
		}

		isAdmin, errAdmin := c.Chat().IsAdmin().Go()
		if errAdmin == nil && isAdmin {
			c.Next()
			return
		}

		// 1. Retrieve Group-wide restrictions from direct database
		groupKey := fmt.Sprintf("blocked_media_group_%v", chatID)
		groupVal, okGroup := c.Bot.dbInstance.Get(groupKey)

		// 2. Retrieve Individual user restrictions from direct database
		userKey := fmt.Sprintf("blocked_media_%v_%d", chatID, senderID)
		userVal, okUser := c.Bot.dbInstance.Get(userKey)

		// Map all active restrictions into a lookup map
		blockedMap := make(map[string]bool)

		if okGroup {
			if blockedSlice, okSlice := groupVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedMap[b] = true
				}
			}
		}

		if okUser {
			if blockedSlice, okSlice := userVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedMap[b] = true
				}
			}
		}

		// If no active restrictions exist, proceed to next handler
		if len(blockedMap) == 0 {
			c.Next()
			return
		}

		// Detect media type in the incoming message
		var detected MediaType
		var matchedTypeFarsi string

		if len(c.Message.Photo) > 0 {
			detected = MediaPhoto
			matchedTypeFarsi = "تصویر"
		} else if c.Message.Video != nil {
			detected = MediaVideo
			matchedTypeFarsi = "ویدیو"
		} else if c.Message.Audio != nil {
			detected = MediaAudio
			matchedTypeFarsi = "فایل صوتی"
		} else if c.Message.Document != nil {
			detected = MediaDocument
			matchedTypeFarsi = "سند (فایل)"
		} else if c.Message.Voice != nil {
			detected = MediaVoice
			matchedTypeFarsi = "پیام صوتی (وویس)"
		} else if c.Message.Sticker != nil {
			detected = MediaSticker
			matchedTypeFarsi = "استیکر"
		} else if c.Message.Animation != nil {
			detected = MediaAnimation
			matchedTypeFarsi = "گیف (انیمیشن)"
		} else if c.Message.Location != nil {
			detected = MediaLocation
			matchedTypeFarsi = "موقعیت مکانی"
		} else if c.Message.Contact != nil {
			detected = MediaContact
			matchedTypeFarsi = "مخاطب"
		}

		// Check if detected media is blocked under group or user rules
		if detected != "" {
			isBlocked := blockedMap[string(detected)] || blockedMap[string(MediaAll)]

			if isBlocked {
				// Delete the restricted media message natively
				_ = c.Del().Go()

				// Send self-destroying warning alert if not set to silent
				if !silent && warnDuration > 0 {
					warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
					warn = strings.ReplaceAll(warn, "{type}", matchedTypeFarsi)
					_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
				}

				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// isValidMediaType validates if the string matches a supported MediaType
func isValidMediaType(t string) (MediaType, bool) {
	switch MediaType(t) {
	case MediaPhoto, MediaVideo, MediaAudio, MediaDocument, MediaVoice, MediaSticker, MediaAnimation, MediaLocation, MediaContact, MediaAll:
		return MediaType(t), true
	}
	return "", false
}

// RestrictMediaHandler handles fluent /restrict command logic in groups and private DMs
func RestrictMediaHandler(delCmds bool) Handler {
	return func(c *Ctx) {
		args, _ := c.Arg().([]string)
		var targetUserID int64
		var targetChatID int64
		var mediaArgs []string

		// Delete the incoming command message natively if enabled and inside a group
		if delCmds && !c.IsPrivate() {
			_ = c.Del().Go()
		}

		// 1. Remote management from Private DM (Chat ID required)
		if c.IsPrivate() {
			if len(args) < 2 {
				_, _ = c.Send().Text("❌ **Remote Restrict Guide:**\n`/restrict [chatID] [userID] [types...]`").Markdown().Go()
				return
			}

			// Parse group/channel chat ID (usually negative, e.g., -100123456)
			cid, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				_, _ = c.Send().Text("❌ Target Chat ID is invalid.").Go()
				return
			}
			targetChatID = cid

			// Parse user ID
			uid, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				_, _ = c.Send().Text("❌ Target User ID is invalid.").Go()
				return
			}
			targetUserID = uid
			mediaArgs = args[2:]
		} else {
			// 2. Regular Group management (Chat ID resolved automatically)
			id, _ := c.ChatID()
			targetChatID = id

			// Check if replying to a message
			if c.Message.ReplyToMessage != nil && c.Message.ReplyToMessage.From != nil {
				targetUserID = c.Message.ReplyToMessage.From.ID
				mediaArgs = args
			} else {
				// Parse from arguments, if no arguments inside group: Lock all media for the entire group
				if len(args) == 0 {
					_ = c.RestrictMedia(0).Chat(targetChatID).Group().Block(MediaAll).Go()
					_, _ = c.Send().Text("🚫 گروه با موفقیت قفل شد. ارسال هرگونه رسانه برای کل اعضا مسدود گردید.").Temp(5 * time.Second).Go()
					return
				}

				// Check if the first argument is a userID or a MediaType for group restrict
				uid, err := strconv.ParseInt(args[0], 10, 64)
				if err != nil {
					// First argument is not a number, so it must be Media Types for Group Restrict
					var toBlock []MediaType
					var invalidTypes []string

					for _, arg := range args {
						t := strings.ToLower(strings.TrimSpace(arg))
						if mt, ok := isValidMediaType(t); ok {
							toBlock = append(toBlock, mt)
						} else {
							invalidTypes = append(invalidTypes, arg)
						}
					}

					if len(invalidTypes) > 0 {
						msg := fmt.Sprintf("❌ موارد زیر نامعتبر هستند:\n`%s`\n\n**رسانه‌های مجاز:** `photo, video, voice, audio, document, sticker, animation, location, contact, all`", strings.Join(invalidTypes, ", "))
						_, _ = c.Send().Text(msg).Markdown().Temp(10 * time.Second).Go()
						return
					}

					// Restrict the entire group
					_ = c.RestrictMedia(0).Chat(targetChatID).Group().Block(toBlock...).Go()
					_, _ = c.Send().Text(fmt.Sprintf("🚫 ارسال موارد [%s] برای کل اعضای گروه مسدود شد.", strings.Join(args, ", "))).Temp(5 * time.Second).Go()
					return
				}

				// UserID indeed detected
				targetUserID = uid
				mediaArgs = args[1:]
			}
		}

		// Block all media for user if no types specified
		if len(mediaArgs) == 0 {
			_ = c.RestrictMedia(targetUserID).Chat(targetChatID).Block(MediaAll).Go()
			msg := fmt.Sprintf("🚫 تمام دسترسی‌های رسانه‌ای کاربر %d در چت %d مسدود شد.", targetUserID, targetChatID)
			if c.IsPrivate() {
				_, _ = c.Send().Text(msg).Go()
			} else {
				_, _ = c.Send().Text(msg).Temp(5 * time.Second).Go()
			}
			return
		}

		var toBlock []MediaType
		var invalidTypes []string

		for _, arg := range mediaArgs {
			t := strings.ToLower(strings.TrimSpace(arg))
			if mt, ok := isValidMediaType(t); ok {
				toBlock = append(toBlock, mt)
			} else {
				invalidTypes = append(invalidTypes, arg)
			}
		}

		if len(invalidTypes) > 0 {
			msg := fmt.Sprintf("❌ موارد زیر نامعتبر هستند:\n`%s`\n\n**رسانه‌های مجاز:** `photo, video, voice, audio, document, sticker, animation, location, contact, all`", strings.Join(invalidTypes, ", "))
			if c.IsPrivate() {
				_, _ = c.Send().Text(msg).Markdown().Go()
			} else {
				_, _ = c.Send().Text(msg).Markdown().Temp(10 * time.Second).Go()
			}
			return
		}

		_ = c.RestrictMedia(targetUserID).Chat(targetChatID).Block(toBlock...).Go()
		msg := fmt.Sprintf("🚫 دسترسی کاربر %d در چت %d به موارد [%s] مسدود شد.", targetUserID, targetChatID, strings.Join(mediaArgs, ", "))
		if c.IsPrivate() {
			_, _ = c.Send().Text(msg).Go()
		} else {
			_, _ = c.Send().Text(msg).Temp(5 * time.Second).Go()
		}
	}
}

// UnrestrictMediaHandler handles fluent /unrestrict command logic in groups and private DMs
func UnrestrictMediaHandler(delCmds bool) Handler {
	return func(c *Ctx) {
		args, _ := c.Arg().([]string)
		var targetUserID int64
		var targetChatID int64
		var mediaArgs []string

		// Delete the incoming command message natively if enabled and inside a group
		if delCmds && !c.IsPrivate() {
			_ = c.Del().Go()
		}

		// 1. Remote management from Private DM (Chat ID required)
		if c.IsPrivate() {
			if len(args) < 2 {
				_, _ = c.Send().Text("❌ **Remote Unrestrict Guide:**\n`/unrestrict [chatID] [userID] [types...]`").Markdown().Go()
				return
			}

			// Parse group/channel chat ID
			cid, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				_, _ = c.Send().Text("❌ Target Chat ID is invalid.").Go()
				return
			}
			targetChatID = cid

			// Parse user ID
			uid, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				_, _ = c.Send().Text("❌ Target User ID is invalid.").Go()
				return
			}
			targetUserID = uid
			mediaArgs = args[2:]
		} else {
			// 2. Regular Group management (Chat ID resolved automatically)
			id, _ := c.ChatID()
			targetChatID = id

			if c.Message.ReplyToMessage != nil && c.Message.ReplyToMessage.From != nil {
				targetUserID = c.Message.ReplyToMessage.From.ID
				mediaArgs = args
			} else {
				// Unlock the entire group if `/unrestrict` is sent inside a group with no arguments
				if len(args) == 0 {
					_ = c.RestrictMedia(0).Chat(targetChatID).Group().Clear().Go()
					_, _ = c.Send().Text("✅ قفل گروه کاملاً باز شد. ارسال تمامی رسانه‌ها مجدداً برای همه آزاد گردید.").Temp(5 * time.Second).Go()
					return
				}

				// Check if the first argument is a userID or a MediaType for group unrestrict
				uid, err := strconv.ParseInt(args[0], 10, 64)
				if err != nil {
					// First argument is not a number, so it must be Media Types for Group Unrestrict
					var toUnblock []MediaType
					for _, arg := range args {
						t := strings.ToLower(strings.TrimSpace(arg))
						if mt, ok := isValidMediaType(t); ok {
							toUnblock = append(toUnblock, mt)
						}
					}

					_ = c.RestrictMedia(0).Chat(targetChatID).Group().Allow(toUnblock...).Go()
					_, _ = c.Send().Text(fmt.Sprintf("✅ محدودیت کل اعضای گروه برای ارسال موارد [%s] لغو شد.", strings.Join(args, ", "))).Temp(5 * time.Second).Go()
					return
				}

				targetUserID = uid
				mediaArgs = args[1:]
			}
		}

		// Clear all user restrictions if no types specified
		if len(mediaArgs) == 0 {
			_ = c.RestrictMedia(targetUserID).Chat(targetChatID).Clear().Go()
			msg := fmt.Sprintf("✅ تمامی محدودیت‌های رسانه‌ای کاربر %d در چت %d لغو شد.", targetUserID, targetChatID)
			if c.IsPrivate() {
				_, _ = c.Send().Text(msg).Go()
			} else {
				_, _ = c.Send().Text(msg).Temp(5 * time.Second).Go()
			}
			return
		}

		var toUnblock []MediaType
		for _, arg := range mediaArgs {
			t := strings.ToLower(strings.TrimSpace(arg))
			if mt, ok := isValidMediaType(t); ok {
				toUnblock = append(toUnblock, mt)
			}
		}

		_ = c.RestrictMedia(targetUserID).Chat(targetChatID).Allow(toUnblock...).Go()
		msg := fmt.Sprintf("✅ محدودیت کاربر %d در چت %d برای ارسال موارد [%s] لغو شد.", targetUserID, targetChatID, strings.Join(mediaArgs, ", "))
		if c.IsPrivate() {
			_, _ = c.Send().Text(msg).Go()
		} else {
			_, _ = c.Send().Text(msg).Temp(5 * time.Second).Go()
		}
	}
}

// RegisterMediaRestrictCommands registers /restrict and /unrestrict commands automatically
func (o *OnChain) RegisterMediaRestrictCommands() *OnChain {
	o.Cmd("restrict").Do(AdminsOnly(), RestrictMediaHandler(false))
	o.Cmd("unrestrict").Do(AdminsOnly(), UnrestrictMediaHandler(false))
	return o
}
