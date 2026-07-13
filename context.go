package gobale

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ctx manages request pipeline state, execution context, and session data
type Ctx struct {
	Bot      *Bot
	Update   *Update
	Message  *Message
	handlers []Handler
	index    int8
	mu       sync.RWMutex
	Keys     map[string]any
	ctx      context.Context
	err      error
	prevText string
}

// SendLater schedules a simple text message to be sent to the current chat after a specified delay
func (c *Ctx) SendLater(text string, d time.Duration) {
	bot := c.Bot
	chatID, err := c.ChatID()
	if err != nil {
		return
	}
	bot.Task().In(d, func() {
		_, _ = bot.Send(chatID).Text(text).Go()
	})
}

// ReplyLater schedules a smart reply message to be sent after a specified delay
func (c *Ctx) ReplyLater(text string, d time.Duration) {
	bot := c.Bot
	chatID, err := c.ChatID()
	if err != nil {
		return
	}
	var replyID int64
	if c.Message != nil {
		// Respect smart reply logic to target the original message if present
		if c.Message.ReplyToMessage != nil {
			replyID = c.Message.ReplyToMessage.MessageID
		} else {
			replyID = c.Message.MessageID
		}
	}
	bot.Task().In(d, func() {
		_, _ = bot.Send(chatID).Text(text).Reply(replyID).Go()
	})
}

// ForwardTo natively forwards the active message in context to a target chat in one line
func (c *Ctx) ForwardTo(targetChat any) (*Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message in context to forward")
	}
	return c.Bot.Send(targetChat).Forward(c.Message.Chat.ID, c.Message.MessageID).Go()
}

// CopyTo natively copies the active message in context to a target chat in one line
func (c *Ctx) CopyTo(targetChat any) (*Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message in context to copy")
	}
	return c.Bot.Send(targetChat).Copy(c.Message.Chat.ID, c.Message.MessageID).Go()
}

// JoinGuard checks if the user is a member of the target channel, and automatically sends a beautiful verification panel if they are not
func (c *Ctx) JoinGuard(targetChat any, inviteLink string) (bool, error) {
	isMember, err := c.IsMemberOf(targetChat)
	if err != nil {
		// Log warning if bot is not admin in the target channel to prevent crashes
		c.Bot.Log().Warn("JoinGuard failed to check membership (ensure bot is admin in the target channel)").Err(err).Go()
		return true, nil // Fallback to let them proceed if the check itself fails
	}

	if isMember {
		return true, nil
	}

	resolved := c.Bot.ResolveChatID(targetChat)

	// Compile a beautiful inline keyboard with join link and dynamic verification callback
	markup := InlineMarkup().
		Row(NewInlineKeyboardButtonURL("📢 عضویت در کانال", inviteLink)).
		Row(Btn("✅ عضو شدم (تایید)").Callback(fmt.Sprintf("_sys_join_verify:%v", resolved))).
		Build()

	text := "⚠️ *[جوین اجباری]*\n\nکاربر عزیز، برای استفاده از خدمات این ربات، ابتدا باید در کانال رسمی ما عضو شوید."
	_, _ = c.Send().Text(text).Markup(markup).Markdown().Go()

	return false, nil
}

// IsMemberOf natively checks if the current sender is a member of the specified channel or group chat (handling Bale's 404 non-member quirk)
func (c *Ctx) IsMemberOf(targetChat any) (bool, error) {
	resolved := c.Bot.ResolveChatID(targetChat)
	member, err := c.Bot.Chat(resolved).Member(c.SenderID()).Go()
	if err != nil {
		// Bale platform quirk: if a user is not a member of the channel, getChatMember returns a hard 400/404 error instead of status: left!
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "no such group or user") || strings.Contains(errMsg, "404") || strings.Contains(errMsg, "400") {
			return false, nil // Conclusive proof that the user is NOT a member (resolves as clean false without system error)
		}
		return false, err // Real system error (e.g. bot is not admin in the target channel or connection timeout)
	}

	// Check if the user's status is creator, administrator or standard member natively
	isMember := member.Status == "creator" || member.Status == "administrator" || member.Status == "member"
	return isMember, nil
}

// Pin is a shortcut helper to natively pin the active message (or specified message ID) in the current group
func (c *Ctx) Pin(messageID ...int64) error {
	chatID, err := c.ChatID()
	if err != nil {
		return err
	}
	targetID := c.Message.MessageID
	if len(messageID) > 0 {
		targetID = messageID[0]
	}
	return c.Bot.Chat(chatID).Pin(targetID).Go()
}

// Unpin is a shortcut helper to natively unpin the active message (or specified message ID) in the current group
func (c *Ctx) Unpin(messageID ...int64) error {
	chatID, err := c.ChatID()
	if err != nil {
		return err
	}
	targetID := c.Message.MessageID
	if len(messageID) > 0 {
		targetID = messageID[0]
	}
	return c.Bot.Chat(chatID).Unpin(targetID).Go()
}

// Purge natively deletes a specified number of recent visible messages from the current chat concurrently, skipping gaps
func (c *Ctx) Purge(count int) error {
	if c.Message == nil {
		return errors.New("no message in context to purge from")
	}

	chatID, err := c.ChatID()
	if err != nil {
		return err
	}

	if count <= 0 {
		count = 5
	}
	if count > 100 {
		count = 100
	}

	msgID := c.Message.MessageID
	botInstance := c.Bot

	// Fire Adaptive Wave Deletion (AWD) in background to skip gaps concurrently
	go func() {
		defer func() {
			if r := recover(); r != nil {
				handlePanic(botInstance, r, nil)
			}
		}()

		targetCount := int64(count)
		deletedCount := int64(0)
		currentOffset := int64(0)
		maxSearchDepth := int64(count + 50) // Safe search boundary limit

		for deletedCount < targetCount && currentOffset < maxSearchDepth {
			remaining := targetCount - deletedCount
			var wg sync.WaitGroup
			var successChan = make(chan bool, remaining)

			for i := int64(0); i < remaining; i++ {
				targetID := msgID - currentOffset - i
				if targetID <= 0 {
					break
				}

				wg.Add(1)
				// Throttle requests using the bot's central semaphore pool
				botInstance.bgSemaphore <- struct{}{}
				go func(id int64) {
					defer func() {
						<-botInstance.bgSemaphore
						wg.Done()
					}()

					var res bool
					errReq := botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": id,
					}, &res)

					if errReq == nil && res {
						successChan <- true
					}
				}(targetID)
			}

			wg.Wait()
			close(successChan)

			// Calculate successful deletions in this wave
			waveSuccesses := int64(0)
			for range successChan {
				waveSuccesses++
			}

			deletedCount += waveSuccesses
			currentOffset += remaining

			// Break if bottom boundary reached
			if msgID-currentOffset <= 0 {
				break
			}
		}
	}()

	return nil
}

// SendMarkdown is a fluent shortcut to start a sending chain pre-configured with Markdown parse mode
func (c *Ctx) SendMarkdown(text string) *SendChain {
	return c.Send().Text(text).Markdown()
}

// ReplyMarkdown is a fluent shortcut to start a replying chain pre-configured with Markdown parse mode
func (c *Ctx) ReplyMarkdown(text string) *SendChain {
	return c.Reply().Text(text).Markdown()
}

// Delete is a shortcut helper to instantly delete the current message in context
func (c *Ctx) Delete() error {
	return c.Del().Go()
}

// RemoveMenu sends a simple text message while collapsing and removing the active reply keyboard
func (c *Ctx) RemoveMenu(text string) (*Message, error) {
	return c.Send().Text(text).MarkupRemove().Go()
}

// AnalyticsReport compiles and returns a beautifully formatted Persian report of the group's current stats dynamically
func (c *Ctx) AnalyticsReport(p ...PeriodType) (string, error) {
	period := PeriodDaily
	if len(p) > 0 {
		period = p[0]
	}

	// Query analytics natively using the fluent chain
	res, err := c.Analytics().Period(period).Go()
	if err != nil {
		return "", err
	}

	// Fetch dynamic group title using getChat natively to replace static name
	title := "گروه بدون نام"
	if info, errInfo := c.Chat().Info().Go(); errInfo == nil && info != nil {
		if info.Title != "" {
			title = info.Title
		} else if info.FirstName != "" {
			title = info.FirstName
		}
	}

	periodName := "امروز (روزانه)"
	if period == PeriodLifetime {
		periodName = "کل دوره (تا به امروز)"
	}

	report := Text().
		Line("📊 **گزارش آماری گروه {group_name}** 📈").
		Line("🆔 **شناسه گروه:** `{chat_id}`").
		Line().
		Line("📅 **دوره آمارگیر:** *{period_name}*").
		Line("🕒 **زمان گزارش:** `{time}`").
		Line().
		Line("📝 **آمار متون و واژگان:**").
		Line("  💬 تعداد پیام‌های متنی: *{text_cnt}*").
		Line("  🔤 تعداد کلمات: *{word_cnt}*").
		Line("  🔢 تعداد کاراکترها: *{char_cnt}*").
		Line("  🤖 تعداد دستورات: *{command_cnt}*").
		Line().
		Line("🖼️ **آمار دقیق رسانه‌ها:**").
		Line("  📦 مجموع کل رسانه‌ها: *{total_media}*").
		Line("  🖼️ تصاویر: *{photo_cnt}*").
		Line("  🎬 ویدیوها: *{video_cnt}*").
		Line("  🎙️ وویس‌ها: *{voice_cnt}*").
		Line("  🎵 موسیقی: *{audio_cnt}*").
		Line("  📁 اسناد و فایل‌ها: *{doc_cnt}*").
		Line("  👾 استیکرها: *{sticker_cnt}*").
		Line("  🎬 گیف‌ها (انیمیشن): *{anim_cnt}*").
		Line("  📍 موقعیت‌های مکانی: *{location_cnt}*").
		Line("  📇 مخاطبین: *{contact_cnt}*").
		Line().
		Line("🤝 **آمار تعاملات و پایش:**").
		Line("  ↩️ تعداد ریپلای‌ها: *{reply_cnt}*").
		Line("  ↪️ تعداد فورواردها: *{forward_cnt}*").
		Line("  📉 پیام‌های حذف‌شده: *{del_cnt}*").
		Line("  📝 پیام‌های ویرایش‌شده: *{edit_cnt}*").
		Line().
		Line("🔥 **ساعت اوج فعالیت:** *{peak_hour}:00* (با {peak_msgs} پیام)").
		Line("📊 **کل پیام‌های ثبت‌شده:** *{total_msgs}*").
		Bind("group_name", title).
		Bind("chat_id", res.ChatID).
		Bind("period_name", periodName).
		Bind("time", c.JalaliString()).
		Bind("text_cnt", Money(res.TextCount)).
		Bind("word_cnt", Money(res.WordCount)).
		Bind("char_cnt", Money(res.CharCount)).
		Bind("command_cnt", Money(res.CommandCount)).
		Bind("total_media", Money(res.TotalMedia)).
		Bind("photo_cnt", Money(res.PhotoCount)).
		Bind("video_cnt", Money(res.VideoCount)).
		Bind("voice_cnt", Money(res.VoiceCount)).
		Bind("audio_cnt", Money(res.AudioCount)).
		Bind("doc_cnt", Money(res.DocCount)).
		Bind("sticker_cnt", Money(res.StickerCount)).
		Bind("anim_cnt", Money(res.AnimCount)).
		Bind("location_cnt", Money(res.LocationCount)).
		Bind("contact_cnt", Money(res.ContactCount)).
		Bind("reply_cnt", Money(res.ReplyCount)).
		Bind("forward_cnt", Money(res.ForwardCount)).
		Bind("del_cnt", Money(res.DeleteCount)).
		Bind("edit_cnt", Money(res.EditCount)).
		Bind("peak_hour", res.PeakHour).
		Bind("peak_msgs", Money(res.PeakHourMsgs)).
		Bind("total_msgs", Money(res.TotalMsgs))

	return report.Go(), nil
}

// MuteUser mutes the target user purely without any logging side-effects
func (c *Ctx) MuteUser(d time.Duration) error {
	target, err := c.TargetUser()
	if err != nil {
		return err
	}
	return c.Chat().Mute(target).For(d).Go()
}

// UnmuteUser natively unmutes the target user (auto-resolves target from reply or arguments if omitted)
func (c *Ctx) UnmuteUser(userID ...int64) error {
	var target int64
	var err error
	if len(userID) > 0 {
		target = userID[0]
	} else {
		target, err = c.TargetUser()
		if err != nil {
			return err
		}
	}
	return c.Chat().Restrict(target).
		SendMessages(true).
		InviteUsers(true).
		PinMessages(true).
		ChangeInfo(true).
		Go()
}

// BanUser bans the target user purely without any logging side-effects
func (c *Ctx) BanUser(userID ...int64) error {
	var target int64
	var err error
	if len(userID) > 0 {
		target = userID[0]
	} else {
		target, err = c.TargetUser()
		if err != nil {
			return err
		}
	}
	return c.Chat().Ban(target).Go()
}

// DBSet is a shortcut helper to write a key-value pair to the local Database
func (c *Ctx) DBSet(key string, val any) error {
	return c.DB().Set(key, val).Go()
}

// DBGet is a shortcut helper to read a value from the local Database
func (c *Ctx) DBGet(key string) (any, bool) {
	return c.DB().Get(key).Go()
}

// DBDel is a shortcut helper to delete a key from the local Database
func (c *Ctx) DBDel(key string) error {
	return c.DB().Del(key).Go()
}

// SetState is a shortcut helper to update the active session FSM state
func (c *Ctx) SetState(state string) (string, error) {
	return c.Session().State(state).Go()
}

// GetState is a shortcut helper to retrieve the active session FSM state
func (c *Ctx) GetState() (string, error) {
	return c.Session().State().Go()
}

// SetData is a shortcut helper to save a value inside the active session data map directly
func (c *Ctx) SetData(key string, val any) {
	c.Session().Set(key, val)
}

// GetData is a shortcut helper to read a value from the active session data map directly
func (c *Ctx) GetData(key string) any {
	s := c.Session()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.DataMap == nil {
		return nil
	}
	return s.DataMap[key]
}

// JalaliString is a shortcut helper to format any Gregorian time (or time.Now() if omitted) into a Shamsi string
func (c *Ctx) JalaliString(t ...time.Time) string {
	target := time.Now()
	if len(t) > 0 {
		target = t[0]
	}
	return Jalali(target).Go()
}

// Reply opens the fluent sending chain pre-configured to reply to the active message with clean Ctx reference
func (c *Ctx) Reply() *SendChain {
	id, _ := c.ChatID()
	s := &SendChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		c:    c,
		chat: id,
	}
	if c.Message != nil {
		if c.Message.ReplyToMessage != nil {
			s.replyTo = c.Message.ReplyToMessage.MessageID
		} else {
			s.replyTo = c.Message.MessageID
		}
	}
	return s
}

// ReplyText is a shortcut helper to reply to the active message (or original replied-to message) with a simple text
func (c *Ctx) ReplyText(text string) (*Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message in context to reply to")
	}
	return c.Reply().Text(text).Go()
}

// SendText is a shortcut helper to send a simple text message to the current chat
func (c *Ctx) SendText(text string) (*Message, error) {
	return c.Send().Text(text).Go()
}

// TempText is a shortcut helper to send a self-destroying text message
func (c *Ctx) TempText(text string, d time.Duration) (*Message, error) {
	return c.Send().Text(text).Temp(d).Go()
}

// TargetUser resolves the target user ID from a reply message or explicit command arguments
func (c *Ctx) TargetUser() (int64, error) {
	if c.Message == nil {
		return 0, errors.New("no message in context")
	}

	// 1. Resolve target from reply-to message
	if c.Message.ReplyToMessage != nil && c.Message.ReplyToMessage.From != nil {
		return c.Message.ReplyToMessage.From.ID, nil
	}

	// 2. Resolve target from explicit command arguments
	args, ok := c.Arg().([]string)
	if ok && len(args) > 0 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			return id, nil
		}
	}

	return 0, errors.New("target user not specified (must reply to a user or provide an ID)")
}

// ArgString retrieves a command argument as string with an optional fallback default
func (c *Ctx) ArgString(idx int, fallback ...string) string {
	args, ok := c.Arg().([]string)
	if !ok || idx < 0 || idx >= len(args) {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return ""
	}
	return args[idx]
}

// ArgInt retrieves a command argument as integer with an optional fallback default
func (c *Ctx) ArgInt(idx int, fallback ...int) int {
	args, ok := c.Arg().([]string)
	if !ok || idx < 0 || idx >= len(args) {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
	if val, err := strconv.Atoi(args[idx]); err == nil {
		return val
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}

// Next executes the next handler in the execution chain
func (c *Ctx) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort halts the current request execution chain
func (c *Ctx) Abort() {
	c.index = int8(len(c.handlers))
}

// ChatID extracts and returns the current chat identifier safely, supporting dynamic callback fallbacks
func (c *Ctx) ChatID() (int64, error) {
	if c.Update == nil {
		return 0, errors.New("nil update")
	}
	if c.Update.Message != nil {
		return c.Update.Message.Chat.ID, nil
	}
	if c.Update.EditedMessage != nil {
		return c.Update.EditedMessage.Chat.ID, nil
	}
	if c.Update.CallbackQuery != nil {
		// If Bale server omits the chat object inside the callback message, fallback to Sender ID
		if c.Update.CallbackQuery.Message != nil && c.Update.CallbackQuery.Message.Chat.ID != 0 {
			return c.Update.CallbackQuery.Message.Chat.ID, nil
		}
		// Fallback safely to Callback Sender ID (perfectly matches Chat ID in PV chats)
		return c.Update.CallbackQuery.From.ID, nil
	}
	return 0, errors.New("cannot determine chat ID")
}

// SenderID extracts the identifier of the message author
func (c *Ctx) SenderID() int64 {
	if c.Update == nil {
		return 0
	}
	if c.Update.Message != nil && c.Update.Message.From != nil {
		return c.Update.Message.From.ID
	}
	if c.Update.EditedMessage != nil && c.Update.EditedMessage.From != nil {
		return c.Update.EditedMessage.From.ID
	}
	if c.Update.CallbackQuery != nil {
		return c.Update.CallbackQuery.From.ID
	}
	if c.Update.PreCheckoutQuery != nil {
		return c.Update.PreCheckoutQuery.From.ID
	}
	return 0
}

// Text returns the normalized message text (Persian/Arabic digits converted to English)
func (c *Ctx) Text() string {
	if c.Message != nil {
		return ToEnDigits(c.Message.Text)
	}
	return ""
}

// RawText returns the original, unmodified raw message text received from the user
func (c *Ctx) RawText() string {
	if c.Message != nil {
		return c.Message.Text
	}
	return ""
}

// Arg parses and returns text command arguments normalized to English digits fluidly
func (c *Ctx) Arg(idx ...int) any {
	if c.Message == nil || c.Message.Text == "" {
		return nil
	}
	normalizedText := ToEnDigits(c.Message.Text)
	parts := strings.Fields(normalizedText)
	if len(parts) <= 1 {
		return nil
	}
	args := parts[1:]
	if len(idx) > 0 {
		i := idx[0]
		if i >= 0 && i < len(args) {
			return args[i]
		}
		return nil
	}
	return args
}

// Session retrieves the active Session for the current chat target
func (c *Ctx) Session() *Session {
	id, _ := c.ChatID()
	return c.Bot.Sessions.Get(id)
}

// Del initiates a message deletion chain in a unified fluent format
func (c *Ctx) Del() *DelChain {
	return &DelChain{c: c}
}

type DelChain struct {
	c   *Ctx
	dur time.Duration
}

// Delay configures the delete action to execute after specified duration in background
func (d *DelChain) Delay(dur time.Duration) *DelChain {
	d.dur = dur
	return d
}

// Go executes the deletion of the message linked to the current context with auto error logging and cumulative analytics
func (d *DelChain) Go() error {
	if d.c.Message == nil {
		return errors.New("no message in context")
	}

	// Skip zero message IDs
	if d.c.Message.MessageID == 0 {
		return nil
	}

	if d.dur > 0 {
		msgID := d.c.Message.MessageID
		id, err := d.c.ChatID()
		if err != nil {
			return err
		}
		d.c.Bot.Task().In(d.dur, func() {
			errDel := d.c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
				"chat_id":    id,
				"message_id": msgID,
			}, nil)
			if errDel == nil {
				d.c.Bot.initAnalyticsDB()
				if d.c.Bot.analyticsDB != nil {
					d.c.Bot.incrementAnalyticsCount(fmt.Sprintf("stat_daily:%d:deletions", id), 1)
					d.c.Bot.incrementAnalyticsCount(fmt.Sprintf("stat_lifetime:%d:deletions", id), 1)
				}

				// Log delayed deletion structurally
				d.c.Bot.Log().Info("حذف پیام با موفقیت (با تاخیر) انجام شد").
					Int64("chat_id", id).
					Int64("message_id", msgID).
					Go()
			}
		})
		return nil
	}

	d.c.mu.RLock()
	if d.c.Keys != nil && d.c.Keys["_sys_msg_deleted"] == true {
		d.c.mu.RUnlock()
		return nil
	}
	d.c.mu.RUnlock()

	id, err := d.c.ChatID()
	if err != nil {
		return err
	}
	errDel := d.c.Bot.BaseRequest(d.c.ctx, "deleteMessage", map[string]any{
		"chat_id":    id,
		"message_id": d.c.Message.MessageID,
	}, nil)

	if errDel != nil {
		logErr(d.c.Bot, "[Message Delete Error] ", errDel)
	} else {
		d.c.mu.Lock()
		if d.c.Keys == nil {
			d.c.Keys = make(map[string]any)
		}
		d.c.Keys["_sys_msg_deleted"] = true
		d.c.mu.Unlock()

		d.c.Bot.initAnalyticsDB()
		if d.c.Bot.analyticsDB != nil {
			d.c.Bot.incrementAnalyticsCount(fmt.Sprintf("stat_daily:%d:deletions", id), 1)
			d.c.Bot.incrementAnalyticsCount(fmt.Sprintf("stat_lifetime:%d:deletions", id), 1)
		}

		// Log instant deletion structurally
		d.c.Bot.Log().Info("حذف پکت شبکه (Outgoing Delete)").
			Int64("chat_id", id).
			Int64("message_id", d.c.Message.MessageID).
			Go()
	}
	return errDel
}

// Answer initiates a callback query answer chain in a unified fluent format
func (c *Ctx) Answer() *AnswerChain {
	return &AnswerChain{c: c}
}

// AnswerChain handles callback query response in a unified fluent style
type AnswerChain struct {
	c    *Ctx
	text string
	show bool
}

// Text attaches informational text to callback query answer
func (a *AnswerChain) Text(t string) *AnswerChain {
	a.text = t
	return a
}

// Alert configures response to be displayed as modal alert dialog box
func (a *AnswerChain) Alert() *AnswerChain {
	a.show = true
	return a
}

// Go executes the callback answer on Bale servers
func (a *AnswerChain) Go() error {
	if a.c.Update == nil || a.c.Update.CallbackQuery == nil {
		return errors.New("no callback query in update")
	}

	// Lock the context mutex safely to mutate the keys map
	a.c.mu.Lock()
	if a.c.Keys == nil {
		a.c.Keys = make(map[string]any)
	}
	// Flag as answered to prevent the auto-answer middleware from double-answering
	a.c.Keys["_sys_cb_answered"] = true
	a.c.mu.Unlock()

	return a.c.Bot.BaseRequest(a.c.ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": a.c.Update.CallbackQuery.ID,
		"text":              a.text,
		"show_alert":        a.show,
	}, nil)
}

// File initializes file management and actions chain using ID and captures safe chat IDs
func (c *Ctx) File(fileID string) *FileChain {
	origName := ""
	// Capture the Chat ID synchronously before context recycling
	chatID, _ := c.ChatID()
	if c.Message != nil {
		if c.Message.Document != nil && c.Message.Document.FileID == fileID {
			origName = c.Message.Document.FileName
		} else if c.Message.Audio != nil && c.Message.Audio.FileID == fileID {
			origName = c.Message.Audio.FileName
		} else if c.Message.Video != nil && c.Message.Video.FileID == fileID {
			origName = c.Message.Video.FileName
		}
	}
	return &FileChain{
		bot:      c.Bot,
		ctx:      c.ctx,
		id:       fileID,
		origName: origName,
		chatID:   chatID,
	}
}

// FileChain provides generic container for file ID scope operations with original name support
type FileChain struct {
	bot      *Bot
	ctx      context.Context
	id       string
	origName string
	chatID   int64
}

// Download starts file downloading fluent chain
func (f *FileChain) Download() *DownloadChain {
	return &DownloadChain{
		fc:   f,
		name: f.origName,
		// Transfer the captured Chat ID
		chatID: f.chatID,
	}
}

// DownloadChain manages physical file write configurations with concurrent pool support
type DownloadChain struct {
	fc         *FileChain
	path       string
	name       string
	onProgress func(percent float64)
	useQueue   bool
	chatID     int64
}

// Name registers a custom filename (including extension) to save the file as
func (d *DownloadChain) Name(n string) *DownloadChain {
	d.name = n
	return d
}

// Path configures directory target to save the file
func (d *DownloadChain) Path(p string) *DownloadChain {
	d.path = p
	return d
}

// OnProgress registers a callback triggered during download progress updates (1% to 100%)
func (d *DownloadChain) OnProgress(fn func(percent float64)) *DownloadChain {
	d.onProgress = fn
	return d
}

// Queue configures the download to run asynchronously in a concurrent background worker queue
func (d *DownloadChain) Queue() *DownloadChain {
	d.useQueue = true
	return d
}

// Go executes download transaction and returns saved path (supports file_id and direct URLs)
func (d *DownloadChain) Go() (string, error) {
	if d.fc.id == "" {
		return "", errors.New("missing file ID or URL")
	}

	var url string
	var fileSize int64

	// Detect if the target ID is a direct HTTP/HTTPS web link
	isDirectURL := strings.HasPrefix(d.fc.id, "http://") || strings.HasPrefix(d.fc.id, "https://")

	if isDirectURL {
		url = d.fc.id
	} else {
		var fileInfo File
		err := d.fc.bot.BaseRequest(d.fc.ctx, "getFile", map[string]any{
			"file_id": d.fc.id,
		}, &fileInfo)
		if err != nil {
			return "", err
		}
		url = "https://tapi.bale.ai/file/bot" + d.fc.bot.Client.token + "/" + fileInfo.FilePath
		fileSize = fileInfo.FileSize
	}

	// Build destination path using sanitized filename
	fileName := d.name
	if fileName == "" {
		fileName = filepath.Base(url)
		// Strip query parameters from URL if present (e.g., ?token=...)
		if idx := strings.Index(fileName, "?"); idx != -1 {
			fileName = fileName[:idx]
		}
		// Decode percent-encoded URL characters safely using the aliased package
		if decoded, err := neturl.PathUnescape(fileName); err == nil {
			fileName = decoded
		}
	}
	repl := strings.NewReplacer(
		":", "_",
		"*", "_",
		"?", "_",
		"|", "_",
		"<", "_",
		">", "_",
		"\"", "_",
	)
	fileName = repl.Replace(fileName)
	destPath := filepath.Join(d.path, fileName)

	if err := os.MkdirAll(d.path, 0755); err != nil {
		return "", err
	}

	// Panic-Proof Context Guard: Ensure context is never nil
	ctx := d.fc.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	resolved := d.fc.bot.ResolveChatID(d.chatID)
	chatIDStr := fmt.Sprintf("%v", resolved)

	// Dispatch job through the concurrent download queue
	if d.useQueue {
		initDownloadPool() // Lazy-load the pool at package level
		resultChan := make(chan error, 1)

		job := &DownloadJob{
			url:        url,
			destPath:   destPath,
			totalSize:  fileSize,
			ctx:        ctx,
			client:     d.fc.bot.Client.httpClient,
			onProgress: d.onProgress,
			resultChan: resultChan,
			chatID:     chatIDStr,
		}

		globalDownloadPool.jobChan <- job
		err := <-resultChan

		// Publish file.download event on successful background queued download with ChatID
		if err == nil {
			d.fc.bot.Bus.Publish("file.download", map[string]any{
				"Path":   destPath,
				"URL":    url,
				"ChatID": d.chatID, // Wrapped cleanly here
			})
		}
		return destPath, err
	}

	// Standard resilient download without queue
	err := resilientDownload(ctx, d.fc.bot.Client.httpClient, url, destPath, fileSize, d.onProgress)

	// Publish file.download event on successful standard download with ChatID
	if err == nil {
		d.fc.bot.Bus.Publish("file.download", map[string]any{
			"Path":   destPath,
			"URL":    url,
			"ChatID": d.chatID, // Wrapped cleanly here
		})
	}
	return destPath, err
}

// Send opens the fluent sending dot system inside the handler context with clean Ctx reference
func (c *Ctx) Send() *SendChain {
	id, _ := c.ChatID()
	return &SendChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		c:    c,
		chat: id,
	}
}

// DeepLink parses and returns the start payload parameter if present
func (c *Ctx) DeepLink() string {
	if c.Message == nil || !strings.HasPrefix(c.Message.Text, "/start ") {
		return ""
	}
	parts := strings.Fields(c.Message.Text)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// T translates a key based on user language preferences
func (c *Ctx) T(key string) string {
	if c.Bot.i18n == nil || c.Message == nil || c.Message.From == nil {
		return key
	}
	lang := c.Message.From.LanguageCode
	if lang == "" {
		lang = "fa"
	}
	if dict, ok := c.Bot.i18n[lang]; ok {
		if val, ok := dict[key]; ok {
			return val
		}
	}
	if dict, ok := c.Bot.i18n["fa"]; ok {
		if val, ok := dict[key]; ok {
			return val
		}
	}
	return key
}

// ScanValues converts string arrays to concrete Go variables portably
func ScanValues(args []string, sep string, targets ...any) error {
	if len(args) < len(targets) {
		return fmt.Errorf("not enough arguments: expected %d, got %d", len(targets), len(args))
	}
	for i, target := range targets {
		arg := args[i]
		if i == len(targets)-1 {
			if strPtr, ok := target.(*string); ok {
				*strPtr = strings.Join(args[i:], sep)
				return nil
			}
		}
		switch ptr := target.(type) {
		case *string:
			*ptr = arg
		case *int:
			val, err := strconv.Atoi(arg)
			if err != nil {
				return err
			}
			*ptr = val
		case *int64:
			val, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return err
			}
			*ptr = val
		case *float64:
			val, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				return err
			}
			*ptr = val
		case *bool:
			val, err := strconv.ParseBool(arg)
			if err != nil {
				return err
			}
			*ptr = val
		case *time.Duration:
			val, err := ParseDuration(arg)
			if err != nil {
				return err
			}
			*ptr = val
		default:
			return fmt.Errorf("unsupported scan target type: %T", target)
		}
	}
	return nil
}

// ScanArgs parses command text parameters directly into given Go variable pointers
func (c *Ctx) ScanArgs(targets ...any) error {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return errors.New("no arguments found")
	}
	return ScanValues(args, " ", targets...)
}

// ScanCallbackArgs parses colon-separated callback parameters directly into pointers
func (c *Ctx) ScanCallbackArgs(targets ...any) error {
	if c.Update == nil || c.Update.CallbackQuery == nil {
		return errors.New("not a callback query")
	}
	parts := strings.Split(c.Update.CallbackQuery.Data, ":")
	if len(parts) <= 1 {
		return errors.New("no callback arguments found")
	}
	return ScanValues(parts[1:], ":", targets...)
}

// IsPrivate checks if current chat is a private direct message with the bot
func (c *Ctx) IsPrivate() bool {
	return c.Message != nil && c.Message.Chat.Type == "private"
}

// IsGroup checks if current chat is a regular group or supergroup
func (c *Ctx) IsGroup() bool {
	if c.Message == nil {
		return false
	}
	t := c.Message.Chat.Type
	return t == "group" || t == "supergroup"
}

// IsChannel checks if current chat is a channel
func (c *Ctx) IsChannel() bool {
	return c.Message != nil && c.Message.Chat.Type == "channel"
}

// Go executes an asynchronous background task safely with panic recovery and semaphore throttling limits
func (c *Ctx) Go(task func()) {
	bot := c.Bot
	bot.bgSemaphore <- struct{}{}
	go func() {
		defer func() {
			<-bot.bgSemaphore
			if r := recover(); r != nil {
				handlePanic(bot, r, nil)
			}
		}()
		task()
	}()
}

// IsSuperGroup checks if the chat has supergroup capabilities
func (c *Ctx) IsSuperGroup() bool {
	if c.Message == nil {
		return false
	}
	// Bale quirk: check for explicit type OR existence of a username/thread
	return c.Message.Chat.Type == "supergroup" ||
		c.Message.Chat.Username != "" ||
		c.Message.MessageThreadID != 0
}

// IsOwner checks if the current message sender is the registered global bot administrator thread-safely
func (c *Ctx) IsOwner() bool {
	c.Bot.mu.RLock()
	defer c.Bot.mu.RUnlock()
	return c.SenderID() == c.Bot.MaintenanceAdminID
}

// Typing triggers typing chat action on Bale servers
func (c *Ctx) Typing() {
	_, _ = c.Action().Typing().Go()
}

// UploadingPhoto triggers upload_photo chat action on Bale servers
func (c *Ctx) UploadingPhoto() {
	_, _ = c.Action().UploadPhoto().Go()
}

// UploadingDocument triggers upload_document chat action on Bale servers
func (c *Ctx) UploadingDocument() {
	_, _ = c.Action().UploadDoc().Go()
}

// File opens the fluent file management dot chain from the Bot context safely
func (b *Bot) File(fileID string) *FileChain {
	return &FileChain{
		bot: b,
		ctx: context.Background(),
		id:  fileID,
	}
}

// Info initiates a fluent file metadata query chain without downloading the actual bytes
func (f *FileChain) Info() *FileInfoChain {
	return &FileInfoChain{fc: f}
}

// FileInfoChain handles fluent queries for file metadata ending with terminal Go
type FileInfoChain struct {
	fc *FileChain
}

// Go executes the file metadata query on Bale servers and returns File info
func (fi *FileInfoChain) Go() (*File, error) {
	if fi.fc.id == "" {
		return nil, errors.New("empty file ID")
	}
	var info File
	err := fi.fc.bot.BaseRequest(fi.fc.ctx, "getFile", map[string]any{
		"file_id": fi.fc.id,
	}, &info)
	if err != nil {
		logErr(fi.fc.bot, "[File Info Error] ", err)
	}
	return &info, err
}

// PrevText returns original message text before being edited
func (c *Ctx) PrevText() string {
	return c.prevText
}

// ScanOptionalArgs parses up to the number of arguments provided without returning errors for missing ones
func (c *Ctx) ScanOptionalArgs(targets ...any) int {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return 0
	}

	limit := len(targets)
	if len(args) < limit {
		// shrink to available args, prevents out-of-range slicing
		limit = len(args)
	}

	_ = ScanValues(args[:limit], " ", targets[:limit]...)
	return limit
}

// Reset clears all fields to prevent context pollution in sync.Pool
func (c *Ctx) Reset() {
	c.Bot = nil
	c.Update = nil
	c.Message = nil
	c.handlers = nil
	c.index = -1
	c.err = nil
	c.prevText = ""
	c.ctx = nil

	// Avoid map re-allocation by clearing existing keys
	if c.Keys != nil {
		for k := range c.Keys {
			delete(c.Keys, k)
		}
	}
}

// ChatID explicitly registers a chat ID for the download chain
func (d *DownloadChain) ChatID(id int64) *DownloadChain {
	d.chatID = id
	return d
}

// GetEntityText extracts the actual text of a specific message entity safely
func (c *Ctx) GetEntityText(e MessageEntity) string {
	if c.Message == nil || c.Message.Text == "" {
		return ""
	}

	// Convert raw string to rune slice to align with Bale's offset calculation
	runes := []rune(c.Message.Text)
	start := e.Offset
	end := e.Offset + e.Length

	if start < 0 || end > len(runes) {
		return ""
	}

	return string(runes[start:end])
}

// FindLinks manually scans the message text for URLs using Regex when Bale fails to provide entities
func (c *Ctx) FindLinks() []string {
	if c.Message == nil || c.Message.Text == "" {
		return nil
	}

	// 1. First, try to get URLs from Bale's official entities (if any)
	var links []string
	for _, e := range c.Message.Entities {
		if e.Type == "url" || e.Type == "text_link" {
			if e.URL != "" {
				links = append(links, e.URL)
			} else {
				links = append(links, c.GetEntityText(e))
			}
		}
	}

	// 2. If no official entities found, perform a deep regex scan on raw text
	// This captures [text](url) and raw links that Bale missed
	rawMatches := rxLinkPattern.FindAllString(c.Message.Text, -1)
	links = append(links, rawMatches...)

	return links
}

// Data returns the callback query data or a specific parameter if index is provided
func (c *Ctx) Data(idx ...int) string {
	if c.Update == nil || c.Update.CallbackQuery == nil {
		return ""
	}

	raw := c.Update.CallbackQuery.Data
	if len(idx) == 0 {
		return raw
	}

	// Split by colon which is the framework's standard separator
	parts := strings.Split(raw, ":")
	i := idx[0]
	if i >= 0 && i < len(parts) {
		return parts[i]
	}

	return ""
}

// IsAnonymous checks if the message was sent by a hidden admin
func (c *Ctx) IsAnonymous() bool {
	if c.Message == nil {
		return false
	}
	// In Bale, anonymous messages have a SenderChat matching the Chat ID
	return c.Message.SenderChat != nil && c.Message.SenderChat.ID == c.Message.Chat.ID
}

// ThreadID returns the ID of the forum topic/thread if applicable
func (c *Ctx) ThreadID() int64 {
	if c.Message != nil {
		return c.Message.MessageThreadID
	}
	return 0
}

// IsTopicMessage checks if the message belongs to a specific forum topic
func (c *Ctx) IsTopicMessage() bool {
	return c.ThreadID() != 0
}

// BotCanPromote checks if the bot itself has permission to add new admins
func (c *Ctx) BotCanPromote() bool {
	if c.Message == nil {
		return false
	}

	// 1. Execute Me() to get bot's identity safely
	me, errMe := c.Bot.Me().Go()
	if errMe != nil {
		return false
	}

	// 2. Execute Member() to get bot's status in the current group
	member, errMem := c.Bot.Chat(c.Message.Chat.ID).Member(me.ID).Go()
	if errMem != nil {
		return false
	}

	// 3. Return the specific permission flag
	return member.CanPromoteMembers
}

// SendSettings is a shortcut helper to send the settings panel natively with an auto-locked owner check and dynamic recovery
func (c *Ctx) SendSettings(text string, layout [][]string, closeCallback string) (*Message, error) {
	// Lock the settings interaction to the command executor dynamically
	c.SetData("active_settings_admin_id", c.SenderID())

	// Cache the custom panel data inside session to allow dynamic restorations on cancel/confirm
	c.SetData("custom_settings_text", text)
	c.SetData("custom_settings_layout", layout)
	c.SetData("custom_settings_close", closeCallback)

	// Fetch dynamic group title natively using getChat
	title := "گروه بدون نام"
	if info, errInfo := c.Chat().Info().Go(); errInfo == nil && info != nil {
		if info.Title != "" {
			title = info.Title
		} else if info.FirstName != "" {
			title = info.FirstName
		}
	}

	// Format the text by replacing {title} placeholder dynamically
	formattedText := strings.ReplaceAll(text, "{title}", title)

	// Compile the matrix keyboard natively
	builder := InlineMarkup()
	for _, rowKeys := range layout {
		var row []any
		for _, k := range rowKeys {
			row = append(row, c.SettingBtn(k))
		}
		builder.Row(row...)
	}
	builder.Row(Btn("❌ بستن پنل").Callback(closeCallback))

	return c.Send().Text(formattedText).Markup(builder.Build()).Markdown().Go()
}

// SettingBtn creates a fully configured InlineButtonBuilder for a setting, dynamically mapped to current status (with automatic remote target detection)
func (c *Ctx) SettingBtn(key string, targetChat ...any) *InlineButtonBuilder {
	var resolved any
	if len(targetChat) > 0 && targetChat[0] != nil {
		resolved = c.Bot.ResolveChatID(targetChat[0])
	} else {
		// Automatically check if a target chat ID was passed as first command argument (for remote PV settings!)
		if arg := c.ArgString(0); arg != "" {
			resolved = c.Bot.ResolveChatID(arg)
		} else {
			chatID, _ := c.ChatID()
			resolved = c.Bot.ResolveChatID(chatID)
		}
	}

	// Retrieve setting metadata
	c.Bot.mu.RLock()
	var entry *SettingEntry
	for i := range c.Bot.settings {
		if c.Bot.settings[i].Key == key {
			entry = &c.Bot.settings[i]
			break
		}
	}
	c.Bot.mu.RUnlock()

	if entry == nil {
		return Btn(key).Callback(fmt.Sprintf("_sys_cfg:%s:%v", key, resolved))
	}

	// Read active state from database
	dbKey := fmt.Sprintf("group_config_%v_%s", resolved, key)
	active := entry.Default
	if val, ok := c.Bot.dbInstance.Get(dbKey); ok {
		if bVal, okBool := val.(bool); okBool {
			active = bVal
		}
	}

	emoji := "🔴"
	if active {
		emoji = "🟢"
	}

	btnText := fmt.Sprintf("%s %s", emoji, entry.Label)
	return Btn(btnText).Callback(fmt.Sprintf("_sys_cfg:%s:%v", key, resolved))
}

// ToggleSetting toggles or sets a registered setting natively, supporting optional target chats for remote management
func (c *Ctx) ToggleSetting(key string, state string, targetChat ...any) (any, error) {
	// Normalize key natively to prevent case-sensitivity and spacing bugs completely
	key = strings.ToLower(strings.TrimSpace(key))

	// Resolve target chat ID with secure type assertions to prevent empty interface/string clashes
	var resolved any
	if len(targetChat) > 0 && targetChat[0] != nil {
		if str, okStr := targetChat[0].(string); okStr {
			cleanStr := strings.TrimSpace(str)
			if cleanStr != "" {
				resolved = c.Bot.ResolveChatID(cleanStr)
			}
		} else {
			resolved = c.Bot.ResolveChatID(targetChat[0])
		}
	}
	if resolved == nil || resolved == "" {
		id, _ := c.ChatID()
		resolved = c.Bot.ResolveChatID(id)
	}

	db := c.Bot.dbInstance
	dbKey := fmt.Sprintf("group_config_%v_%s", resolved, key)

	// Find setting entry (Safe to read concurrently without locks as settings is read-only after startup)
	var entry *SettingEntry
	for i := range c.Bot.settings {
		if c.Bot.settings[i].Key == key {
			entry = &c.Bot.settings[i]
			break
		}
	}
	if entry == nil {
		return false, fmt.Errorf("setting key %q not found", key)
	}

	// Determine active state
	current := entry.Default
	if val, ok := db.Get(dbKey); ok {
		if bVal, okBool := val.(bool); okBool {
			current = bVal
		}
	}

	var nextState any = !current
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "on" || state == "1" || state == "true" || state == "active" {
		nextState = true
	} else if state == "off" || state == "0" || state == "false" || state == "inactive" {
		nextState = false
	} else if state != "" {
		// Store custom duration strings directly inside GOB DB (e.g. "10m", "30s", "1h")
		nextState = state
	}

	err := db.Set(dbKey, nextState)
	return nextState, err
}

// GetBool retrieves a boolean setting value natively from GOB DB, falling back to registered default
func (c *Ctx) GetBool(key string) bool {
	chatID, err := c.ChatID()
	if err != nil {
		return false
	}
	dbKey := fmt.Sprintf("group_config_%d_%s", chatID, key)

	// Fallback to registered default natively
	c.Bot.mu.RLock()
	defaultVal := false
	for _, entry := range c.Bot.settings {
		if entry.Key == key {
			defaultVal = entry.Default
			break
		}
	}
	c.Bot.mu.RUnlock()

	if val, ok := c.Bot.dbInstance.Get(dbKey); ok {
		if bVal, okBool := val.(bool); okBool {
			return bVal
		}
	}
	return defaultVal
}

// ChatTitle natively retrieves and caches the dynamic title of the target group (supporting optional remote overrides)
func (c *Ctx) ChatTitle(targetChat ...any) string {
	var resolved any
	if len(targetChat) > 0 && targetChat[0] != nil && targetChat[0] != "" {
		resolved = c.Bot.ResolveChatID(targetChat[0])
	} else {
		// Only parse ArgString(0) as remote chat ID if we are in PV and the arg is a valid numeric ID
		if c.IsPrivate() && c.ArgString(0) != "" {
			arg := c.ArgString(0)
			if strings.HasPrefix(arg, "-") || (len(arg) > 0 && arg[0] >= '0' && arg[0] <= '9') {
				resolved = c.Bot.ResolveChatID(arg)
			}
		}
		if resolved == nil || resolved == "" {
			id, _ := c.ChatID()
			resolved = c.Bot.ResolveChatID(id)
		}
	}

	cacheKey := fmt.Sprintf("chat_title:%v", resolved)
	if cachedVal, ok := c.Bot.Cache().Get(cacheKey).Go(); ok {
		if str, okStr := cachedVal.(string); okStr {
			return str
		}
	}

	title := "بدون نام" // Fixed default fallback to prevent "group group" duplication
	if info, err := c.Bot.Chat(resolved).Info().Go(); err == nil && info != nil {
		if info.Title != "" {
			title = info.Title
		} else if info.FirstName != "" {
			title = info.FirstName
		}
		c.Bot.Cache().Set(cacheKey, title, 12*time.Hour).Go()
	}
	return title
}

// ToggleChain handles fluent configurations for dynamic settings toggling natively
type ToggleChain struct {
	c          *Ctx
	successMsg string
	errorMsg   string
	invalidMsg string
	statusOn   string
	statusOff  string
}

// Toggle opens the fluent settings toggling chain natively with smart defaults
func (c *Ctx) Toggle() *ToggleChain {
	return &ToggleChain{
		c:          c,
		successMsg: "✅ تنظیم `%s` با موفقیت به حالت `%s` تغییر یافت.",
		errorMsg:   "❌ خطایی رخ داد: %v",
		invalidMsg: "⚠️ دستور نامعتبر! مثال:\n`/toggle lock_sticker on`\n`/toggle lock_gif off 4542691229` (Remote)",
		statusOn:   "🟢 روشن",
		statusOff:  "🔴 خاموش",
	}
}

// Success overrides the default success message template
func (t *ToggleChain) Success(msg string) *ToggleChain { t.successMsg = msg; return t }

// Error overrides the default error message template
func (t *ToggleChain) Error(msg string) *ToggleChain { t.errorMsg = msg; return t }

// Invalid overrides the default invalid command warning template
func (t *ToggleChain) Invalid(msg string) *ToggleChain { t.invalidMsg = msg; return t }

// StatusOn overrides the active status label natively (defaults to 🟢 روشن)
func (t *ToggleChain) StatusOn(s string) *ToggleChain { t.statusOn = s; return t }

// StatusOff overrides the inactive status label natively (defaults to 🔴 خاموش)
func (t *ToggleChain) StatusOff(s string) *ToggleChain { t.statusOff = s; return t }

// Go executes the dynamic toggle process natively with customized or default templates
func (t *ToggleChain) Go() (*Message, error) {
	key := t.c.ArgString(0)
	state := t.c.ArgString(1)
	chat := t.c.ArgString(2)

	if key == "" {
		return t.c.ReplyText(t.invalidMsg)
	}

	active, err := t.c.ToggleSetting(key, state, chat)
	if err != nil {
		return t.c.ReplyText(fmt.Sprintf(t.errorMsg, err.Error()))
	}

	status := t.statusOff
	if activeBool, ok := active.(bool); ok && activeBool {
		status = t.statusOn
	} else if activeStr, ok := active.(string); ok && activeStr != "" {
		status = fmt.Sprintf("%s (%s)", t.statusOn, activeStr)
	}

	return t.c.ReplyText(fmt.Sprintf(t.successMsg, key, status))
}

// DownloadFile is a shortcut helper to natively download any file by its ID into a local path in one line
func (c *Ctx) DownloadFile(fileID string, path string, name ...string) (string, error) {
	dl := c.File(fileID).Download().Path(path)
	if len(name) > 0 {
		dl = dl.Name(name[0])
	}
	return dl.Go()
}

// ClearSession destroys the active session data and resets FSM states in one line
func (c *Ctx) ClearSession() {
	id, _ := c.ChatID()
	c.Bot.Sessions.Clear(id)
}

// Emit is a shortcut helper to publish an event concurrently to the central EventBus
func (c *Ctx) Emit(topic string, payload any) {
	c.Bot.Bus.Publish(topic, payload)
}

// GetActiveFile extracts the file ID and dynamic original file name of any active media in the message
func (c *Ctx) GetActiveFile() (string, string, error) {
	msg := c.Message
	if msg == nil {
		return "", "", errors.New("no message in context")
	}

	// Automatically fallback to replied-to message if the trigger message contains no downloadable media
	hasMedia := msg.Document != nil || msg.Video != nil || msg.Audio != nil || len(msg.Photo) > 0 ||
		msg.Voice != nil || msg.Sticker != nil || msg.Animation != nil

	if !hasMedia && msg.ReplyToMessage != nil {
		msg = msg.ReplyToMessage
	}

	// Helper to extract a unique 12-character hash from the actual unique field of the FileID
	hash := func(id string) string {
		// 1. If it's a colon-separated Bale ID, extract the second field which is the unique file identifier
		parts := strings.Split(id, ":")
		if len(parts) >= 2 {
			clean := strings.TrimPrefix(parts[1], "-")
			if len(clean) > 12 {
				return clean[len(clean)-12:] // Get the last 12 digits of the actual unique field
			}
			return clean
		}
		// 2. Fallback for standard base64 Telegram file IDs
		if len(id) < 12 {
			return id
		}
		return id[len(id)-12:]
	}

	var fileID string
	var fileName string
	var mimeType string
	var defaultPrefix string

	// Extract raw fields dynamically based on the active media type in the packet
	if msg.Animation != nil {
		fileID = msg.Animation.FileID
		mimeType = msg.Animation.MimeType
		defaultPrefix = "animation"
		// Always ignore the static duplicate file name of client, and force-generate based on unique FileID hash
		fileName = ""
	} else if msg.Document != nil {
		fileID = msg.Document.FileID
		fileName = msg.Document.FileName
		mimeType = msg.Document.MimeType
		defaultPrefix = "file"
	} else if msg.Video != nil {
		fileID = msg.Video.FileID
		fileName = msg.Video.FileName
		mimeType = msg.Video.MimeType
		defaultPrefix = "video"
	} else if msg.Audio != nil {
		fileID = msg.Audio.FileID
		fileName = msg.Audio.FileName
		mimeType = msg.Audio.MimeType
		defaultPrefix = "audio"
	} else if len(msg.Photo) > 0 {
		largest := msg.Photo.Largest()
		fileID = largest.FileID
		mimeType = "image/jpeg"
		defaultPrefix = "photo"
	} else if msg.Voice != nil {
		fileID = msg.Voice.FileID
		mimeType = msg.Voice.MimeType
		defaultPrefix = "voice"
	} else if msg.Sticker != nil {
		fileID = msg.Sticker.FileID
		mimeType = "image/webp"
		defaultPrefix = "sticker"
	}

	if fileID == "" {
		return "", "", errors.New("no downloadable media found in this message")
	}

	// Resolve extension using a robust, fallback-safe bidirectional logic
	ext := ""
	if fileName != "" {
		ext = filepath.Ext(fileName)
	}

	// If the filename has no extension (or empty), resolve it dynamically from the MIME subtype
	if ext == "" && mimeType != "" {
		parts := strings.Split(strings.ToLower(mimeType), "/")
		if len(parts) == 2 {
			subtype := parts[1]
			switch subtype {
			case "jpeg", "jpg":
				ext = ".jpg"
			case "mpeg":
				ext = ".mp3"
			case "plain":
				ext = ".txt"
			case "octet-stream":
				ext = ".bin"
			case "quicktime":
				ext = ".mov"
			case "x-gif":
				ext = ".gif"
			default:
				ext = "." + subtype
			}
		}
	}

	// Final fallback if both methods yielded no extension
	if ext == "" {
		ext = ".dat"
	}

	// Build the final filename, ensuring we append the extension if the original filename lacks one
	if fileName != "" {
		if filepath.Ext(fileName) == "" {
			fileName = fileName + ext
		}
	} else {
		fileName = fmt.Sprintf("%s_%s%s", defaultPrefix, hash(fileID), ext)
	}

	return fileID, fileName, nil
}

// ProcessReferral parses the start deep-link, prevents self-referrals, records the invitation natively, and returns the inviter's ID
func (c *Ctx) ProcessReferral() (int64, bool) {
	code := c.DeepLink()
	if code == "" {
		return 0, false
	}

	// Parse inviter numeric ID (deep link usually holds inviter's ID, e.g. "/start 300075772")
	inviterID, err := strconv.ParseInt(code, 10, 64)
	if err != nil || inviterID <= 0 {
		return 0, false
	}

	// Prevent self-referral fraud natively (users cannot invite themselves)
	if inviterID == c.SenderID() {
		return 0, false
	}

	dbKey := fmt.Sprintf("inviter_for:%d", c.SenderID())

	// Check if this user was already invited previously (prevents double-credit fraud globally)
	if _, ok := c.DBGet(dbKey); ok {
		return 0, false
	}

	// Store who invited this user globally in the GOB database
	_ = c.DBSet(dbKey, inviterID)

	// Natively increment the inviter's global successful invites count
	invitesKey := fmt.Sprintf("invites_count:%d", inviterID)
	currentInvites := int64(0)
	if val, ok := c.DBGet(invitesKey); ok {
		if num, okNum := AsInt64(val); okNum {
			currentInvites = num
		}
	}
	_ = c.DBSet(invitesKey, currentInvites+1)

	// Publish referral event concurrently to the EventBus
	c.Emit("user.referral", map[string]any{
		"InviterID": inviterID,
		"InvitedID": c.SenderID(),
	})

	return inviterID, true
}

// ReferralLink dynamically generates the secure referral link of the current user for this bot
func (c *Ctx) ReferralLink() (string, error) {
	// Fetch bot profile natively using Me() helper
	me, err := c.Bot.Me().Go()
	if err != nil {
		c.Bot.Log().Error("ReferralLink failed to query bot profile").Err(err).Go()
		return "", err
	}
	// Construct the correct Bale compliant deep link using bot username and sender ID
	return fmt.Sprintf("https://ble.ir/%s?start=%d", me.Username, c.SenderID()), nil
}

// DownloadAvatar is a shortcut helper to natively download any user's profile photo in one line
func (c *Ctx) DownloadAvatar(userID int64, path string, name ...string) (string, error) {
	var chatInfo ChatFullInfo
	err := c.Bot.BaseRequest(c.ctx, "getChat", map[string]any{
		"chat_id": userID,
	}, &chatInfo)
	if err != nil {
		return "", err
	}

	// Verify if the target user actually has an active profile photo set on Bale
	if chatInfo.Photo == nil || (chatInfo.Photo.BigFileID == "" && chatInfo.Photo.SmallFileID == "") {
		return "", fmt.Errorf("user %d has no profile photo", userID)
	}

	// Prioritize the highest resolution BigFileID (640x640), fallback to SmallFileID if empty
	fileID := chatInfo.Photo.BigFileID
	if fileID == "" {
		fileID = chatInfo.Photo.SmallFileID
	}

	fileName := fmt.Sprintf("avatar_%d.jpg", userID)
	if len(name) > 0 {
		fileName = name[0]
	}

	return c.DownloadFile(fileID, path, fileName)
}

// SendAvatar sends the target user's profile photo natively with a custom caption, and returns the file ID for optional downloads
func (c *Ctx) SendAvatar(userID ...int64) (string, *Message, error) {
	var targetID int64
	var err error
	if len(userID) > 0 {
		targetID = userID[0]
	} else {
		targetID, err = c.TargetUser()
		if err != nil {
			targetID = c.SenderID()
		}
	}

	// Fetch target user metadata natively using getChat
	var chatInfo ChatFullInfo
	err = c.Bot.BaseRequest(c.ctx, "getChat", map[string]any{
		"chat_id": targetID,
	}, &chatInfo)
	if err != nil {
		return "", nil, err
	}

	// Verify if the target user actually has an active profile photo set on Bale
	if chatInfo.Photo == nil || (chatInfo.Photo.BigFileID == "" && chatInfo.Photo.SmallFileID == "") {
		return "", nil, fmt.Errorf("user %d has no profile photo", targetID)
	}

	// Prioritize the highest resolution BigFileID (640x640), fallback to SmallFileID if empty
	fileID := chatInfo.Photo.BigFileID
	if fileID == "" {
		fileID = chatInfo.Photo.SmallFileID
	}

	// Extract actual display name
	name := chatInfo.FirstName
	if chatInfo.LastName != "" {
		name += " " + chatInfo.LastName
	}
	if name == "" {
		name = fmt.Sprintf("User %d", targetID)
	}

	// Build a beautiful Bale specific mention link
	userLink := Link(name, fmt.Sprintf("uid:%d", targetID))
	caption := fmt.Sprintf("👤 **کاربر:** %s\n🆔 **شناسه:** `%d`", userLink, targetID)

	// Send the photo instantly using the existing fileID natively on Bale's servers
	msg, errSend := c.Send().
		Photo(fileID).
		Caption(caption).
		Markdown().
		Go()

	return fileID, msg, errSend
}

// ModLogChain handles fluent configurations for dynamic moderation logging natively with auto-expiry
type ModLogChain struct {
	c          *Ctx
	title      string
	targetID   int64
	executorID int64
	reason     string
	duration   time.Duration
	markup     any
	expireDur  time.Duration // Added for global automatic expiration tracking
	expireText string        // Added for custom expired status label
}

// ModLog opens the fluent, highly customizable moderation logging chain natively
func (c *Ctx) ModLog(title string) *ModLogChain {
	executorID := int64(0)
	if c.Message != nil && c.Message.From != nil {
		executorID = c.Message.From.ID
	}
	return &ModLogChain{
		c:          c,
		title:      title,
		executorID: executorID,
		reason:     "ثبت نشده",
	}
}

// Target sets the target user ID for the log, automatically resolving their cached display name
func (m *ModLogChain) Target(userID int64) *ModLogChain {
	m.targetID = userID
	return m
}

// Executor sets a custom executor user ID (or 0 for automated system actions)
func (m *ModLogChain) Executor(userID int64) *ModLogChain {
	m.executorID = userID
	return m
}

// Reason sets the reason of the moderation action
func (m *ModLogChain) Reason(r string) *ModLogChain {
	if r != "" {
		m.reason = r
	}
	return m
}

// For sets a native duration of the restriction to be printed in the log
func (m *ModLogChain) For(d time.Duration) *ModLogChain {
	m.duration = d
	return m
}

// ExpireIn automatically schedules the ModLog message to be edited in-place (removing buttons and appending expired text) after a duration
func (m *ModLogChain) ExpireIn(d time.Duration, text string) *ModLogChain {
	m.expireDur = d
	m.expireText = text
	return m
}

// UnmuteBtn automatically compiles and appends the native Unmute inline button
func (m *ModLogChain) UnmuteBtn() *ModLogChain {
	chatID, _ := m.c.ChatID()
	m.markup = InlineMarkup().Row(Btn("🔊 رفع سکوت (Unmute)").Callback(fmt.Sprintf("_sys_modlog:unmute:%d:%d", chatID, m.targetID))).Build()
	return m
}

// UnbanBtn automatically compiles and appends the native Unban inline button
func (m *ModLogChain) UnbanBtn() *ModLogChain {
	chatID, _ := m.c.ChatID()
	m.markup = InlineMarkup().Row(Btn("✅ رفع مسدودسازی (Unban)").Callback(fmt.Sprintf("_sys_modlog:unban:%d:%d", chatID, m.targetID))).Build()
	return m
}

// UnwarnBtn automatically compiles and appends the native Unwarn inline button
func (m *ModLogChain) UnwarnBtn() *ModLogChain {
	chatID, _ := m.c.ChatID()
	m.markup = InlineMarkup().Row(Btn("📉 بخشش اخطار (Unwarn)").Callback(fmt.Sprintf("_sys_modlog:unwarn:%d:%d", chatID, m.targetID))).Build()
	return m
}

// Go compiles, formats, and dispatches the customized ModLog report asynchronously to the configured channel
func (m *ModLogChain) Go() {
	if m.c.Bot.modLogChatID == nil || m.c.Bot.modLogChatID == "" {
		return
	}

	botInstance := m.c.Bot
	modLogChat := botInstance.modLogChatID
	markupCopy := m.markup
	expireDur := m.expireDur
	expireText := m.expireText
	title := m.title
	targetID := m.targetID
	executorID := m.executorID
	reason := m.reason
	duration := m.duration
	chatTitle := m.c.ChatTitle()

	go func() {
		defer func() { recover() }()

		db := botInstance.dbInstance
		dbConcrete, ok := db.(*Database)
		if !ok || dbConcrete == nil {
			return
		}

		// Resolve executor ID safely: if system triggers it, use Group Creator or Bot Owner
		if executorID == targetID {
			chatID, errChat := m.c.ChatID()
			if errChat == nil {
				creatorID := int64(0)
				var admins []ChatMember
				errAdmins := botInstance.BaseRequest(context.Background(), "getChatAdministrators", map[string]any{"chat_id": chatID}, &admins)
				if errAdmins == nil {
					for _, adm := range admins {
						if adm.Status == "creator" {
							creatorID = adm.User.ID
							break
						}
					}
				}
				if creatorID > 0 {
					executorID = creatorID
				} else {
					executorID = botInstance.MaintenanceAdminID
				}
			} else {
				executorID = botInstance.MaintenanceAdminID
			}
		}

		resolveName := func(uid int64) string {
			if uid <= 0 {
				return ""
			}
			dbConcrete.mu.RLock()
			cachedVal, okName := dbConcrete.store[fmt.Sprintf("user_name:%d", uid)]
			dbConcrete.mu.RUnlock()
			if okName {
				if str, okStr := cachedVal.(string); okStr && str != "" {
					return str
				}
			}
			var chatInfo ChatFullInfo
			err := botInstance.BaseRequest(context.Background(), "getChat", map[string]any{"chat_id": uid}, &chatInfo)
			if err == nil {
				name := chatInfo.FirstName
				if chatInfo.LastName != "" {
					name += " " + chatInfo.LastName
				}
				if name != "" {
					dbConcrete.mu.Lock()
					dbConcrete.store[fmt.Sprintf("user_name:%d", uid)] = name
					dbConcrete.appendWAL(walEntry{Op: walSet, Key: fmt.Sprintf("user_name:%d", uid), Val: name})
					dbConcrete.mu.Unlock()
					return name
				}
			}
			return fmt.Sprintf("User %d", uid)
		}

		executorLink := "🤖 سیستم خودکار ربات"
		if executorID > 0 {
			executorLink = Link(resolveName(executorID), fmt.Sprintf("uid:%d", executorID))
		}
		targetLink := "نامشخص"
		if targetID > 0 {
			targetLink = Link(resolveName(targetID), fmt.Sprintf("uid:%d", targetID))
		}

		report := Text().Line(title).Line()
		if targetID > 0 {
			report.Line("👤 **کاربر هدف:** ", targetLink).Line("🆔 **شناسه عددی:** `", fmt.Sprintf("%d", targetID), "`")
		}
		if duration > 0 {
			report.Line("🕒 **مدت زمان:** `", duration.String(), "`")
		}
		report.Line("👮‍♂️ **مجری:** ", executorLink).Line("📝 **علت:** *", reason, "*").Line("📢 **مکان:** ", chatTitle)

		reportStr := report.Go()
		logMsg, err := botInstance.SendModLog(reportStr, markupCopy)

		// Handle automatic in-place visual expiration of the ModLog message
		if err == nil && logMsg != nil && expireDur > 0 && expireText != "" {
			logMsgID := logMsg.MessageID
			botInstance.Task().In(expireDur, func() {
				resolvedText := fmt.Sprintf("%s\n\n🔄 **وضعیت:** %s", reportStr, expireText)
				_ = botInstance.BaseRequest(context.Background(), "editMessageText", map[string]any{
					"chat_id":      botInstance.ResolveChatID(modLogChat),
					"message_id":   logMsgID,
					"text":         resolvedText,
					"parse_mode":   "Markdown",
					"reply_markup": nil, // Explicitly pass nil to delete/remove the buttons!
				}, nil)
			})
		}
	}()
}

// ConfigToggleBtn automatically compiles and appends an inline button that, when clicked by the owner, disables/unlocks the specified setting key in GOB DB
func (m *ModLogChain) ConfigToggleBtn(key string, btnLabel string, successLabel string) *ModLogChain {
	chatID, _ := m.c.ChatID()
	m.markup = InlineMarkup().Row(
		Btn(btnLabel).Callback(fmt.Sprintf("_sys_modlog:config_toggle:%d:%s:%s", chatID, key, successLabel)),
	).Build()
	return m
}

// ArgsTail joins all command arguments starting from startIdx to the end as a single space-separated string
func (c *Ctx) ArgsTail(startIdx int, fallback ...string) string {
	args, ok := c.Arg().([]string)
	if !ok || startIdx < 0 || startIdx >= len(args) {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return ""
	}
	return strings.Join(args[startIdx:], " ")
}

// ParseTimedArgs parses a timed command (e.g., /mute [userID] [duration] [reason]) and handles optional arguments dynamically
func (c *Ctx) ParseTimedArgs(defaultDur time.Duration, defaultReason string) (time.Duration, string) {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return defaultDur, defaultReason
	}

	// Determine starting index of timed arguments (skip target user ID if explicitly provided)
	startIdx := 0
	if c.Message.ReplyToMessage == nil {
		if len(args) > 0 {
			if _, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				startIdx = 1
			}
		}
	}

	if startIdx >= len(args) {
		return defaultDur, defaultReason
	}

	timedArgs := args[startIdx:]

	// Try to parse the first timed word as a duration
	if dur, err := ParseDuration(timedArgs[0]); err == nil && dur > 0 {
		reason := defaultReason
		if len(timedArgs) > 1 {
			reason = strings.Join(timedArgs[1:], " ")
		}
		return dur, reason
	}

	// If the first timed word is not a duration, treat all timed arguments as the reason
	return defaultDur, strings.Join(timedArgs, " ")
}

// ResolveTarget resolves target user ID automatically and replies with a friendly alert on failure
func (c *Ctx) ResolveTarget() (int64, bool) {
	target, err := c.TargetUser()
	if err != nil {
		_, _ = c.ReplyText("⚠️ کاربر مورد نظر مشخص نشده است. لطفاً روی پیام کاربر ریپلای کنید یا آیدی عددی او را وارد کنید.")
		return 0, false
	}
	return target, true
}

// Title sets the main title of the ModLogChain report
func (m *ModLogChain) Title(t string) *ModLogChain { m.title = t; return m }

// TargetID returns the resolved target user ID of the ModLogChain
func (m *ModLogChain) TargetID() int64 { return m.targetID }

// Duration returns the configured duration of the ModLogChain
func (m *ModLogChain) Duration() time.Duration { return m.duration }

// ReasonText returns the configured reason of the ModLogChain
func (m *ModLogChain) ReasonText() string { return m.reason }

// ModLogTemplate initializes and compiles a globally registered ModLog layout cleanly
func (c *Ctx) ModLogTemplate(name string, target int64, duration time.Duration, reason string) *ModLogChain {
	m := c.ModLog("")
	m.Target(target).For(duration).Reason(reason)

	c.Bot.mu.RLock()
	fn, ok := c.Bot.modLogTemplates[name]
	c.Bot.mu.RUnlock()

	if ok && fn != nil {
		fn(m) // Execute the custom layout defined by the developer at startup
	}
	return m
}
