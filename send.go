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
	bot       *Bot
	ctx       context.Context
	chat      any
	text      string
	photo     any
	audio     any
	doc       any
	video     any
	voice     any
	anim      any
	caption   string
	replyTo   int64
	markup    any
	pm        string
	from      any
	msgID     int64
	action    string
	temp      time.Duration
	lat       float64
	lon       float64
	phone     string
	first     string
	last      string
	isContact bool
	isLoc     bool
	sticker   any
	dur       int
	width     int
	height    int
	title     string
	stretch   bool
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

// Paginated sets up automatic button pagination for the send chain
func (s *SendChain) Paginated(items []InlineKeyboardButton, page, perPage int, prefix string) *SendChain {
	s.markup = NewPaginatedKeyboard(items, page, perPage, prefix)
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

// Settings builds the dynamic system configuration keyboard automatically supporting local states
func (s *SendChain) Settings() *SendChain {
	resolved := s.bot.ResolveChatID(s.chat)
	builder := InlineMarkup()
	
	s.bot.mu.RLock()
	db := s.bot.dbInstance
	for _, entry := range s.bot.settings {
		status := "🔴 خاموش"
		if entry.IsLocal {
			// Read group-isolated config dynamically in-place
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
			// Read global pointer configuration state
			if entry.Ptr != nil && *entry.Ptr {
				status = "🟢 روشن"
			}
		}
		
		callbackKey := "_sys_cfg:" + entry.Key
		builder.Row(Btn(entry.Label + ": " + status).Callback(callbackKey))
	}
	s.bot.mu.RUnlock()
	
	s.markup = builder.Build()
	return s
}

// Go executes the sending chain process with full support for media, locations, contacts, and auto-delete
func (s *SendChain) Go() (*Message, error) {
	if s.chat == nil {
		return nil, errors.New("missing chat destination")
	}
	resolved := s.bot.ResolveChatID(s.chat)
	var msg Message
	var err error

	if s.markup != nil {
		if _, isInline := s.markup.(*InlineKeyboardMarkup); isInline {
			if chatID, ok := resolved.(int64); ok {
				sess := s.bot.Sessions.Get(chatID)
				sess.mu.RLock()
				lastMenuIDVal, exists := sess.DataMap["last_menu_id"]
				sess.mu.RUnlock()
				if exists {
					if lastMenuID, ok := lastMenuIDVal.(int64); ok && lastMenuID > 0 {
						go func(cid, mid int64) {
							_ = s.bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
								"chat_id":    cid,
								"message_id": mid,
							}, nil)
						}(chatID, lastMenuID)
					}
				}
			}
		}
	}

	if s.action == "forward" {
		err = s.bot.BaseRequest(s.ctx, "forwardMessage", map[string]any{
			"chat_id":      resolved,
			"from_chat_id": s.bot.ResolveChatID(s.from),
			"message_id":   s.msgID,
		}, &msg)
	} else if s.action == "copy" {
		var msgIDHolder MessageId
		err = s.bot.BaseRequest(s.ctx, "copyMessage", map[string]any{
			"chat_id":      resolved,
			"from_chat_id": s.bot.ResolveChatID(s.from),
			"message_id":   s.msgID,
		}, &msgIDHolder)
		if err == nil {
			msg.MessageID = msgIDHolder.MessageID
		}
	} else if s.sticker != nil {
		// Call the native sendSticker API on Bale servers
		err = s.uploadMedia("sendSticker", "sticker", s.sticker, &msg)
	} else if s.isContact {
		err = s.bot.BaseRequest(s.ctx, "sendContact", map[string]any{
			"chat_id":             resolved,
			"phone_number":        s.phone,
			"first_name":          s.first,
			"last_name":           s.last,
			"reply_to_message_id": s.replyTo,
			"reply_markup":        s.markup,
		}, &msg)
	} else if s.isLoc {
		err = s.bot.BaseRequest(s.ctx, "sendLocation", map[string]any{
			"chat_id":             resolved,
			"latitude":            s.lat,
			"longitude":           s.lon,
			"reply_to_message_id": s.replyTo,
			"reply_markup":        s.markup,
		}, &msg)
	} else if s.photo != nil {
		err = s.uploadMedia("sendPhoto", "photo", s.photo, &msg)
	} else if s.audio != nil {
		err = s.uploadMedia("sendAudio", "audio", s.audio, &msg)
	} else if s.doc != nil {
		err = s.uploadMedia("sendDocument", "document", s.doc, &msg)
	} else if s.video != nil {
		err = s.uploadMedia("sendVideo", "video", s.video, &msg)
	} else if s.voice != nil {
		err = s.uploadMedia("sendVoice", "voice", s.voice, &msg)
	} else if s.anim != nil {
		err = s.uploadMedia("sendAnimation", "animation", s.anim, &msg)
	} else if s.text != "" {
		text := s.text
		if s.bot.AutoStretch || s.stretch {
			text = stretchText(text)
		}
		err = s.bot.BaseRequest(s.ctx, "sendMessage", map[string]any{
			"chat_id":             resolved,
			"text":                text,
			"parse_mode":          s.pm,
			"reply_to_message_id": s.replyTo,
			"reply_markup":        s.markup,
		}, &msg)
	} else {
		return nil, errors.New("empty payload parameters")
	}

	if err == nil && s.markup != nil {
		if _, isInline := s.markup.(*InlineKeyboardMarkup); isInline {
			if chatID, ok := resolved.(int64); ok {
				sess := s.bot.Sessions.Get(chatID)
				sess.mu.Lock()
				if sess.DataMap == nil {
					sess.DataMap = make(map[string]any)
				}
				sess.DataMap["last_menu_id"] = msg.MessageID
				sess.mu.Unlock()
			}
		}
	}

	if err == nil && s.temp > 0 {
		msgID := msg.MessageID
		s.bot.Task().In(s.temp, func() {
			errDel := s.bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
				"chat_id":    resolved,
				"message_id": msgID,
			}, nil)
			if errDel != nil && s.bot.loggerInstance != nil {
				s.bot.loggerInstance.Log(LevelError, "[Temp Delete Error] ", "failed to delete message %d: %v", []any{msgID, errDel})
			}
		})
	}

	if err != nil {
		logErr(s.bot, "[Send Error] ", err)
	}

	return &msg, err
}

// uploadMedia manages files dispatching via cache lookup or multipart encoding
func (s *SendChain) uploadMedia(method, field string, media any, out *Message) error {
	resolved := s.bot.ResolveChatID(s.chat)
	payload := map[string]any{
		"chat_id":             resolved,
		"caption":             s.caption,
		"reply_to_message_id": s.replyTo,
		"reply_markup":        s.markup,
		"duration":            s.dur,
	}

	// Only attach optional values if they are explicitly configured greater than zero/non-empty
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
				return s.bot.BaseRequest(s.ctx, method, payload, out)
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
			err = s.bot.BaseRequestMultipart(s.ctx, method, payload, []InputFile{inputFile}, out)
			if err == nil {
				s.cacheUploadedID(m, field, out)
			}
			return err
		}
		payload[field] = m
		return s.bot.BaseRequest(s.ctx, method, payload, out)
	case InputFile:
		m.Field = field
		return s.bot.BaseRequestMultipart(s.ctx, method, payload, []InputFile{m}, out)
	}
	return errors.New("invalid media configuration type")
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
				return // Break execution if message was deleted or chat blocked
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

// stretchText ensures the message bubble is wide enough for inline keyboards without adding empty vertical lines
func stretchText(text string) string {
	lines := strings.Split(text, "\n")
	maxLen := 0
	for _, line := range lines {
		l := len([]rune(line))
		if l > maxLen {
			maxLen = l
		}
	}

	// Set target stretch limit suitable for standard mobile screen widths
	targetMinLen := 35
	if maxLen < targetMinLen {
		diff := targetMinLen - maxLen

		var sb strings.Builder
		sb.WriteString(text)

		// Add a standard space first as a safe word break
		sb.WriteString(" ")

		// Fill only the exact difference dynamically with invisible braille spaces
		for i := 0; i < diff-1; i++ {
			sb.WriteString("\u2800")
		}
		return sb.String()
	}
	return text
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

// Paginated sets up automatic button pagination for edited markup
func (e *EditChain) Paginated(items []InlineKeyboardButton, page, perPage int, prefix string) *EditChain {
	e.markup = NewPaginatedKeyboard(items, page, perPage, prefix)
	return e
}

// Settings builds the dynamic system configuration keyboard inside Edit supporting local states
func (e *EditChain) Settings() *EditChain {
	id, err := e.c.ChatID()
	if err != nil {
		return e
	}

	builder := InlineMarkup()
	e.c.Bot.mu.RLock()
	db := e.c.Bot.dbInstance
	for _, entry := range e.c.Bot.settings {
		status := "🔴 خاموش"
		if entry.IsLocal {
			// Read group-isolated config dynamically in-place
			dbKey := fmt.Sprintf("group_config_%v_%s", id, entry.Key)
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
			// Read global pointer configuration in-place
			if entry.Ptr != nil && *entry.Ptr {
				status = "🟢 روشن"
			}
		}
		
		callbackKey := "_sys_cfg:" + entry.Key
		builder.Row(Btn(entry.Label + ": " + status).Callback(callbackKey))
	}
	e.c.Bot.mu.RUnlock()
	
	e.markup = builder.Build()
	return e
}

// Go executes the edit operation on Bale servers safely with a single unified BaseRequest call
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
