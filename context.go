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

// ChatID extracts and returns the current chat identifier safely
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
	if c.Update.CallbackQuery != nil && c.Update.CallbackQuery.Message != nil {
		return c.Update.CallbackQuery.Message.Chat.ID, nil
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
	return a.c.Bot.BaseRequest(a.c.ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": a.c.Update.CallbackQuery.ID,
		"text":              a.text,
		"show_alert":        a.show,
	}, nil)
}

// File initializes file management and actions chain using ID and captures safe chat IDs
func (c *Ctx) File(fileID string) *FileChain {
	origName := ""
	chatID, _ := c.ChatID() // Capture the Chat ID synchronously before context recycling
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
		fc:     f,
		name:   f.origName,
		chatID: f.chatID, // Transfer the captured Chat ID
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
		return destPath, err
	}

	// Standard resilient download without queue (updated: removed chatIDStr parameter)
	err := resilientDownload(ctx, d.fc.bot.Client.httpClient, url, destPath, fileSize, d.onProgress)
	return destPath, err
}

// Send opens the fluent sending dot system inside the handler context
func (c *Ctx) Send() *SendChain {
	id, _ := c.ChatID()
	return &SendChain{
		bot:  c.Bot,
		ctx:  c.ctx,
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

// IsSuperGroup checks if the current chat is a supergroup
func (c *Ctx) IsSuperGroup() bool {
	return c.Message != nil && c.Message.Chat.Type == "supergroup"
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
		limit = len(targets)
	}

	_ = ScanValues(args[:limit], " ", targets[:limit]...)
	return limit
}
