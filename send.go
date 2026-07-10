package gobale

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SendChain provides fluid chain methods to construct and send payloads
type SendChain struct {
	bot        *Bot
	ctx        context.Context
	c          *Ctx
	chat       any
	text       string
	isSettings bool
	photo      any
	audio      any
	doc        any
	video      any
	voice      any
	anim       any
	caption    string
	replyTo    int64
	markup     any
	pm         string
	from       any
	msgID      int64
	action     string
	temp       time.Duration
	lat        float64
	lon        float64
	phone      string
	first      string
	last       string
	isContact  bool
	isLoc      bool
	sticker    any
	dur        int
	width      int
	height     int
	title      string
	stretch    bool
	onProgress func(percent float64)
	useQueue   bool
}

// Buttons dynamically appends a single row of inline buttons (supports string callback pairs, custom builders, URL/Copy/WebApp buttons)
func (s *SendChain) Buttons(buttons ...any) *SendChain {
	if len(buttons) == 0 {
		return s
	}

	var row []InlineKeyboardButton

	for i := 0; i < len(buttons); {
		item := buttons[i]

		// Process raw string callback pairs
		if str, ok := item.(string); ok {
			if i+1 < len(buttons) {
				cbData, okCb := buttons[i+1].(string)
				if okCb {
					// Auto-detect settings callback to activate automated lifecycle natively
					if strings.HasPrefix(cbData, "_sys_cfg:") {
						s.isSettings = true
					}
					row = append(row, NewInlineKeyboardButtonData(str, cbData))
					i += 2
					continue
				}
			}
			// Fallback: treat unmatched odd string as single callback button
			if strings.HasPrefix(str, "_sys_cfg:") {
				s.isSettings = true
			}
			row = append(row, NewInlineKeyboardButtonData(str, str))
			i++
		} else if builder, ok := item.(*InlineButtonBuilder); ok && builder != nil {
			// Process advanced custom buttons (Copy, URL, WebApp) fluidly
			btn := builder.btn
			if strings.HasPrefix(btn.CallbackData, "_sys_cfg:") {
				s.isSettings = true
			}
			row = append(row, btn)
			i++
		} else if btn, ok := item.(InlineKeyboardButton); ok {
			if strings.HasPrefix(btn.CallbackData, "_sys_cfg:") {
				s.isSettings = true
			}
			row = append(row, btn)
			i++
		} else {
			i++ // skip unsupported parameters safely
		}
	}

	if len(row) == 0 {
		return s
	}

	// Fetch or initialize active inline keyboard to append rows dynamically
	var currentMarkup *InlineKeyboardMarkup
	if s.markup != nil {
		if m, ok := s.markup.(*InlineKeyboardMarkup); ok && m != nil {
			currentMarkup = m
		}
	}

	if currentMarkup == nil {
		currentMarkup = &InlineKeyboardMarkup{
			InlineKeyboard: make([][]InlineKeyboardButton, 0),
		}
		s.markup = currentMarkup
	}

	currentMarkup.InlineKeyboard = append(currentMarkup.InlineKeyboard, row)
	return s
}

// ReplyMenu dynamically appends a single row of reply keyboard buttons to the message (supports multi-row appending)
func (s *SendChain) ReplyMenu(buttons ...string) *SendChain {
	if len(buttons) == 0 {
		return s
	}

	var row []KeyboardButton
	for _, text := range buttons {
		row = append(row, NewKeyboardButton(text))
	}

	// Fetch or initialize active reply keyboard to append rows dynamically
	var currentMarkup *ReplyKeyboardMarkup
	if s.markup != nil {
		if m, ok := s.markup.(*ReplyKeyboardMarkup); ok && m != nil {
			currentMarkup = m
		}
	}

	if currentMarkup == nil {
		currentMarkup = &ReplyKeyboardMarkup{
			Keyboard:       make([][]KeyboardButton, 0),
			ResizeKeyboard: true,
		}
		s.markup = currentMarkup
	}

	currentMarkup.Keyboard = append(currentMarkup.Keyboard, row)
	return s
}

// OnProgress registers a callback triggered during upload progress updates (1% to 100%)
func (s *SendChain) OnProgress(fn func(percent float64)) *SendChain {
	s.onProgress = fn
	return s
}

// Queue configures the upload to run asynchronously in a concurrent background worker queue
func (s *SendChain) Queue() *SendChain {
	s.useQueue = true
	return s
}

// Stretch enables or disables message stretching for this specific send action
func (s *SendChain) Stretch(v bool) *SendChain {
	s.stretch = v
	return s
}

// Title sets the display title of the audio or music player fluidly
func (s *SendChain) Title(t string) *SendChain {
	s.title = t
	return s
}

// Width sets the player width of the video or animation in pixels fluidly
func (s *SendChain) Width(w int) *SendChain {
	s.width = w
	return s
}

// Height sets the player height of the video or animation in pixels fluidly
func (s *SendChain) Height(h int) *SendChain {
	s.height = h
	return s
}

// Dur sets the duration of the audio, video, voice, or animation in seconds fluidly
func (s *SendChain) Dur(seconds int) *SendChain {
	s.dur = seconds
	return s
}

// Send registers target destination and returns a sending dot chain
func (b *Bot) Send(chat any) *SendChain {
	return &SendChain{
		bot:  b,
		ctx:  context.Background(),
		chat: chat,
	}
}

// Sticker appends a sticker file path or File ID directly to the send chain
func (s *SendChain) Sticker(stk any) *SendChain {
	s.sticker = stk
	return s
}

// Text attaches text body to the sending pipeline
func (s *SendChain) Text(t string) *SendChain {
	s.text = t
	return s
}

// Photo attaches a photo file path or file ID to the pipeline
func (s *SendChain) Photo(p any) *SendChain {
	s.photo = p
	return s
}

// Audio attaches an audio file path or file ID to the pipeline
func (s *SendChain) Audio(a any) *SendChain {
	s.audio = a
	return s
}

// Doc attaches a document file path or file ID to the pipeline
func (s *SendChain) Doc(d any) *SendChain {
	s.doc = d
	return s
}

// Video attaches a video file path or file ID to the pipeline
func (s *SendChain) Video(v any) *SendChain {
	s.video = v
	return s
}

// Voice attaches a voice file path or file ID to the pipeline
func (s *SendChain) Voice(v any) *SendChain {
	s.voice = v
	return s
}

// Anim attaches an animation file path or file ID to the pipeline
func (s *SendChain) Anim(a any) *SendChain {
	s.anim = a
	return s
}

// Contact appends a contact card (phone and names) directly to the send chain
func (s *SendChain) Contact(phoneNumber, firstName, lastName string) *SendChain {
	s.isContact = true
	s.phone = phoneNumber
	s.first = firstName
	s.last = lastName
	return s
}

// Location appends a geographic coordinate map to the send chain
func (s *SendChain) Location(latitude, longitude float64) *SendChain {
	s.isLoc = true
	s.lat = latitude
	s.lon = longitude
	return s
}

// Caption appends descriptive caption to media objects
func (s *SendChain) Caption(c string) *SendChain {
	s.caption = c
	return s
}

// Reply links the outgoing message as response to a given ID
func (s *SendChain) Reply(id int64) *SendChain {
	s.replyTo = id
	return s
}

// Markup appends a markup keyboard payload to the message
func (s *SendChain) Markup(m any) *SendChain {
	s.markup = m
	return s
}

// Markdown enables Markdown styling rules for message text
func (s *SendChain) Markdown() *SendChain {
	s.pm = "Markdown"
	return s
}

// From registers the source origin chat ID for copying or forwarding
func (s *SendChain) From(chat any) *SendChain {
	s.from = chat
	return s
}

// Forward configures the sending chain to forward an existing message
func (s *SendChain) Forward(fromChat any, messageID int64) *SendChain {
	s.action = "forward"
	s.from = fromChat
	s.msgID = messageID
	return s
}

// Copy configures the sending chain to copy an existing message without links
func (s *SendChain) Copy(fromChat any, messageID int64) *SendChain {
	s.action = "copy"
	s.from = fromChat
	s.msgID = messageID
	return s
}

// MarkupRemove appends a reply keyboard removal markup to the message
func (s *SendChain) MarkupRemove() *SendChain {
	s.markup = &ReplyKeyboardRemove{RemoveKeyboard: true}
	return s
}

// Temp configures the message to automatically delete itself after duration expires
func (s *SendChain) Temp(d time.Duration) *SendChain {
	s.temp = d
	return s
}

// Confirm appends a standard two-option confirmation glass keyboard to the send chain
func (s *SendChain) Confirm(yesCallback, noCallback string) *SendChain {
	s.markup = InlineMarkup().
		Row(
			Btn("✅ بله، مطمئنم").Callback(yesCallback),
			Btn("❌ خیر، لغو شود").Callback(noCallback),
		).
		Build()
	return s
}

// Settings builds the dynamic configuration keyboard natively supporting dynamic back buttons
func (s *SendChain) Settings(chatID ...any) *SendChain {
	// Set internal settings flag to handle automated deletions
	s.isSettings = true
	resolved := s.bot.ResolveChatID(s.chat)
	if len(chatID) > 0 {
		resolved = s.bot.ResolveChatID(chatID[0])
	}
	builder := InlineMarkup()

	s.bot.mu.RLock()
	db := s.bot.dbInstance
	for _, entry := range s.bot.settings {
		status := "🔴 خاموش"
		if entry.IsLocal {
			dbKey := fmt.Sprintf("group_config_%v_%s", resolved, entry.Key)
			val, ok := db.Get(dbKey)
			active := entry.Default
			if ok {
				if bVal, okBool := val.(bool); okBool {
					active = bVal
				}
			}
			if active {
				status = "🟢 روشن"
			}
		} else {
			if entry.Ptr != nil && *entry.Ptr {
				status = "🟢 روشن"
			}
		}

		callbackKey := fmt.Sprintf("_sys_cfg:%s:%v", entry.Key, resolved)
		builder.Row(Btn(entry.Label + ": " + status).Callback(callbackKey))
	}

	if cid, ok := resolved.(int64); ok {
		if sess := s.bot.Sessions.Get(cid); sess != nil {
			if menuID := sess.String("_current_menu"); menuID != "" {
				builder.Row(Btn("🔙 بازگشت").Callback(fmt.Sprintf("_menu:%s", menuID)))
			}
		}
	}
	s.bot.mu.RUnlock()

	s.markup = builder.Build()
	return s
}

// Go executes the sending chain process with full support for media, locations, contacts, and automated settings lifecycle
func (s *SendChain) Go() (*Message, error) {
	if s.chat == nil {
		return nil, errors.New("missing chat destination")
	}

	resolved := s.bot.ResolveChatID(s.chat)
	var chatID int64
	if id, ok := resolved.(int64); ok {
		chatID = id
	}

	// Automated Settings Panel Lifecycle (deletes previous panel, locks admin click, and saves new ID silently)
	if s.isSettings && chatID > 0 {
		sess := s.bot.Sessions.Get(chatID)

		// 1. Delete previously active settings panel concurrently in background using unchained Int64
		oldID := sess.Int64("active_settings_msg_id")
		if oldID > 0 {
			botInstance := s.bot
			go func() {
				defer func() {
					if r := recover(); r != nil {
						handlePanic(botInstance, r, nil)
					}
				}()
				_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": oldID,
				}, nil)
			}()
		}

		// 2. Lock settings panel interaction natively to the current command executor via unchained Set
		if s.c != nil {
			sess.Set("active_settings_admin_id", s.c.SenderID())
		}
	}

	var msg *Message
	var err error

	if s.useQueue {
		initUploadPool()
		resultChan := make(chan *UploadResult, 1)

		job := &UploadJob{
			sendChain:  s,
			resultChan: resultChan,
		}
		globalUploadPool.jobChan <- job
		res := <-resultChan
		msg, err = res.Msg, res.Err
	} else {
		msg, err = s.executeUpload(s.ctx)
	}

	// 3. Save the newly created settings panel's ID to session for subsequent cleanups via unchained Set
	if err == nil && msg != nil && s.isSettings && chatID > 0 {
		sess := s.bot.Sessions.Get(chatID)
		sess.Set("active_settings_msg_id", msg.MessageID)
	}

	// Handle scheduled dynamic self-deletions on temporary messages
	if err == nil && msg != nil && s.temp > 0 {
		msgID := msg.MessageID
		s.bot.Task().In(s.temp, func() {
			_ = s.bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
				"chat_id":    resolved,
				"message_id": msgID,
			}, nil)
		})
	}

	return msg, err
}

// cacheUploadedID stores received file identifier to avoid uploading duplicates
func (s *SendChain) cacheUploadedID(path, field string, msg *Message) {
	var id string
	switch field {
	case "photo":
		if len(msg.Photo) > 0 {
			id = msg.Photo[len(msg.Photo)-1].FileID
		}
	case "sticker":
		// Cache successfully uploaded sticker file ID automatically
		if msg.Sticker != nil {
			id = msg.Sticker.FileID
		}
	case "audio":
		if msg.Audio != nil {
			id = msg.Audio.FileID
		}
	case "document":
		if msg.Document != nil {
			id = msg.Document.FileID
		}
	case "video":
		if msg.Video != nil {
			id = msg.Video.FileID
		}
	case "voice":
		if msg.Voice != nil {
			id = msg.Voice.FileID
		}
	case "animation":
		if msg.Animation != nil {
			id = msg.Animation.FileID
		}
	}
	if id != "" {
		s.bot.fileCache.Store(path, id)
	}
}

// isLocalFile determines if input string references a physical file on system disk
func isLocalFile(path string) bool {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return false
	}
	if len(path) > 100 && !strings.Contains(path, "/") && !strings.Contains(path, "\\") {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// Album initializes an album fluent delivery chain
func (s *SendChain) Album() *AlbumChain {
	return &AlbumChain{chain: s}
}

// AlbumChain manages fluid structures of media groups
type AlbumChain struct {
	chain *SendChain
	media []any
}

// Photo appends a photo file path or ID directly into the album group
func (a *AlbumChain) Photo(path string, caption ...string) *AlbumChain {
	p := InputMediaPhoto{
		Type:  "photo",
		Media: path,
	}
	if len(caption) > 0 {
		p.Caption = caption[0]
	}
	a.media = append(a.media, p)
	return a
}

// Video appends a video file path or ID directly into the album group
func (a *AlbumChain) Video(path string, caption ...string) *AlbumChain {
	v := InputMediaVideo{
		Type:  "video",
		Media: path,
	}
	if len(caption) > 0 {
		v.Caption = caption[0]
	}
	a.media = append(a.media, v)
	return a
}

// Go executes the media group sending process supporting both local files and file IDs
func (a *AlbumChain) Go() ([]Message, error) {
	if len(a.media) == 0 {
		return nil, errors.New("empty album group")
	}
	resolved := a.chain.bot.ResolveChatID(a.chain.chat)

	var filesToUpload []InputFile
	var resolvedMedia []any
	var filesToClose []*os.File

	defer func() {
		for _, f := range filesToClose {
			_ = f.Close()
		}
	}()

	for idx, item := range a.media {
		switch m := item.(type) {
		case InputMediaPhoto:
			if isLocalFile(m.Media) {
				field := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("failed to open photo file %s: %w", m.Media, err) // Return open error immediately
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, InputFile{
					Field:    field,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + field
			}
			resolvedMedia = append(resolvedMedia, m)
		case InputMediaVideo:
			if isLocalFile(m.Media) {
				field := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("failed to open video file %s: %w", m.Media, err) // Return open error immediately
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, InputFile{
					Field:    field,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + field
			}
			resolvedMedia = append(resolvedMedia, m)
		default:
			resolvedMedia = append(resolvedMedia, item)
		}
	}

	payload := map[string]any{
		"chat_id":             resolved,
		"media":               resolvedMedia,
		"reply_to_message_id": a.chain.replyTo,
	}

	var msgs []Message
	var err error

	if len(filesToUpload) > 0 {
		err = a.chain.bot.BaseRequestMultipart(a.chain.ctx, "sendMediaGroup", payload, filesToUpload, &msgs)
	} else {
		err = a.chain.bot.BaseRequest(a.chain.ctx, "sendMediaGroup", payload, &msgs)
	}

	return msgs, err
}

// ProgressChain handles fluent progress monitoring message edits in background
type ProgressChain struct {
	sc    *SendChain
	title string
	steps []string
	delay time.Duration
}

// Progress initiates the fluent step-by-step progress sender
func (s *SendChain) Progress(title string, steps []string) *ProgressChain {
	return &ProgressChain{
		sc:    s,
		title: title,
		steps: steps,
		delay: 1 * time.Second,
	}
}

// Delay registers custom transition sleep duration between steps
func (p *ProgressChain) Delay(d time.Duration) *ProgressChain {
	p.delay = d
	return p
}

// Go executes initial progress message and starts background edits
func (p *ProgressChain) Go() (*Message, error) {
	if len(p.steps) == 0 {
		return nil, errors.New("empty progress steps")
	}
	resolved := p.sc.bot.ResolveChatID(p.sc.chat)
	chatID, ok := resolved.(int64)
	if !ok {
		return nil, errors.New("progress requires a numeric chat ID")
	}
	initialText := fmt.Sprintf("%s\n\n⏳ %s", p.title, p.steps[0])
	var msg Message
	err := p.sc.bot.BaseRequest(p.sc.ctx, "sendMessage", map[string]any{
		"chat_id":    chatID,
		"text":       initialText,
		"parse_mode": p.sc.pm,
	}, &msg)
	if err != nil {
		return nil, err
	}

	// Safely copy message ID before passing to the concurrent progress edits goroutine
	msgID := msg.MessageID
	p.sc.bot.Task().In(p.delay, func() {
		for i := 1; i < len(p.steps); i++ {
			time.Sleep(p.delay)
			var sb strings.Builder
			sb.WriteString(p.title)
			sb.WriteString("\n\n")
			for j := 0; j < i; j++ {
				sb.WriteString(fmt.Sprintf("✅ %s\n", p.steps[j]))
			}
			sb.WriteString(fmt.Sprintf("⏳ %s", p.steps[i]))
			errEdit := p.sc.bot.BaseRequest(context.Background(), "editMessageText", map[string]any{
				"chat_id":    chatID,
				"message_id": msgID,
				"text":       sb.String(),
			}, nil)
			if errEdit != nil {
				// Break execution if message was deleted or chat blocked
				return
			}
		}
		time.Sleep(p.delay)
		var sb strings.Builder
		sb.WriteString(p.title)
		sb.WriteString("\n\n")
		for _, step := range p.steps {
			sb.WriteString(fmt.Sprintf("✅ %s\n", step))
		}
		sb.WriteString("\n🎉 فرآیند با موفقیت به پایان رسید!")
		_ = p.sc.bot.BaseRequest(context.Background(), "editMessageText", map[string]any{
			"chat_id":    chatID,
			"message_id": msgID,
			"text":       sb.String(),
		}, nil)
	})

	return &msg, nil
}

// Context registers a custom parent context to control deadlines or cancellation propagation
func (s *SendChain) Context(ctx context.Context) *SendChain {
	if ctx != nil {
		s.ctx = ctx
	}
	return s
}

// stretchText ensures every single line is stretched using standard and non-breaking spaces to prevent trailing collapse
func stretchText(text string) string {
	lines := strings.Split(text, "\n")
	targetMinLen := 45

	var sb strings.Builder
	for idx, line := range lines {
		runes := []rune(line)
		l := len(runes)

		sb.WriteString(line)
		if l < targetMinLen {
			diff := targetMinLen - l
			// Pad with standard spaces
			for i := 0; i < diff-1; i++ {
				sb.WriteString(" ")
			}
			// Append non-breaking space at the end to prevent client-side trailing space collapse
			sb.WriteString("\u00A0")
		}
		if idx < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// EditChain manages fluent message edits dynamically
type EditChain struct {
	c       *Ctx
	text    string
	caption string
	markup  any
	pm      string
	stretch bool
}

// Stretch enables or disables message stretching for this specific edit action
func (e *EditChain) Stretch(v bool) *EditChain {
	e.stretch = v
	return e
}

// Edit opens fluent editing dot system inside context
func (c *Ctx) Edit() *EditChain {
	return &EditChain{c: c}
}

// Text registers new text body for editing
func (e *EditChain) Text(t string) *EditChain {
	e.text = t
	return e
}

// Caption registers new caption body for editing
func (e *EditChain) Caption(c string) *EditChain {
	e.caption = c
	return e
}

// Markup appends updated markup keyboard
func (e *EditChain) Markup(m any) *EditChain {
	e.markup = m
	return e
}

// Markdown enables Markdown styling rules for edited text
func (e *EditChain) Markdown() *EditChain {
	e.pm = "Markdown"
	return e
}

// Settings builds the dynamic configuration keyboard natively supporting dynamic back buttons inside Edit
func (e *EditChain) Settings(chatID ...any) *EditChain {
	id, err := e.c.ChatID()
	if err != nil && len(chatID) == 0 {
		return e
	}

	var resolved any
	if len(chatID) > 0 {
		resolved = e.c.Bot.ResolveChatID(chatID[0])
	} else {
		resolved = e.c.Bot.ResolveChatID(id)
	}

	builder := InlineMarkup()
	e.c.Bot.mu.RLock()
	db := e.c.Bot.dbInstance
	for _, entry := range e.c.Bot.settings {
		status := "🔴 خاموش"
		if entry.IsLocal {
			dbKey := fmt.Sprintf("group_config_%v_%s", resolved, entry.Key)
			val, ok := db.Get(dbKey)
			active := entry.Default
			if ok {
				if bVal, okBool := val.(bool); okBool {
					active = bVal
				}
			}
			if active {
				status = "🟢 روشن"
			}
		} else {
			if entry.Ptr != nil && *entry.Ptr {
				status = "🟢 روشن"
			}
		}

		callbackKey := fmt.Sprintf("_sys_cfg:%s:%v", entry.Key, resolved)
		builder.Row(Btn(entry.Label + ": " + status).Callback(callbackKey))
	}

	// NEW: Automatically append Back button to Settings panel inside Edit
	if sess := e.c.Session(); sess != nil {
		if menuID := sess.String("_current_menu"); menuID != "" {
			builder.Row(Btn("🔙 بازگشت").Callback(fmt.Sprintf("_menu:%s", menuID)))
		}
	}
	e.c.Bot.mu.RUnlock()

	e.markup = builder.Build()
	return e
}

// Go executes the edit operation on Bale servers safely with output logs
func (e *EditChain) Go() (*Message, error) {
	if e.c.Message == nil {
		return nil, errors.New("no message in context to edit")
	}
	id, err := e.c.ChatID()
	if err != nil {
		return nil, err
	}

	var method string
	payload := map[string]any{
		"chat_id":    id,
		"message_id": e.c.Message.MessageID,
	}

	// Dynamically build the payload and select API method to eliminate duplicate codeblocks
	switch {
	case e.text != "":
		method = "editMessageText"
		text := e.text
		if e.c.Bot.AutoStretch || e.stretch {
			text = stretchText(text)
		}
		payload["text"] = text
		payload["parse_mode"] = e.pm
		payload["reply_markup"] = e.markup
	case e.caption != "":
		method = "editMessageCaption"
		payload["caption"] = e.caption
		payload["reply_markup"] = e.markup
	case e.markup != nil:
		method = "editMessageReplyMarkup"
		payload["reply_markup"] = e.markup
	default:
		return nil, errors.New("empty edit parameters")
	}

	var msg Message
	err = e.c.Bot.BaseRequest(e.c.ctx, method, payload, &msg)

	if err != nil {
		logErr(e.c.Bot, "[Edit Error] ", err)
	} else {
		// Log outgoing edit requests structurally
		e.c.Bot.Log().Info("ویرایش پکت شبکه (Outgoing Edit)").
			Any("chat_id", id).
			Any("payload", &msg).
			Go()
	}

	return &msg, err
}

// BroadcastResult holds the outcome of a single message delivery attempt
type BroadcastResult struct {
	UserID  int64
	Success bool
	Err     error
}

// BroadcastChain manages bulk message delivery with concurrency control
type BroadcastChain struct {
	bot      *Bot
	ctx      context.Context
	users    []int64
	text     string
	photo    any
	doc      any
	markup   any
	pm       string
	workers  int
	delay    time.Duration
	onResult func(BroadcastResult)
}

// Broadcast opens the fluent bulk delivery dot system from Bot context
func (b *Bot) Broadcast(userIDs []int64) *BroadcastChain {
	return &BroadcastChain{
		bot:     b,
		ctx:     context.Background(),
		users:   userIDs,
		workers: 5,
		delay:   50 * time.Millisecond,
	}
}

// Broadcast opens the fluent bulk delivery dot system from Handler context
func (c *Ctx) Broadcast(userIDs []int64) *BroadcastChain {
	return &BroadcastChain{
		bot:     c.Bot,
		ctx:     c.ctx,
		users:   userIDs,
		workers: 5,
		delay:   50 * time.Millisecond,
	}
}

// Text sets the message text payload
func (bc *BroadcastChain) Text(t string) *BroadcastChain {
	bc.text = t
	return bc
}

// Photo sets a photo file ID or path payload
func (bc *BroadcastChain) Photo(p any) *BroadcastChain {
	bc.photo = p
	return bc
}

// Doc sets a document file ID or path payload
func (bc *BroadcastChain) Doc(d any) *BroadcastChain {
	bc.doc = d
	return bc
}

// Markup appends a keyboard to each broadcast message
func (bc *BroadcastChain) Markup(m any) *BroadcastChain {
	bc.markup = m
	return bc
}

// Markdown enables Markdown parse mode for the broadcast
func (bc *BroadcastChain) Markdown() *BroadcastChain {
	bc.pm = "Markdown"
	return bc
}

// Workers sets how many concurrent senders run in parallel (default: 5)
func (bc *BroadcastChain) Workers(n int) *BroadcastChain {
	if n > 0 {
		bc.workers = n
	}
	return bc
}

// Delay sets the pause between each send to avoid flood limits (default: 50ms)
func (bc *BroadcastChain) Delay(d time.Duration) *BroadcastChain {
	bc.delay = d
	return bc
}

// OnResult registers a callback called after each individual delivery attempt
func (bc *BroadcastChain) OnResult(fn func(BroadcastResult)) *BroadcastChain {
	bc.onResult = fn
	return bc
}

// Go executes the broadcast and returns total sent count and error count
func (bc *BroadcastChain) Go() (sent int, failed int) {
	if len(bc.users) == 0 {
		return
	}

	jobs := make(chan int64, len(bc.users))
	for _, uid := range bc.users {
		jobs <- uid
	}
	close(jobs)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < bc.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for userID := range jobs {
				var err error

				s := bc.bot.Send(userID)
				if bc.markup != nil {
					s = s.Markup(bc.markup)
				}
				if bc.pm != "" {
					s = s.Markdown()
				}

				if bc.photo != nil {
					_, err = s.Photo(bc.photo).Go()
				} else if bc.doc != nil {
					_, err = s.Doc(bc.doc).Go()
				} else if bc.text != "" {
					_, err = s.Text(bc.text).Go()
				}

				result := BroadcastResult{
					UserID:  userID,
					Success: err == nil,
					Err:     err,
				}

				mu.Lock()
				if err == nil {
					sent++
				} else {
					failed++
				}
				mu.Unlock()

				if bc.onResult != nil {
					bc.onResult(result)
				}

				if bc.delay > 0 {
					time.Sleep(bc.delay)
				}
			}
		}()
	}

	wg.Wait()
	return
}

// GroupSettings builds a chat-isolated, dynamic inline settings keyboard
func (s *SendChain) GroupSettings() *SendChain {
	resolved := s.bot.ResolveChatID(s.chat)
	var chatID int64
	switch v := resolved.(type) {
	case int64:
		chatID = v
	case int:
		chatID = int64(v)
	case int32:
		chatID = int64(v)
	}

	builder := InlineMarkup()
	s.bot.mu.RLock()
	db := s.bot.dbInstance
	for _, setting := range s.bot.groupSettings {
		dbKey := fmt.Sprintf("group_config_%d_%s", chatID, setting.Key)

		status := setting.Default
		if val, ok := db.Get(dbKey); ok {
			if bVal, okBool := val.(bool); okBool {
				status = bVal
			}
		}

		emoji := "🔴 خاموش"
		if status {
			emoji = "🟢 روشن"
		}

		callbackKey := "_sys_gcfg:" + setting.Key
		builder.Row(Btn(setting.Label + ": " + emoji).Callback(callbackKey))
	}
	s.bot.mu.RUnlock()
	s.markup = builder.Build()
	return s
}

// GroupSettings builds a chat-isolated, dynamic inline settings keyboard inside Edit
func (e *EditChain) GroupSettings() *EditChain {
	id, err := e.c.ChatID()
	if err != nil {
		return e
	}

	builder := InlineMarkup()
	e.c.Bot.mu.RLock()
	db := e.c.Bot.dbInstance
	for _, setting := range e.c.Bot.groupSettings {
		dbKey := fmt.Sprintf("group_config_%d_%s", id, setting.Key)

		status := setting.Default
		if val, ok := db.Get(dbKey); ok {
			if bVal, okBool := val.(bool); okBool {
				status = bVal
			}
		}

		emoji := "🔴 خاموش"
		if status {
			emoji = "🟢 روشن"
		}

		callbackKey := "_sys_gcfg:" + setting.Key
		builder.Row(Btn(setting.Label + ": " + emoji).Callback(callbackKey))
	}
	e.c.Bot.mu.RUnlock()
	e.markup = builder.Build()
	return e
}

// BotEditChain manages fluent message edits from a Bot context
type BotEditChain struct {
	bot       *Bot
	ctx       context.Context
	chat      any
	messageID int64
	text      string
	caption   string
	markup    any
	pm        string
}

// Edit opens the fluent editing dot system from Bot context using target chat and message ID
func (b *Bot) Edit(chat any, messageID int64) *BotEditChain {
	return &BotEditChain{
		bot:       b,
		ctx:       context.Background(),
		chat:      chat,
		messageID: messageID,
	}
}

// Text registers new text body for editing from Bot context
func (e *BotEditChain) Text(t string) *BotEditChain {
	e.text = t
	return e
}

// Caption registers new caption body for editing from Bot context
func (e *BotEditChain) Caption(c string) *BotEditChain {
	e.caption = c
	return e
}

// Markup appends updated markup keyboard from Bot context
func (e *BotEditChain) Markup(m any) *BotEditChain {
	e.markup = m
	return e
}

// Markdown enables Markdown styling rules for edited text from Bot context
func (e *BotEditChain) Markdown() *BotEditChain {
	e.pm = "Markdown"
	return e
}

// Go executes the edit operation on Bale servers from Bot context
func (e *BotEditChain) Go() (*Message, error) {
	resolved := e.bot.ResolveChatID(e.chat)
	var method string
	payload := map[string]any{
		"chat_id":    resolved,
		"message_id": e.messageID,
	}

	switch {
	case e.text != "":
		method = "editMessageText"
		payload["text"] = e.text
		if e.pm != "" {
			payload["parse_mode"] = e.pm
		}
		if e.markup != nil {
			payload["reply_markup"] = e.markup
		}
	case e.caption != "":
		method = "editMessageCaption"
		payload["caption"] = e.caption
		if e.markup != nil {
			payload["reply_markup"] = e.markup
		}
	case e.markup != nil:
		method = "editMessageReplyMarkup"
		payload["reply_markup"] = e.markup
	default:
		return nil, errors.New("empty edit parameters")
	}

	var msg Message
	err := e.bot.BaseRequest(e.ctx, method, payload, &msg)
	if err != nil {
		logErr(e.bot, "[Bot Edit Error] ", err)
	}
	return &msg, err
}

// executeUpload processes the final message transmission with progress context and output logs
func (s *SendChain) executeUpload(ctx context.Context) (*Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resolved := s.bot.ResolveChatID(s.chat)
	var msg Message
	var err error

	if s.sticker != nil {
		err = s.uploadMediaWithProgress(ctx, "sendSticker", "sticker", s.sticker, s.onProgress, &msg)
	} else if s.photo != nil {
		err = s.uploadMediaWithProgress(ctx, "sendPhoto", "photo", s.photo, s.onProgress, &msg)
	} else if s.audio != nil {
		err = s.uploadMediaWithProgress(ctx, "sendAudio", "audio", s.audio, s.onProgress, &msg)
	} else if s.doc != nil {
		err = s.uploadMediaWithProgress(ctx, "sendDocument", "document", s.doc, s.onProgress, &msg)
	} else if s.video != nil {
		err = s.uploadMediaWithProgress(ctx, "sendVideo", "video", s.video, s.onProgress, &msg)
	} else if s.voice != nil {
		err = s.uploadMediaWithProgress(ctx, "sendVoice", "voice", s.voice, s.onProgress, &msg)
	} else if s.anim != nil {
		err = s.uploadMediaWithProgress(ctx, "sendAnimation", "animation", s.anim, s.onProgress, &msg)
	} else if s.text != "" {
		text := s.text
		if s.bot.AutoStretch || s.stretch {
			text = stretchText(text)
		}
		payload := map[string]any{
			"chat_id": resolved,
			"text":    text,
		}
		if s.pm != "" {
			payload["parse_mode"] = s.pm
		}
		if s.replyTo > 0 {
			payload["reply_to_message_id"] = s.replyTo
		}
		if s.markup != nil {
			payload["reply_markup"] = s.markup
		}
		err = s.bot.BaseRequest(ctx, "sendMessage", payload, &msg)
	} else {
		return nil, errors.New("empty payload parameters")
	}

	if err == nil {
		// Log outgoing send requests structurally
		s.bot.Log().Info("ارسال پکت شبکه (Outgoing Send)").
			Any("chat_id", resolved).
			Any("payload", &msg).
			Go()
	}

	return &msg, err
}

// uploadMediaWithProgress manages file uploads with dynamic progress context and API cache support safely
func (s *SendChain) uploadMediaWithProgress(ctx context.Context, method, field string, media any, onProgress func(pct float64), out *Message) error {
	resolved := s.bot.ResolveChatID(s.chat)
	payload := map[string]any{
		"chat_id": resolved,
	}
	if s.caption != "" {
		payload["caption"] = s.caption
	}
	if s.replyTo > 0 {
		payload["reply_to_message_id"] = s.replyTo
	}
	if s.markup != nil {
		payload["reply_markup"] = s.markup
	}
	if s.dur > 0 {
		payload["duration"] = s.dur
	}
	if s.width > 0 {
		payload["width"] = s.width
	}
	if s.height > 0 {
		payload["height"] = s.height
	}
	if s.title != "" {
		payload["title"] = s.title
	}

	switch m := media.(type) {
	case string:
		if isLocalFile(m) {
			if cached, ok := s.bot.fileCache.Load(m); ok {
				payload[field] = cached
				return s.bot.BaseRequest(ctx, method, payload, out)
			}
			file, err := os.Open(m)
			if err != nil {
				return err
			}
			defer file.Close()
			inputFile := InputFile{
				FileName: filepath.Base(m),
				Reader:   file,
				Field:    field,
			}
			// Call the new BaseRequestMultipartWithProgress to enable true network progress and retry capabilities
			err = s.bot.Client.BaseRequestMultipartWithProgress(ctx, method, payload, []InputFile{inputFile}, onProgress, out)
			if err == nil {
				s.cacheUploadedID(m, field, out)
			}
			return err
		}
		payload[field] = m
		return s.bot.BaseRequest(ctx, method, payload, out)
	case InputFile:
		m.Field = field
		// Call the new BaseRequestMultipartWithProgress to enable true network progress and retry capabilities
		return s.bot.Client.BaseRequestMultipartWithProgress(ctx, method, payload, []InputFile{m}, onProgress, out)
	}
	return errors.New("invalid media configuration type")
}

// IDs returns a slice of all message IDs generated by the album upload
func (a *AlbumChain) IDs(msgs []Message) []int64 {
	var ids []int64
	for _, m := range msgs {
		ids = append(ids, m.MessageID)
	}
	return ids
}

// EXPERIMENTAL (Not Implemented)

// SendPollChain manages fluent poll creation and delivery
type SendPollChain struct {
	sc       *SendChain
	question string
	options  []string
	isAnon   bool
	multi    bool
}

// Poll initiates a poll sending process from the SendChain context
func (s *SendChain) Poll(question string, options ...string) *SendPollChain {
	return &SendPollChain{
		sc:       s,
		question: question,
		options:  options,
		isAnon:   false,
	}
}

// Anonymous configures whether the poll results are anonymous
func (p *SendPollChain) Anonymous(v bool) *SendPollChain {
	p.isAnon = v
	return p
}

// MultiAnswer allows users to select multiple options in the poll
func (p *SendPollChain) MultiAnswer(v bool) *SendPollChain {
	p.multi = v
	return p
}

// Go executes the poll sending request to Bale servers
func (p *SendPollChain) Go() (*Message, error) {
	resolved := p.sc.bot.ResolveChatID(p.sc.chat)
	payload := map[string]any{
		"chat_id":                 resolved,
		"question":                p.question,
		"options":                 p.options,
		"is_anonymous":            p.isAnon,
		"allows_multiple_answers": p.multi,
		"reply_to_message_id":     p.sc.replyTo,
	}

	var msg Message
	err := p.sc.bot.BaseRequest(p.sc.ctx, "sendPoll", payload, &msg)
	return &msg, err
}
