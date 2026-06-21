package gobale

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/PHX-Go/GoBale/db"
	"github.com/PHX-Go/GoBale/logger"
	"github.com/PHX-Go/GoBale/methods"
	"github.com/PHX-Go/GoBale/middleware"
	"github.com/PHX-Go/GoBale/models"
	"github.com/PHX-Go/GoBale/session"
	"github.com/PHX-Go/GoBale/utils"
)

type Request interface {
	Method() string
	Params() any
}

type SettingEntry struct {
	Key   string
	Label string
	Ptr   *bool
}

type wrappedError struct {
	err error
}

type MemoryStats struct {
	AllocMegabytes     float64
	SysMegabytes       float64
	HeapAllocMegabytes float64
	NumGC              uint32
	MemoryLimitBytes   int64
}

type RouterGroup struct {
	bot         *Bot
	middlewares []HandlerFunc
}

type userLimit struct {
	mu          sync.Mutex
	windowStart int64
	msgCount    int
}

type commandInfo struct {
	Command     string
	Description string
}

type Metrics struct {
	TotalUpdates      uint64
	_                 [56]byte
	ProcessedMessages uint64
	_                 [56]byte
	FailedRequests    uint64
	_                 [56]byte
	NetworkLatencyNs  int64
	_                 [56]byte
}

type Bot struct {
	*Client
	Shield              *SystemShield
	muRoutes            sync.RWMutex
	muSettings          sync.RWMutex
	AutoPageLimit       int
	handlers            map[string][]HandlerFunc
	anyMessage          []HandlerFunc
	textRoutes          map[string][]HandlerFunc
	stateRoutes         map[string][]HandlerFunc
	commands            []commandInfo
	preCheckoutHandlers []HandlerFunc
	callbackHandlers    []HandlerFunc
	callbackDataRoutes  map[string][]HandlerFunc
	middlewares         []HandlerFunc
	Sessions            session.SessionStore
	OnError             func(err error, c *Context)
	ctxPool             sync.Pool
	workerChan          chan *models.Update
	numWorkers          int
	errPtr              atomic.Pointer[wrappedError]
	metrics             Metrics
	userLimits          sync.Map
	AntiSpamWindow      time.Duration
	AntiSpamLimit       int
	OnSpam              func(c *Context)
	inviteCache         sync.Map
	Blacklist           map[int64]bool
	Maintenance         bool
	MaintenanceAdminID  int64
	MaintenanceText     string
	i18n                map[string]map[string]string
	workersWg           sync.WaitGroup
	bgSemaphore         chan struct{}
	shieldMode          uint32
	settings            []SettingEntry
	Log                 *logger.Logger
}

func NewBot(token string, numWorkers int) *Bot {
	if token == "" {
		log.Fatal("❌ [Bale Error] Bot Token is empty! Please verify your .env file path and make sure BALE_TOKEN is loaded successfully.")
	}

	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU() * 10
		if numWorkers < 32 {
			numWorkers = 32
		}
	}

	store := session.NewGOBStore("gobale_sessions.db")
	_ = store.Load()

	var bot *Bot

	bot = &Bot{
		Client:             NewClient(token),
		AutoPageLimit:      8,
		Log:                logger.NewLogger(logger.LevelInfo, "bot.log", true),
		handlers:           make(map[string][]HandlerFunc),
		textRoutes:         make(map[string][]HandlerFunc),
		callbackDataRoutes: make(map[string][]HandlerFunc),
		stateRoutes:        make(map[string][]HandlerFunc),
		Sessions:           store,
		workerChan:         make(chan *models.Update, 1000),
		numWorkers:         numWorkers,
		AntiSpamLimit:      3,
		AntiSpamWindow:     5 * time.Second,
		Blacklist:          make(map[int64]bool),
		Maintenance:        false,
		MaintenanceText:    "تعمیرات سرور",
		OnError: func(err error, c *Context) {
			if err != nil && strings.Contains(err.Error(), "query is too old") {
				return
			}
			log.Printf("[Bale Bot Error]: %v", err)
		},
	}
	bot.ctxPool.New = func() any { return &Context{} }

	bot.Use(func(c *Context) {
		middleware.Recovery(func(err error) {
			if bot.OnError != nil {
				bot.OnError(err, c)
			}
		})(func() {
			c.Next()
		})
	})

	bot.Use(func(c *Context) {
		userID := c.SenderID()

		if userID > 0 {
			bot.muSettings.RLock()
			isBlacklisted := bot.Blacklist[userID]
			isMaintenance := bot.Maintenance
			maintenanceAdminID := bot.MaintenanceAdminID
			maintenanceText := bot.MaintenanceText
			bot.muSettings.RUnlock()

			if isBlacklisted {
				c.Abort()
				return
			}
			if isMaintenance && userID != maintenanceAdminID {
				c.Reply(maintenanceText)
				c.Abort()
				return
			}
		}
		c.Next()
	})

	bot.bgSemaphore = make(chan struct{}, 100)
	bot.shieldMode = 0
	bot.Shield = NewSystemShield(bot)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		var lastUpdates uint64
		for range ticker.C {
			currentUpdates := atomic.LoadUint64(&bot.metrics.TotalUpdates)
			diff := currentUpdates - lastUpdates
			lastUpdates = currentUpdates

			ups := float64(diff) / 10.0
			queueDepth := len(bot.workerChan)

			if queueDepth > 800 || ups > 150 {
				bot.Shield.Activate()
				if atomic.CompareAndSwapUint32(&bot.shieldMode, 0, 1) {
					anomalyErr := fmt.Errorf("🚨 TRAFFIC ANOMALY DETECTED! UPS: %.2f | Queue: %d/1000", ups, queueDepth)
					if bot.OnError != nil {
						bot.OnError(anomalyErr, nil)
					}
				}
			} else if queueDepth < 100 && ups < 10 {
				bot.Shield.Deactivate()
				if atomic.CompareAndSwapUint32(&bot.shieldMode, 1, 0) {
					log.Println("🛡️ [Traffic Shield] Deactivated. Bot returned to normal mode.")
				}
			}
		}
	}()

	bot.OnCallbackData("_sys_cfg", func(c *Context) {
		args := c.CallbackArgs()
		if len(args) == 0 {
			return
		}
		key := args[0]

		_ = c.Answer("تغییرات اعمال شد", false)

		db := db.NewLocalDB("gobale_settings.db")

		bot.muSettings.Lock()
		for _, entry := range bot.settings {
			if entry.Key == key {
				*entry.Ptr = !*entry.Ptr
				_ = db.Set(key, *entry.Ptr)
				break
			}
		}
		bot.muSettings.Unlock()

		_, _ = c.EditSettingsMenu("⚙️ منوی تنظیمات سیستمی:")
	})

	bot.OptimizeForHardware()

	return bot
}

func (b *Bot) SetMaxBackgroundTasks(n int) {
	if n > 0 {
		b.bgSemaphore = make(chan struct{}, n)
	}
}

func (b *Bot) OnState(state string, handlers ...HandlerFunc) {
	b.muRoutes.Lock()
	b.stateRoutes[state] = handlers
	b.muRoutes.Unlock()
}

func (b *Bot) Use(m ...HandlerFunc) {
	b.middlewares = append(b.middlewares, m...)
}

func (b *Bot) OnCommand(command string, handlers ...HandlerFunc) {
	b.handlers[command] = handlers
}

func (b *Bot) OnMessage(handlers ...HandlerFunc) {
	b.anyMessage = handlers
}

func (b *Bot) OnText(text string, handlers ...HandlerFunc) {
	b.textRoutes[text] = handlers
}

func (b *Bot) OnPreCheckout(handlers ...HandlerFunc) {
	b.preCheckoutHandlers = handlers
}

func (b *Bot) OnCallback(handlers ...HandlerFunc) {
	b.callbackHandlers = handlers
}

func (b *Bot) OnCallbackData(data string, handlers ...HandlerFunc) {
	b.muRoutes.Lock()
	b.callbackDataRoutes[data] = handlers
	b.muRoutes.Unlock()
}

func (b *Bot) ExecuteWithContext(ctx context.Context, req Request, result any) error {
	startTime := time.Now()
	err := b.BaseRequest(ctx, req.Method(), req.Params(), result)
	b.RecordLatency(time.Since(startTime))
	if err != nil {
		b.RecordFailure()
	} else {
		b.RecordMessage()
	}
	return err
}

func (b *Bot) Execute(req Request, result any) error {
	startTime := time.Now()
	err := b.BaseRequest(context.Background(), req.Method(), req.Params(), result)
	b.RecordLatency(time.Since(startTime))
	if err != nil {
		// b.setErr(err)
		b.RecordFailure()
		if b.OnError != nil {
			b.OnError(err, nil)
		}
	} else {
		b.RecordMessage()
	}
	return err
}

func (b *Bot) Err() error {
	ptr := b.errPtr.Load()
	if ptr == nil {
		return nil
	}
	return ptr.err
}

func (b *Bot) setErr(err error) {
	if err == nil {
		b.errPtr.Store(nil)
	} else {
		b.errPtr.Store(&wrappedError{err: err})
	}
}

func (b *Bot) ClearErr() {
	b.errPtr.Store(nil)
}

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

func (b *Bot) InlineBtn(text string, callbackData string, handler HandlerFunc) models.InlineKeyboardButton {
	b.OnCallbackData(callbackData, handler)
	return models.NewInlineKeyboardButtonData(text, callbackData)
}

func (b *Bot) InlineBtnText(text string, callbackData string, replyText string) models.InlineKeyboardButton {
	b.OnCallbackData(callbackData, func(c *Context) {
		c.Reply(replyText)
	})
	return models.NewInlineKeyboardButtonData(text, callbackData)
}

func (b *Bot) InlineBtnState(text string, callbackData string, nextState string, replyText string) models.InlineKeyboardButton {
	b.OnCallbackData(callbackData, func(c *Context) {
		c.SetState(nextState)
		c.Reply(replyText)
	})
	return models.NewInlineKeyboardButtonData(text, callbackData)
}

func (b *Bot) URLBtn(text string, url string) models.InlineKeyboardButton {
	return models.NewInlineKeyboardButtonURL(text, url)
}

func (b *Bot) CopyBtn(text string, textToCopy string) models.InlineKeyboardButton {
	return models.NewInlineKeyboardButtonCopy(text, textToCopy)
}

func (b *Bot) WebAppBtn(text string, url string) models.InlineKeyboardButton {
	return models.NewInlineKeyboardButtonWebApp(text, url)
}

func (b *Bot) ResolveChatID(target any) any {
	switch t := target.(type) {
	case int64, int, int32:
		return t
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return t
		}
		if strings.HasPrefix(t, "-") || (t[0] >= '0' && t[0] <= '9') {
			var num int64
			fmt.Sscanf(t, "%d", &num)
			if num != 0 {
				return num
			}
		}
		if strings.HasPrefix(t, "@") {
			return t
		}
		if strings.Contains(t, "join/") {
			cleanLink := strings.TrimPrefix(t, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			if cached, ok := b.inviteCache.Load(cleanLink); ok {
				return cached.(int64)
			}
			return t
		}
		if strings.Contains(t, "ble.ir/") {
			parts := strings.Split(t, "/")
			username := parts[len(parts)-1]
			if username != "" {
				return "@" + username
			}
		}
		return "@" + t
	}
	return target
}

func (b *Bot) SendMessage(chatID any, text string, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.SendMessage{
		ChatID:           b.ResolveChatID(chatID),
		Text:             stretchText(text),
		ParseMode:        config.ParseMode,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) SendLocation(chatID any, latitude, longitude float64, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.SendLocation{
		ChatID:             b.ResolveChatID(chatID),
		Latitude:           latitude,
		Longitude:          longitude,
		HorizontalAccuracy: config.HorizontalAccuracy,
		ReplyToMessageID:   config.ReplyToMessageID,
		ReplyMarkup:        config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) SendContact(chatID any, phoneNumber any, firstName string, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.SendContact{
		ChatID:           b.ResolveChatID(chatID),
		PhoneNumber:      phoneNumber,
		FirstName:        firstName,
		LastName:         config.LastName,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) SendChatAction(chatID any, action string) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.SendChatAction{
		ChatID: b.ResolveChatID(chatID),
		Action: action,
	}
	var ok bool
	err := b.Execute(req, &ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (b *Bot) AnswerCallbackQuery(callbackQueryID string, text string, showAlert bool) error {
	if b.Err() != nil {
		return b.Err()
	}

	req := methods.AnswerCallbackQuery{
		CallbackQueryID: callbackQueryID,
		Text:            text,
		ShowAlert:       showAlert,
	}
	return b.Execute(req, nil)
}

func (b *Bot) GetMe() (*models.User, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var user models.User
	err := b.Execute(methods.GetMe{}, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (b *Bot) GetChat(chatID any) (*models.ChatFullInfo, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var chat models.ChatFullInfo
	req := methods.GetChat{ChatID: b.ResolveChatID(chatID)}
	err := b.Execute(req, &chat)
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

func (b *Bot) DeleteMessage(chatID any, messageID int64) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.DeleteMessage{
		ChatID:    b.ResolveChatID(chatID),
		MessageID: messageID,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) EditMessageText(chatID any, messageID int64, text string, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.EditMessageText{
		ChatID:      b.ResolveChatID(chatID),
		MessageID:   messageID,
		Text:        stretchText(text),
		ParseMode:   config.ParseMode,
		ReplyMarkup: config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) EditMessageCaption(chatID any, messageID int64, caption string, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.EditMessageCaption{
		ChatID:      b.ResolveChatID(chatID),
		MessageID:   messageID,
		Caption:     caption,
		ReplyMarkup: config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) EditMessageReplyMarkup(chatID any, messageID int64, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.EditMessageReplyMarkup{
		ChatID:      b.ResolveChatID(chatID),
		MessageID:   messageID,
		ReplyMarkup: config.ReplyMarkup,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) LeaveChat(chatID any) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.LeaveChat{ChatID: b.ResolveChatID(chatID)}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) GetChatAdministrators(chatID any) ([]models.ChatMember, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var admins []models.ChatMember
	req := methods.GetChatAdministrators{ChatID: b.ResolveChatID(chatID)}
	err := b.Execute(req, &admins)
	if err != nil {
		return nil, err
	}
	return admins, nil
}

func (b *Bot) GetChatMembersCount(chatID any) (int, error) {
	if b.Err() != nil {
		return 0, b.Err()
	}

	req := methods.GetChatMembersCount{ChatID: b.ResolveChatID(chatID)}
	var count int
	err := b.Execute(req, &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (b *Bot) GetChatMember(chatID any, userID int64) (*models.ChatMember, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var member models.ChatMember
	req := methods.GetChatMember{
		ChatID: b.ResolveChatID(chatID),
		UserID: userID,
	}
	err := b.Execute(req, &member)
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (b *Bot) BanChatMember(chatID any, userID int64) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.BanChatMember{
		ChatID: b.ResolveChatID(chatID),
		UserID: userID,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) UnbanChatMember(chatID any, userID int64, opts ...Option) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.UnbanChatMember{
		ChatID:       b.ResolveChatID(chatID),
		UserID:       userID,
		OnlyIfBanned: config.OnlyIfBanned,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) PromoteChatMember(chatID any, userID int64, opts ...Option) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.PromoteChatMember{
		ChatID:              b.ResolveChatID(chatID),
		UserID:              userID,
		CanChangeInfo:       config.CanChangeInfo,
		CanPostMessages:     config.CanPostMessages,
		CanEditMessages:     config.CanEditMessages,
		CanDeleteMessages:   config.CanDeleteMessages,
		CanManageVideoChats: config.CanManageVideoChats,
		CanInviteUsers:      config.CanInviteUsers,
		CanRestrictMembers:  config.CanRestrictMembers,
	}

	var ok bool
	err := b.Execute(req, &ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (b *Bot) SetChatTitle(chatID any, title string) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.SetChatTitle{ChatID: b.ResolveChatID(chatID), Title: title}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) SetChatDescription(chatID any, description string) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.SetChatDescription{ChatID: b.ResolveChatID(chatID), Description: description}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) DeleteChatPhoto(chatID any) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.DeleteChatPhoto{ChatID: b.ResolveChatID(chatID)}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) SetChatPhoto(chatID any, photo models.InputFile) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.SetChatPhoto{
		ChatID: b.ResolveChatID(chatID),
	}
	photo.Field = "photo"
	var ok bool
	err := b.BaseRequestMultipart(context.Background(), "setChatPhoto", req, []models.InputFile{photo}, &ok)
	if err != nil {
		b.setErr(err)
		if b.OnError != nil {
			b.OnError(err, nil)
		}
	}
	return ok, err
}

func (b *Bot) CreateChatInviteLink(chatID any) (*models.ChatInviteLink, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var link models.ChatInviteLink
	req := methods.CreateChatInviteLink{ChatID: b.ResolveChatID(chatID)}
	err := b.Execute(req, &link)
	if err != nil {
		return nil, err
	}
	if link.InviteLink != "" {
		resolvedChatID := b.ResolveChatID(chatID)
		if cid, ok := resolvedChatID.(int64); ok {
			cleanLink := strings.TrimPrefix(link.InviteLink, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			b.inviteCache.Store(cleanLink, cid)
		}
	}
	return &link, nil
}

func (b *Bot) RevokeChatInviteLink(chatID any, inviteLink string) (*models.ChatInviteLink, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var link models.ChatInviteLink
	req := methods.RevokeChatInviteLink{ChatID: b.ResolveChatID(chatID), InviteLink: inviteLink}
	err := b.Execute(req, &link)
	if err != nil {
		return nil, err
	}
	if link.InviteLink != "" {
		resolvedChatID := b.ResolveChatID(chatID)
		if cid, ok := resolvedChatID.(int64); ok {
			cleanLink := strings.TrimPrefix(link.InviteLink, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			b.inviteCache.Store(cleanLink, cid)
		}
	}
	return &link, nil
}

func (b *Bot) ExportChatInviteLink(chatID any) (string, error) {
	if b.Err() != nil {
		return "", b.Err()
	}

	var link string
	req := methods.ExportChatInviteLink{ChatID: b.ResolveChatID(chatID)}
	err := b.Execute(req, &link)
	if err != nil {
		return "", err
	}
	if link != "" {
		resolvedChatID := b.ResolveChatID(chatID)
		if cid, ok := resolvedChatID.(int64); ok {
			cleanLink := strings.TrimPrefix(link, "http://")
			cleanLink = strings.TrimPrefix(cleanLink, "https://")
			b.inviteCache.Store(cleanLink, cid)
		}
	}
	return link, nil
}

func (b *Bot) GetFile(fileID string) (*models.File, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	var file models.File
	req := methods.GetFile{FileID: fileID}
	err := b.Execute(req, &file)
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (b *Bot) GetDownloadURL(filePath string) string {
	return fmt.Sprintf("https://tapi.bale.ai/file/bot%s/%s", b.Client.token, filePath)
}

func (b *Bot) SendMediaGroup(chatID any, media []any, opts ...Option) ([]models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	var filesToUpload []models.InputFile
	var resolvedMedia []any
	var filesToClose []*os.File

	defer func() {
		for _, f := range filesToClose {
			_ = f.Close()
		}
	}()

	for idx, item := range media {
		// لاگ نوع فیزیکی متغیر دریافتی در زمان اجرا جهت عیب‌یابی دقیق نوع (Type)
		log.Printf("🔍 [DEBUG] Album item %d type: %T\n", idx, item)

		switch m := item.(type) {
		// حالت اول: مقدار فیزیکی
		case models.InputMediaPhoto:
			if !strings.HasPrefix(m.Media, "http://") && !strings.HasPrefix(m.Media, "https://") && len(m.Media) < 100 {
				fieldName := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("❌ [Bale Error] local photo file not found or cannot be opened (%s): %w", m.Media, err)
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, models.InputFile{
					Field:    fieldName,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + fieldName
			}
			resolvedMedia = append(resolvedMedia, m)

		// حالت دوم: آدرس پوینتر فیزیکی
		case *models.InputMediaPhoto:
			if !strings.HasPrefix(m.Media, "http://") && !strings.HasPrefix(m.Media, "https://") && len(m.Media) < 100 {
				fieldName := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("❌ [Bale Error] local photo file not found or cannot be opened (%s): %w", m.Media, err)
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, models.InputFile{
					Field:    fieldName,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + fieldName
			}
			resolvedMedia = append(resolvedMedia, m)

		// حالت سوم: مقدار ویدیو
		case models.InputMediaVideo:
			if !strings.HasPrefix(m.Media, "http://") && !strings.HasPrefix(m.Media, "https://") && len(m.Media) < 100 {
				fieldName := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("❌ [Bale Error] local video file not found or cannot be opened (%s): %w", m.Media, err)
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, models.InputFile{
					Field:    fieldName,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + fieldName
			}
			resolvedMedia = append(resolvedMedia, m)

		// حالت چهارم: پوینتر ویدیو
		case *models.InputMediaVideo:
			if !strings.HasPrefix(m.Media, "http://") && !strings.HasPrefix(m.Media, "https://") && len(m.Media) < 100 {
				fieldName := fmt.Sprintf("file_%d", idx)
				file, err := os.Open(m.Media)
				if err != nil {
					return nil, fmt.Errorf("❌ [Bale Error] local video file not found or cannot be opened (%s): %w", m.Media, err)
				}
				filesToClose = append(filesToClose, file)
				filesToUpload = append(filesToUpload, models.InputFile{
					Field:    fieldName,
					FileName: filepath.Base(m.Media),
					Reader:   file,
				})
				m.Media = "attach://" + fieldName
			}
			resolvedMedia = append(resolvedMedia, m)

		default:
			resolvedMedia = append(resolvedMedia, item)
		}
	}

	req := methods.SendMediaGroup{
		ChatID:           b.ResolveChatID(chatID),
		Media:            resolvedMedia,
		ReplyToMessageID: config.ReplyToMessageID,
	}

	var result []models.Message

	if len(filesToUpload) > 0 {
		startTime := time.Now()
		err := b.BaseRequestMultipart(context.Background(), "sendMediaGroup", req, filesToUpload, &result)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return result, nil
	}

	err := b.Execute(req, &result)
	return result, err
}

func (b *Bot) PinChatMessage(chatID any, messageID int64) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.PinChatMessage{
		ChatID:    b.ResolveChatID(chatID),
		MessageID: messageID,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) UnPinChatMessage(chatID any, messageID int64) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.UnPinChatMessage{
		ChatID:    b.ResolveChatID(chatID),
		MessageID: messageID,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) UnpinAllChatMessages(chatID any) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.UnpinAllChatMessages{
		ChatID: b.ResolveChatID(chatID),
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) AskReview(userID int64, delaySeconds int) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.AskReview{
		UserID:       userID,
		DelaySeconds: delaySeconds,
	}
	var ok bool
	err := b.Execute(req, &ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (b *Bot) UploadStickerFile(userID int64, sticker models.InputFile) (*models.File, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	req := methods.UploadStickerFile{
		UserID: userID,
	}
	sticker.Field = "sticker"
	var file models.File
	err := b.BaseRequestMultipart(context.Background(), "uploadStickerFile", req, []models.InputFile{sticker}, &file)
	if err != nil {
		b.setErr(err)
		if b.OnError != nil {
			b.OnError(err, nil)
		}
	}
	return &file, err
}

func (b *Bot) CreateNewStickerSet(userID int64, name string, title string, stickers []models.InputSticker) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.CreateNewStickerSet{
		UserID:  userID,
		Name:    name,
		Title:   title,
		Sticker: stickers,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) AddStickerToSet(userID int64, name string, sticker models.InputSticker) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.AddStickerToSet{
		UserID:  userID,
		Name:    name,
		Sticker: sticker,
	}
	var ok bool
	err := b.Execute(req, &ok)
	return ok, err
}

func (b *Bot) SendInvoice(chatID any, title, description, payload, providerToken string, prices []models.LabeledPrice, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	req := methods.SendInvoice{
		ChatID:        b.ResolveChatID(chatID),
		Title:         title,
		Description:   description,
		Payload:       payload,
		ProviderToken: providerToken,
		Currency:      "IRR",
		Prices:        prices,
	}

	var msg models.Message
	err := b.Execute(req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) AnswerPreCheckoutQuery(preCheckoutQueryID string, ok bool, errorMessage string) (bool, error) {
	if b.Err() != nil {
		return false, b.Err()
	}

	req := methods.AnswerPreCheckoutQuery{
		PreCheckoutQueryID: preCheckoutQueryID,
		OK:                 ok,
		ErrorMessage:       errorMessage,
	}
	var result bool
	err := b.Execute(req, &result)
	return result, err
}

func (b *Bot) GetTransaction(transactionID string) (*models.Transaction, error) {
	var tx models.Transaction
	req := methods.GetTransaction{TransactionID: transactionID}
	err := b.Execute(req, &tx)
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (b *Bot) ForwardMessage(ctx context.Context, chatID any, fromChatID any, messageID int64) (*models.Message, error) {
	req := methods.ForwardMessage{
		ChatID:     b.ResolveChatID(chatID),
		FromChatID: b.ResolveChatID(fromChatID),
		MessageID:  messageID,
	}
	var msg models.Message
	err := b.ExecuteWithContext(ctx, req, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (b *Bot) CopyMessage(ctx context.Context, chatID any, fromChatID any, messageID int64) (*models.MessageId, error) {
	req := methods.CopyMessage{
		ChatID:     b.ResolveChatID(chatID),
		FromChatID: b.ResolveChatID(fromChatID),
		MessageID:  messageID,
	}
	var msgID models.MessageId
	err := b.ExecuteWithContext(ctx, req, &msgID)
	if err != nil {
		return nil, err
	}
	return &msgID, nil
}

func (b *Bot) StartWorkers(ctx context.Context) {
	for i := 0; i < b.numWorkers; i++ {
		b.workersWg.Add(1)

		go func() {
			defer b.workersWg.Done()

			for update := range b.workerChan {
				b.processUpdate(ctx, update)
			}
		}()
	}
}

func (b *Bot) processUpdate(ctx context.Context, update *models.Update) {
	NormalizeUpdateDigits(update)

	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/start ") {
		parts := strings.Fields(update.Message.Text)
		if len(parts) > 1 {
			deepLink := parts[1]
			b.Sessions.Get(update.Message.Chat.ID).SetData("deep_link", deepLink)
		}
	}

	if update.Message != nil && update.Message.Text == "dummy" {
		return
	}

	c := b.ctxPool.Get().(*Context)
	c.Bot = b
	c.Update = update
	c.Message = update.Message
	c.index = -1
	c.ctx = ctx
	c.err = nil
	c.Logger = b.Logger

	if c.Keys != nil {
		for k := range c.Keys {
			delete(c.Keys, k)
		}
	}

	if b.Logger {
		if update.Message != nil {
			log.Printf("[DEBUG] New Message: ChatID=%d, Text=%q", update.Message.Chat.ID, update.Message.Text)
		} else if update.CallbackQuery != nil {
			log.Printf("[DEBUG] New CallbackQuery: From=%d, Data=%q", update.CallbackQuery.From.ID, update.CallbackQuery.Data)
		} else if update.PreCheckoutQuery != nil {
			log.Printf("[DEBUG] New PreCheckoutQuery: ID=%s, Amount=%d", update.PreCheckoutQuery.ID, update.PreCheckoutQuery.TotalAmount)
		}
	}

	var chain []HandlerFunc
	chain = append(chain, b.middlewares...)

	var matchedHandlers []HandlerFunc
	var ok bool

	if update.Message != nil {
		text := update.Message.Text
		chatID := update.Message.Chat.ID
		userState := b.Sessions.Get(chatID).GetState()

		b.muRoutes.RLock()
		if text != "" && strings.HasPrefix(text, "/") {
			cmd := strings.Fields(text)[0]
			if matchedHandlers, ok = b.handlers[cmd]; ok {
				chain = append(chain, matchedHandlers...)
			} else {
				chain = append(chain, b.anyMessage...)
			}
		} else if userState != "" {
			if matchedHandlers, ok = b.stateRoutes[userState]; ok {
				chain = append(chain, matchedHandlers...)
			} else if text != "" {
				if matchedHandlers, ok = b.textRoutes[text]; ok {
					chain = append(chain, matchedHandlers...)
				} else {
					chain = append(chain, b.anyMessage...)
				}
			} else {
				chain = append(chain, b.anyMessage...)
			}
		} else if text != "" {
			if matchedHandlers, ok = b.textRoutes[text]; ok {
				chain = append(chain, matchedHandlers...)
			} else {
				chain = append(chain, b.anyMessage...)
			}
		} else {
			chain = append(chain, b.anyMessage...)
		}
		b.muRoutes.RUnlock()

	} else if update.CallbackQuery != nil {
		c.Message = update.CallbackQuery.Message
		data := update.CallbackQuery.Data
		parts := strings.Split(data, ":")
		prefix := parts[0]

		b.muRoutes.RLock()
		if matchedHandlers, ok = b.callbackDataRoutes[data]; ok {
			chain = append(chain, matchedHandlers...)
		} else if matchedHandlers, ok = b.callbackDataRoutes[prefix]; ok {
			chain = append(chain, matchedHandlers...)
		} else {
			chain = append(chain, b.callbackHandlers...)
		}
		b.muRoutes.RUnlock()

		chain = append(chain, func(c *Context) {
			_ = c.Bot.ExecuteWithContext(c.ctx, methods.AnswerCallbackQuery{CallbackQueryID: update.CallbackQuery.ID}, nil)
			c.Next()
		})
	} else if update.PreCheckoutQuery != nil {
		chain = append(chain, b.preCheckoutHandlers...)
	}

	if len(chain) > 0 {
		c.handlers = chain
		c.Next()
	}

	c.handlers = nil
	c.Update = nil
	c.Message = nil
	c.ctx = nil
	b.ctxPool.Put(c)
}

func (b *Bot) RunPolling() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Clearing any active Webhook on Bale servers to enable Polling...")
	err := b.BaseRequest(ctx, "deleteWebhook", nil, nil)
	if err != nil {
		log.Printf("Warning: failed to clear webhook: %v", err)
	} else {
		log.Println("Webhook cleared successfully!")
	}

	b.StartWorkers(ctx)
	log.Printf("Bale Bot started in POLLING mode with %d workers...", b.numWorkers)

	offset := -1
	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping polling loop...")

			close(b.workerChan)

			log.Println("⏳ Waiting for active workers to drain the queue...")
			b.workersWg.Wait()

			log.Println("Saving active sessions to disk...")
			if saver, ok := b.Sessions.(interface{ Save() error }); ok {
				_ = saver.Save()
			}
			log.Println("🟢 Bale Bot shut down gracefully with zero lost updates!")
			return
		default:
			params := map[string]any{"offset": offset, "limit": 100, "timeout": 20}
			var updates []models.Update
			err := b.BaseRequest(ctx, "getUpdates", params, &updates)
			if err != nil {
				log.Printf("Polling API error: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}
			for _, update := range updates {
				b.workerChan <- &update
				offset = update.UpdateID + 1
			}
		}
	}
}

func (b *Bot) RunWebhook(addr string, path string, publicURL string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !strings.HasSuffix(addr, ":443") && !strings.HasSuffix(addr, ":88") {
		log.Printf("[Webhook Warning] Your address port in '%s' is not standard! Bale API only supports ports 443 and 88 for direct Webhooks (Note: You can safely ignore this if you are running behind a reverse proxy like Nginx or Cloudflare).", addr)
	}

	b.StartWorkers(ctx)
	log.Printf("Bale Bot started in WEBHOOK mode on %s%s with %d workers...", addr, path, b.numWorkers)

	if publicURL != "" {
		log.Println("Registering Webhook on Bale servers...")
		webhookURL := strings.TrimSuffix(publicURL, "/") + path
		params := map[string]any{"url": webhookURL}
		err := b.BaseRequest(ctx, "setWebhook", params, nil)
		if err != nil {
			log.Printf("Warning: failed to set webhook: %v", err)
		} else {
			log.Printf("Webhook set successfully to: %s", webhookURL)
		}
	}

	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var bodyReader io.ReadCloser = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(r.Body)
			if err == nil {
				bodyReader = gzipReader
			}
		}
		defer bodyReader.Close()

		var update models.Update
		if err := json.NewDecoder(bodyReader).Decode(&update); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b.workerChan <- &update
		w.WriteHeader(http.StatusOK)
	})

	cert, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to generate self-signed cert: %w", err)
	}

	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	go func() {
		<-ctx.Done()
		log.Println("Stopping Webhook server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)

		close(b.workerChan)

		log.Println("⏳ Waiting for active workers to drain the queue...")
		b.workersWg.Wait()

		log.Println("Saving active sessions to disk...")
		if saver, ok := b.Sessions.(interface{ Save() error }); ok {
			_ = saver.Save()
		}
		log.Println("🟢 Bale Bot Webhook shut down gracefully with zero lost updates!")
	}()

	return server.ListenAndServeTLS("", "")
}

func (b *Bot) RunWebhookTLS(addr string, path string, publicURL string, certFile string, keyFile string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !strings.HasSuffix(addr, ":443") && !strings.HasSuffix(addr, ":88") {
		log.Printf("[Webhook Warning] Your address port in '%s' is not standard! Bale API only supports ports 443 and 88 for direct Webhooks (Note: You can safely ignore this if you are running behind a reverse proxy like Nginx or Cloudflare).", addr)
	}

	b.StartWorkers(ctx)
	log.Printf("Bale Bot started in WEBHOOK TLS mode on %s%s with %d workers...", addr, path, b.numWorkers)

	if publicURL != "" {
		log.Println("Registering Webhook on Bale servers...")
		webhookURL := strings.TrimSuffix(publicURL, "/") + path
		params := map[string]any{"url": webhookURL}
		err := b.BaseRequest(ctx, "setWebhook", params, nil)
		if err != nil {
			log.Printf("Warning: failed to set webhook: %v", err)
		} else {
			log.Printf("Webhook set successfully to: %s", webhookURL)
		}
	}

	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var bodyReader io.ReadCloser = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(r.Body)
			if err == nil {
				bodyReader = gzipReader
			}
		}
		defer bodyReader.Close()

		var update models.Update
		if err := json.NewDecoder(bodyReader).Decode(&update); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b.workerChan <- &update
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Println("Stopping Webhook TLS server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)

		close(b.workerChan)

		log.Println("⏳ Waiting for active workers to drain the queue...")
		b.workersWg.Wait()

		log.Println("Saving active sessions to disk...")
		if saver, ok := b.Sessions.(interface{ Save() error }); ok {
			_ = saver.Save()
		}
		log.Println("🟢 Bale Bot Webhook TLS shut down gracefully with zero lost updates!")
	}()

	return server.ListenAndServeTLS(certFile, keyFile)
}

func (b *Bot) RecordUpdate() {
	atomic.AddUint64(&b.metrics.TotalUpdates, 1)
}

func (b *Bot) RecordMessage() {
	atomic.AddUint64(&b.metrics.ProcessedMessages, 1)
}

func (b *Bot) RecordFailure() {
	atomic.AddUint64(&b.metrics.FailedRequests, 1)
}

func (b *Bot) RecordLatency(duration time.Duration) {
	atomic.StoreInt64(&b.metrics.NetworkLatencyNs, int64(duration))
}

func (b *Bot) GetMetrics() Metrics {
	return Metrics{
		TotalUpdates:      atomic.LoadUint64(&b.metrics.TotalUpdates),
		ProcessedMessages: atomic.LoadUint64(&b.metrics.ProcessedMessages),
		FailedRequests:    atomic.LoadUint64(&b.metrics.FailedRequests),
		NetworkLatencyNs:  atomic.LoadInt64(&b.metrics.NetworkLatencyNs),
	}
}

func (b *Bot) Schedule(delay time.Duration, task func()) {
	time.AfterFunc(delay, func() {
		defer func() {
			if r := recover(); r != nil {
				if b.OnError != nil {
					b.OnError(fmt.Errorf("scheduled task panic: %v", r), nil)
				}
			}
		}()
		task()
	})
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"GoBale"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}

func (b *Bot) SetMemoryLimit(megabytes int64) {
	debug.SetMemoryLimit(megabytes * 1024 * 1024)
}

func (b *Bot) SendUpdateToWorkerChan(update *models.Update) {
	b.workerChan <- update
}

func (b *Bot) RegisterCommand(command string, description string, handlers ...HandlerFunc) {
	b.OnCommand(command, handlers...)
	b.commands = append(b.commands, commandInfo{
		Command:     command,
		Description: description,
	})
}

func (b *Bot) GenerateHelpMenu() string {
	if len(b.commands) == 0 {
		return "دستوری ثبت نشده است."
	}

	var sb strings.Builder
	sb.WriteString("📋 *راهنمای دستورات ربات*:\n\n")
	for _, cmd := range b.commands {
		sb.WriteString(fmt.Sprintf("%s - %s\n", cmd.Command, cmd.Description))
	}
	return sb.String()
}

func (b *Bot) SetTranslations(translations map[string]map[string]string) {
	b.i18n = translations
}

type ScheduledTask struct {
	stop chan struct{}
}

func (t *ScheduledTask) Stop() {
	close(t.stop)
}

func (b *Bot) safeExecuteTask(task func()) {
	defer func() {
		if r := recover(); r != nil {
			if b.OnError != nil {
				b.OnError(fmt.Errorf("scheduled task panic: %v", r), nil)
			}
		}
	}()
	task()
}

func (b *Bot) ScheduleEvery(interval time.Duration, task func()) *ScheduledTask {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.safeExecuteTask(task)
			case <-stop:
				return
			}
		}
	}()
	return &ScheduledTask{stop: stop}
}

func (b *Bot) ScheduleDaily(hour, minute int, task func()) *ScheduledTask {
	stop := make(chan struct{})
	go func() {
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
			if nextRun.Before(now) || nextRun.Sub(now) < 1*time.Second {
				nextRun = nextRun.Add(24 * time.Hour)
			}

			delay := nextRun.Sub(now)
			select {
			case <-time.After(delay):
				b.safeExecuteTask(task)
			case <-stop:
				return
			}
		}
	}()
	return &ScheduledTask{stop: stop}
}

func (b *Bot) RemoveState(state string) {
	b.muRoutes.Lock()
	delete(b.stateRoutes, state)
	b.muRoutes.Unlock()
}

func (b *Bot) RemoveCallbackData(data string) {
	b.muRoutes.Lock()
	delete(b.callbackDataRoutes, data)
	b.muRoutes.Unlock()
}

func (b *Bot) GetMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	limit := debug.SetMemoryLimit(-1)

	return MemoryStats{
		AllocMegabytes:     float64(m.Alloc) / (1024 * 1024),
		SysMegabytes:       float64(m.Sys) / (1024 * 1024),
		HeapAllocMegabytes: float64(m.HeapAlloc) / (1024 * 1024),
		NumGC:              m.NumGC,
		MemoryLimitBytes:   limit,
	}
}

func (b *Bot) SetGCPercent(percent int) int {
	return debug.SetGCPercent(percent)
}

func (b *Bot) Group(middlewares ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		bot:         b,
		middlewares: middlewares,
	}
}

func (g *RouterGroup) Group(middlewares ...HandlerFunc) *RouterGroup {
	combined := append(g.middlewares, middlewares...)
	return &RouterGroup{
		bot:         g.bot,
		middlewares: combined,
	}
}

func (g *RouterGroup) OnCommand(command string, handlers ...HandlerFunc) *RouterGroup {
	finalHandlers := append(g.middlewares, handlers...)
	g.bot.OnCommand(command, finalHandlers...)
	return g
}

func (g *RouterGroup) OnText(text string, handlers ...HandlerFunc) *RouterGroup {
	finalHandlers := append(g.middlewares, handlers...)
	g.bot.OnText(text, finalHandlers...)
	return g
}

func (g *RouterGroup) OnCallbackData(data string, handlers ...HandlerFunc) *RouterGroup {
	finalHandlers := append(g.middlewares, handlers...)
	g.bot.OnCallbackData(data, finalHandlers...)
	return g
}

func (b *Bot) ReportErrorToAdmin(botName string, targetChatID any) {

	originalOnError := b.OnError

	resolvedTarget := b.ResolveChatID(targetChatID)

	b.OnError = func(err error, c *Context) {

		if err != nil && strings.Contains(err.Error(), "query is too old") {
			return
		}

		if originalOnError != nil {
			originalOnError(err, c)
		}

		var userInfo string
		if c != nil && c.Message != nil && c.Message.From != nil {
			userInfo = fmt.Sprintf(
				"👤 فرستنده: %s (%d)\n📍 چت: %s (%d)\n💬 متن پیام: %q",
				utils.Bold(c.Message.From.FirstName),
				c.Message.From.ID,
				utils.Bold(c.Message.Chat.Title),
				c.Message.Chat.ID,
				c.Message.Text,
			)
		} else {
			userInfo = "🤖 خطای سیستمی (خارج از کانتکست پیام کاربر)"
		}

		report := fmt.Sprintf(
			"🤖 %s\n\n❌ خطا:\n`%v`\n\n%s",
			utils.Bold(fmt.Sprintf("[%s] گزارش خطای رانتایم", botName)),
			err,
			userInfo,
		)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ [Bale Critical] Panic during sending error report to admin: %v\n", r)
				}
			}()

			req := methods.SendMessage{
				ChatID:    resolvedTarget,
				Text:      report,
				ParseMode: "Markdown",
			}
			var msg models.Message
			_ = b.BaseRequest(context.Background(), req.Method(), req.Params(), &msg)
		}()
	}
}

func (b *Bot) GetWorkerChan() chan *models.Update {
	return b.workerChan
}

func (b *Bot) GetWorkersWg() *sync.WaitGroup {
	return &b.workersWg
}

func (b *Bot) FreeMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}

func (b *Bot) RegisterSetting(key string, label string, ptr *bool) *Bot {
	b.muSettings.Lock()
	defer b.muSettings.Unlock()

	b.settings = append(b.settings, SettingEntry{Key: key, Label: label, Ptr: ptr})
	db := db.NewLocalDB("gobale_settings.db")
	if val, ok := db.Get(key); ok {
		if bVal, ok := val.(bool); ok {
			*ptr = bVal
		}
	}
	return b
}

func (b *Bot) OptimizeForHardware() {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	log.Printf("🔍 [Hardware Monitor] OS: %s | Arch: %s | CPU Cores: %d\n", goos, goarch, runtime.NumCPU())

	switch goarch {
	case "arm", "arm64":
		b.SetGCPercent(80)
		if b.GetMemoryStats().MemoryLimitBytes == math.MaxInt64 {
			b.SetMemoryLimit(16)
		}
		log.Println("🛡️ [Hardware Tuning] Optimized for low-power/mobile ARM architecture.")

	case "amd64", "386":
		b.SetGCPercent(100)
		if b.GetMemoryStats().MemoryLimitBytes == math.MaxInt64 {
			b.SetMemoryLimit(32)
		}
		log.Println("🛡️ [Hardware Tuning] Optimized for high-performance x86/64 Server architecture.")
	}
	switch goos {
	case "windows":
		log.Println("🛡️ [Hardware Tuning] OS specific tweaks applied for Windows Environment.")
	case "linux", "android":
		log.Println("🛡️ [Hardware Tuning] OS specific tweaks applied for Unix-like Environment.")
	}
}

func NormalizeUpdateDigits(u *models.Update) {
	if u == nil {
		return
	}
	if u.Message != nil {
		if u.Message.Text != "" {
			u.Message.Text = utils.ToEnglishDigits(u.Message.Text)
		}
		if u.Message.Caption != "" {
			u.Message.Caption = utils.ToEnglishDigits(u.Message.Caption)
		}
	}
	if u.EditedMessage != nil {
		if u.EditedMessage.Text != "" {
			u.EditedMessage.Text = utils.ToEnglishDigits(u.EditedMessage.Text)
		}
		if u.EditedMessage.Caption != "" {
			u.EditedMessage.Caption = utils.ToEnglishDigits(u.EditedMessage.Caption)
		}
	}
	if u.CallbackQuery != nil && u.CallbackQuery.Data != "" {
		u.CallbackQuery.Data = utils.ToEnglishDigits(u.CallbackQuery.Data)
	}
}

func (b *Bot) SendPhotoWithContext(ctx context.Context, chatID any, photo any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendPhoto{
		ChatID:           resolvedChatID,
		FromChatID:       config.FromChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch p := photo.(type) {
	case string:
		if isLocalFile(p) {
			if cached, ok := b.Client.fileCache.Load(p); ok {
				req.Photo = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(p)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(p),
				Reader:   file,
				Field:    "photo",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendPhoto", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if len(msg.Photo) > 0 {
				bestPhoto := msg.Photo[len(msg.Photo)-1]
				b.Client.fileCache.Store(p, bestPhoto.FileID)
			}
			return &msg, nil
		}
		req.Photo = p
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		p.Field = "photo"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendPhoto", req, []models.InputFile{p}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid photo type")
}

func (b *Bot) SendPhoto(chatID any, photo any, opts ...Option) (*models.Message, error) {
	return b.SendPhotoWithContext(context.Background(), chatID, photo, opts...)
}

func (b *Bot) SendAudioWithContext(ctx context.Context, chatID any, audio any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendAudio{
		ChatID:           resolvedChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch a := audio.(type) {
	case string:
		if isLocalFile(a) {
			if cached, ok := b.Client.fileCache.Load(a); ok {
				req.Audio = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(a)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(a),
				Reader:   file,
				Field:    "audio",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendAudio", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if msg.Audio != nil {
				b.Client.fileCache.Store(a, msg.Audio.FileID)
			}
			return &msg, nil
		}
		req.Audio = a
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		a.Field = "audio"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendAudio", req, []models.InputFile{a}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid audio type")
}

func (b *Bot) SendAudio(chatID any, audio any, opts ...Option) (*models.Message, error) {
	return b.SendAudioWithContext(context.Background(), chatID, audio, opts...)
}

func (b *Bot) SendDocumentWithContext(ctx context.Context, chatID any, document any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendDocument{
		ChatID:           resolvedChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch d := document.(type) {
	case string:
		if isLocalFile(d) {
			if cached, ok := b.Client.fileCache.Load(d); ok {
				req.Document = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(d)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(d),
				Reader:   file,
				Field:    "document",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendDocument", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if msg.Document != nil {
				b.Client.fileCache.Store(d, msg.Document.FileID)
			}
			return &msg, nil
		}
		req.Document = d
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		d.Field = "document"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendDocument", req, []models.InputFile{d}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid document type")
}

func (b *Bot) SendDocument(chatID any, document any, opts ...Option) (*models.Message, error) {
	return b.SendDocumentWithContext(context.Background(), chatID, document, opts...)
}

func (b *Bot) SendVideoWithContext(ctx context.Context, chatID any, video any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendVideo{
		ChatID:           resolvedChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch v := video.(type) {
	case string:
		if isLocalFile(v) {
			if cached, ok := b.Client.fileCache.Load(v); ok {
				req.Video = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(v)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(v),
				Reader:   file,
				Field:    "video",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendVideo", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if msg.Video != nil {
				b.Client.fileCache.Store(v, msg.Video.FileID)
			}
			return &msg, nil
		}
		req.Video = v
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		v.Field = "video"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendVideo", req, []models.InputFile{v}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid video type")
}

func (b *Bot) SendVideo(chatID any, video any, opts ...Option) (*models.Message, error) {
	return b.SendVideoWithContext(context.Background(), chatID, video, opts...)
}

func (b *Bot) SendAnimationWithContext(ctx context.Context, chatID any, animation any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendAnimation{
		ChatID:           resolvedChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch a := animation.(type) {
	case string:
		if isLocalFile(a) {
			if cached, ok := b.Client.fileCache.Load(a); ok {
				req.Animation = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(a)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(a),
				Reader:   file,
				Field:    "animation",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendAnimation", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if msg.Animation != nil {
				b.Client.fileCache.Store(a, msg.Animation.FileID)
			}
			return &msg, nil
		}
		req.Animation = a
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		a.Field = "animation"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendAnimation", req, []models.InputFile{a}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid animation type")
}

func (b *Bot) SendAnimation(chatID any, animation any, opts ...Option) (*models.Message, error) {
	return b.SendAnimationWithContext(context.Background(), chatID, animation, opts...)
}

func (b *Bot) SendVoiceWithContext(ctx context.Context, chatID any, voice any, opts ...Option) (*models.Message, error) {
	if b.Err() != nil {
		return nil, b.Err()
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	resolvedChatID := b.ResolveChatID(chatID)
	req := methods.SendVoice{
		ChatID:           resolvedChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message

	switch v := voice.(type) {
	case string:
		if isLocalFile(v) {
			if cached, ok := b.Client.fileCache.Load(v); ok {
				req.Voice = cached.(string)
				err := b.ExecuteWithContext(ctx, req, &msg)
				return &msg, err
			}

			file, err := os.Open(v)
			if err != nil {
				b.setErr(err)
				return nil, err
			}
			defer file.Close()

			inputFile := models.InputFile{
				FileName: filepath.Base(v),
				Reader:   file,
				Field:    "voice",
			}
			startTime := time.Now()
			err = b.BaseRequestMultipart(ctx, "sendVoice", req, []models.InputFile{inputFile}, &msg)
			b.RecordLatency(time.Since(startTime))
			if err != nil {
				b.setErr(err)
				b.RecordFailure()
				if b.OnError != nil {
					b.OnError(err, nil)
				}
				return nil, err
			}

			b.RecordMessage()
			if msg.Voice != nil {
				b.Client.fileCache.Store(v, msg.Voice.FileID)
			}
			return &msg, nil
		}
		req.Voice = v
		err := b.ExecuteWithContext(ctx, req, &msg)
		return &msg, err

	case models.InputFile:
		v.Field = "voice"
		startTime := time.Now()
		err := b.BaseRequestMultipart(ctx, "sendVoice", req, []models.InputFile{v}, &msg)
		b.RecordLatency(time.Since(startTime))
		if err != nil {
			b.setErr(err)
			b.RecordFailure()
			if b.OnError != nil {
				b.OnError(err, nil)
			}
			return nil, err
		}
		b.RecordMessage()
		return &msg, err
	}

	return nil, fmt.Errorf("invalid voice type")
}

func (b *Bot) SendVoice(chatID any, voice any, opts ...Option) (*models.Message, error) {
	return b.SendVoiceWithContext(context.Background(), chatID, voice, opts...)
}