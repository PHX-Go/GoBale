package gobale

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PHX-Go/GoBale/methods"
	"github.com/PHX-Go/GoBale/models"
	"github.com/PHX-Go/GoBale/session"
)

type HandlerFunc func(*Context)

type Context struct {
	Bot      *Bot
	Update   *models.Update
	Message  *models.Message
	handlers []HandlerFunc
	index    int8
	mu       sync.RWMutex
	Keys     map[string]any
	ctx      context.Context
	err      error
	Logger   bool
}

type paginatedCacheItem struct {
	provider  func(page int, limit int) ([]models.InlineKeyboardButton, int, error)
	limit     int
	expiresAt time.Time
}

type PaginationRegistry struct {
	mu    sync.RWMutex
	items map[string]*paginatedCacheItem
}

var globalPaginationRegistry = &PaginationRegistry{
	items: make(map[string]*paginatedCacheItem),
}

func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

func (c *Context) Abort() {
	c.index = int8(len(c.handlers))
}

func (c *Context) DetermineChatID() (int64, error) {
	if c.Update == nil {
		return 0, errors.New("update object is nil")
	}
	if c.Update.Message != nil {
		return c.Update.Message.Chat.ID, nil
	}
	if c.Update.CallbackQuery != nil && c.Update.CallbackQuery.Message != nil {
		return c.Update.CallbackQuery.Message.Chat.ID, nil
	}
	return 0, errors.New("chat id could not be determined")
}

func (c *Context) Send(text string, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}

	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}

	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendMessage{
		ChatID:           c.Bot.ResolveChatID(chatID),
		Text:             text,
		ParseMode:        config.ParseMode,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var result models.Message
	activeCtx := c.ctx
	if c.Logger {
		activeCtx = context.WithValue(activeCtx, loggerKey, true)
	}

	err = c.Bot.ExecuteWithContext(activeCtx, req, &result)
	if err != nil {
		if strings.Contains(err.Error(), "can't parse entities") || strings.Contains(err.Error(), "bad description") {
			log.Printf("⚠️ [GoBale Fallback] Markdown parsing failed due to syntax error. Falling back to PLAIN TEXT for safe delivery...\n")
			req.ParseMode = ""
			err = c.Bot.ExecuteWithContext(activeCtx, req, &result)
		}
	}

	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &result, nil
}

func (c *Context) Text(text string, opts ...Option) {
	_, err := c.Send(text, opts...)
	if err != nil && c.Bot.OnError != nil {
		c.Bot.OnError(err, c)
	}
}

func (c *Context) Reply(text string, opts ...Option) {
	c.Text(text, append(opts, WithReply())...)
}

func (c *Context) Args() []string {
	if c.Message == nil || c.Message.Text == "" {
		return nil
	}
	parts := strings.Fields(c.Message.Text)
	if len(parts) <= 1 {
		return nil
	}
	return parts[1:]
}

func (c *Context) MessageText() string {
	if c.Message != nil {
		return c.Message.Text
	}
	return ""
}

func (c *Context) Delete() error {
	if c.err != nil {
		return c.err
	}
	if c.Message == nil {
		return errors.New("no message context to delete")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return err
	}
	req := methods.DeleteMessage{ChatID: chatID, MessageID: c.Message.MessageID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, nil)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return err
	}
	return err
}

func (c *Context) DeleteReply() error {
	if c.err != nil {
		return c.err
	}
	if c.Message == nil || c.Message.ReplyToMessage == nil {
		return errors.New("no replied message found to delete")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return err
	}
	req := methods.DeleteMessage{ChatID: chatID, MessageID: c.Message.ReplyToMessage.MessageID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, nil)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return err
	}
	return err
}

func (c *Context) IsPhoto() bool {
	return c.Message != nil && len(c.Message.Photo) > 0
}

func (c *Context) IsDocument() bool {
	return c.Message != nil && c.Message.Document != nil
}

func (c *Context) IsVoice() bool {
	return c.Message != nil && c.Message.Voice != nil
}

func (c *Context) IsLocation() bool {
	return c.Message != nil && c.Message.Location != nil
}

func (c *Context) IsContact() bool {
	return c.Message != nil && c.Message.Contact != nil
}

func (c *Context) IsInvoice() bool {
	return c.Message != nil && c.Message.Invoice != nil
}

func (c *Context) IsAnimation() bool {
	return c.Message != nil && c.Message.Animation != nil
}

func (c *Context) IsPayment() bool {
	return c.Message != nil && c.Message.SuccessfulPayment != nil
}

func (c *Context) NewMembers() []models.User {
	if c.Message != nil {
		return c.Message.NewChatMembers
	}
	return nil
}

func (c *Context) GetMe() (*models.User, error) {
	if c.err != nil {
		return nil, c.err
	}
	var user models.User
	err := c.Bot.ExecuteWithContext(c.activeCtx(), methods.GetMe{}, &user)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &user, err
}

func (c *Context) GetChat(chatID int64) (*models.ChatFullInfo, error) {
	if c.err != nil {
		return nil, c.err
	}
	var chat models.ChatFullInfo
	req := methods.GetChat{ChatID: chatID}
	err := c.Bot.ExecuteWithContext(c.activeCtx(), req, &chat)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &chat, nil
}

func (c *Context) Answer(text string, showAlert bool) error {
	if c.err != nil {
		return c.err
	}
	if c.Update == nil || c.Update.CallbackQuery == nil {
		return errors.New("no callback query found in this context")
	}
	queryID := c.Update.CallbackQuery.ID
	if strings.HasPrefix(queryID, "1") && text != "" {
		_ = c.Bot.ExecuteWithContext(c.activeCtx(), methods.AnswerCallbackQuery{CallbackQueryID: queryID}, nil)
		_, err := c.Send(text, WithReply())
		return err
	}
	req := methods.AnswerCallbackQuery{CallbackQueryID: queryID, Text: text, ShowAlert: showAlert}
	err := c.Bot.ExecuteWithContext(c.activeCtx(), req, nil)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return err
	}
	return err
}

func (c *Context) IsOldCallbackClient() bool {
	if c.Update == nil || c.Update.CallbackQuery == nil {
		return false
	}
	return strings.HasPrefix(c.Update.CallbackQuery.ID, "1")
}

func (c *Context) SendPhoto(photo any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendPhoto{
		ChatID:           chatID,
		FromChatID:       config.FromChatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch p := photo.(type) {
	case string:
		if isLocalFile(p) {
			if cached, ok := c.Bot.Client.fileCache.Load(p); ok {
				req.Photo = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(p)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(p),
					Reader:   file,
					Field:    "photo",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendPhoto", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && len(msg.Photo) > 0 {
					bestPhoto := msg.Photo[len(msg.Photo)-1]
					c.Bot.Client.fileCache.Store(p, bestPhoto.FileID)
				}
			}
		} else {
			req.Photo = p
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}

	case models.InputFile:
		p.Field = "photo"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendPhoto", req, []models.InputFile{p}, &msg)
	default:
		errSend = errors.New("invalid photo type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendAudio(audio any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendAudio{
		ChatID:           chatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch a := audio.(type) {
	case string:
		if isLocalFile(a) {
			if cached, ok := c.Bot.Client.fileCache.Load(a); ok {
				req.Audio = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(a)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(a),
					Reader:   file,
					Field:    "audio",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendAudio", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && msg.Audio != nil {
					c.Bot.Client.fileCache.Store(a, msg.Audio.FileID)
				}
			}
		} else {
			req.Audio = a
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}
	case models.InputFile:
		a.Field = "audio"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendAudio", req, []models.InputFile{a}, &msg)
	default:
		errSend = errors.New("invalid audio type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendDocument(document any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendDocument{
		ChatID:           chatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch d := document.(type) {
	case string:
		if isLocalFile(d) {
			if cached, ok := c.Bot.Client.fileCache.Load(d); ok {
				req.Document = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(d)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(d),
					Reader:   file,
					Field:    "document",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendDocument", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && msg.Document != nil {
					c.Bot.Client.fileCache.Store(d, msg.Document.FileID)
				}
			}
		} else {
			req.Document = d
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}
	case models.InputFile:
		d.Field = "document"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendDocument", req, []models.InputFile{d}, &msg)
	default:
		errSend = errors.New("invalid document type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendVideo(video any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendVideo{
		ChatID:           chatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch v := video.(type) {
	case string:
		if isLocalFile(v) {
			if cached, ok := c.Bot.Client.fileCache.Load(v); ok {
				req.Video = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(v)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(v),
					Reader:   file,
					Field:    "video",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendVideo", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && msg.Video != nil {
					c.Bot.Client.fileCache.Store(v, msg.Video.FileID)
				}
			}
		} else {
			req.Video = v
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}
	case models.InputFile:
		v.Field = "video"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendVideo", req, []models.InputFile{v}, &msg)
	default:
		errSend = errors.New("invalid video type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendAnimation(animation any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendAnimation{
		ChatID:           chatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch a := animation.(type) {
	case string:
		if isLocalFile(a) {
			if cached, ok := c.Bot.Client.fileCache.Load(a); ok {
				req.Animation = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(a)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(a),
					Reader:   file,
					Field:    "animation",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendAnimation", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && msg.Animation != nil {
					c.Bot.Client.fileCache.Store(a, msg.Animation.FileID)
				}
			}
		} else {
			req.Animation = a
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}
	case models.InputFile:
		a.Field = "animation"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendAnimation", req, []models.InputFile{a}, &msg)
	default:
		errSend = errors.New("invalid animation type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendVoice(voice any, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendVoice{
		ChatID:           chatID,
		Caption:          config.Caption,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	var errSend error

	switch v := voice.(type) {
	case string:
		if isLocalFile(v) {
			if cached, ok := c.Bot.Client.fileCache.Load(v); ok {
				req.Voice = cached.(string)
				errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
			} else {
				file, err := os.Open(v)
				if err != nil {
					c.err = err
					return nil, err
				}
				defer file.Close()

				inputFile := models.InputFile{
					FileName: filepath.Base(v),
					Reader:   file,
					Field:    "voice",
				}

				errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendVoice", req, []models.InputFile{inputFile}, &msg)
				if errSend == nil && msg.Voice != nil {
					c.Bot.Client.fileCache.Store(v, msg.Voice.FileID)
				}
			}
		} else {
			req.Voice = v
			errSend = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
		}
	case models.InputFile:
		v.Field = "voice"
		errSend = c.Bot.BaseRequestMultipart(c.activeCtx(), "sendVoice", req, []models.InputFile{v}, &msg)
	default:
		errSend = errors.New("invalid voice type")
	}

	if errSend != nil {
		c.err = errSend
		if c.Bot.OnError != nil {
			c.Bot.OnError(errSend, c)
		}
		return nil, errSend
	}
	return &msg, nil
}

func (c *Context) SendLocation(latitude, longitude float64, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendLocation{
		ChatID:             chatID,
		Latitude:           latitude,
		Longitude:          longitude,
		HorizontalAccuracy: config.HorizontalAccuracy,
		ReplyToMessageID:   config.ReplyToMessageID,
		ReplyMarkup:        config.ReplyMarkup,
	}

	var msg models.Message
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &msg, err
}

func (c *Context) SendContact(phoneNumber any, firstName string, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	req := methods.SendContact{
		ChatID:           chatID,
		PhoneNumber:      phoneNumber,
		FirstName:        firstName,
		LastName:         config.LastName,
		ReplyToMessageID: config.ReplyToMessageID,
		ReplyMarkup:      config.ReplyMarkup,
	}

	var msg models.Message
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &msg)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &msg, err
}

func (c *Context) SendChatAction(action string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}

	resolvedID := c.Bot.ResolveChatID(chatID)

	req := methods.SendChatAction{
		ChatID: fmt.Sprintf("%v", resolvedID),
		Action: action,
	}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) LeaveChat() (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.LeaveChat{ChatID: chatID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) GetChatAdministrators() ([]models.ChatMember, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	var admins []models.ChatMember
	req := methods.GetChatAdministrators{ChatID: chatID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &admins)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return admins, nil
}

func (c *Context) GetChatMembersCount() (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return 0, err
	}
	var count int
	req := methods.GetChatMembersCount{ChatID: chatID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &count)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return 0, err
	}
	return count, nil
}

func (c *Context) GetChatMember(userID int64) (*models.ChatMember, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	var member models.ChatMember
	req := methods.GetChatMember{ChatID: chatID, UserID: userID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &member)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &member, nil
}

func (c *Context) IsAdmin() bool {
	if c.Message == nil || c.Message.From == nil {
		return false
	}
	if c.Message.Chat.Type == "private" {
		return true
	}
	member, err := c.GetChatMember(c.Message.From.ID)
	if err != nil {
		return false
	}
	return member.Status == "administrator" || member.Status == "creator"
}

func (c *Context) SetChatTitle(title string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.SetChatTitle{ChatID: chatID, Title: title}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) SetChatDescription(description string) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.SetChatDescription{ChatID: chatID, Description: description}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) DeleteChatPhoto() (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		return false, err
	}
	req := methods.DeleteChatPhoto{ChatID: chatID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) SetChatPhoto(photo models.InputFile) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.SetChatPhoto{ChatID: chatID}
	photo.Field = "photo"
	var ok bool
	err = c.Bot.BaseRequestMultipart(c.activeCtx(), "setChatPhoto", req, []models.InputFile{photo}, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) CreateChatInviteLink() (*models.ChatInviteLink, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	var link models.ChatInviteLink
	req := methods.CreateChatInviteLink{ChatID: chatID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &link)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &link, nil
}

func (c *Context) RevokeChatInviteLink(inviteLink string) (*models.ChatInviteLink, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	var link models.ChatInviteLink
	req := methods.RevokeChatInviteLink{ChatID: chatID, InviteLink: inviteLink}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &link)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &link, nil
}

func (c *Context) ExportChatInviteLink() (string, error) {
	if c.err != nil {
		return "", c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return "", err
	}
	var link string
	req := methods.ExportChatInviteLink{ChatID: chatID}
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &link)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return "", err
	}
	return link, nil
}

func (c *Context) GetFile(fileID string) (*models.File, error) {
	if c.err != nil {
		return nil, c.err
	}
	var file models.File
	req := methods.GetFile{FileID: fileID}
	err := c.Bot.ExecuteWithContext(c.activeCtx(), req, &file)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &file, nil
}

func (c *Context) GetDownloadURL(file *models.File) string {
	return c.Bot.GetDownloadURL(file.FilePath)
}

func (c *Context) SendAlbum(media []any, opts ...Option) ([]models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}

	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	if config.ReplyToMessageID == -1 && c.Message != nil {
		config.ReplyToMessageID = c.Message.MessageID
	}

	var finalOpts []Option
	if config.ReplyToMessageID > 0 {
		finalOpts = append(finalOpts, WithReplyTo(config.ReplyToMessageID))
	}

	result, err := c.Bot.SendMediaGroup(chatID, media, finalOpts...)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return result, nil
}

func (c *Context) Pin(messageID int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.PinChatMessage{ChatID: chatID, MessageID: messageID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) PinCurrent() (bool, error) {
	if c.Message == nil {
		return false, errors.New("no message in context to pin")
	}
	return c.Pin(c.Message.MessageID)
}

func (c *Context) PinReply() (bool, error) {
	if c.Message == nil || c.Message.ReplyToMessage == nil {
		return false, errors.New("no replied message found to pin")
	}
	return c.Pin(c.Message.ReplyToMessage.MessageID)
}

func (c *Context) Unpin(messageID int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.UnPinChatMessage{ChatID: chatID, MessageID: messageID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) UnpinReply() (bool, error) {
	if c.Message == nil || c.Message.ReplyToMessage == nil {
		return false, errors.New("no replied message found to unpin")
	}
	return c.Unpin(c.Message.ReplyToMessage.MessageID)
}

func (c *Context) UnpinAll() (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.UnpinAllChatMessages{ChatID: chatID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) AskReview(delaySeconds int) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	var userID int64
	if c.Message != nil && c.Message.From != nil {
		userID = c.Message.From.ID
	} else if c.Update.CallbackQuery != nil {
		userID = c.Update.CallbackQuery.From.ID
	} else {
		return false, errors.New("could not find a valid user ID to ask review")
	}
	req := methods.AskReview{UserID: userID, DelaySeconds: delaySeconds}
	var ok bool
	err := c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) Ban(userID int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.BanChatMember{ChatID: chatID, UserID: userID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) Unban(userID int64, opts ...Option) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	req := methods.UnbanChatMember{ChatID: chatID, UserID: userID, OnlyIfBanned: config.OnlyIfBanned}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) BanReply() (bool, error) {
	if c.Message == nil || c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
		return false, errors.New("no replied user found to ban")
	}
	return c.Ban(c.Message.ReplyToMessage.From.ID)
}

func (c *Context) Promote(userID int64, opts ...Option) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	req := methods.PromoteChatMember{
		ChatID:              chatID,
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
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) Demote(userID int64) (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return false, err
	}
	req := methods.PromoteChatMember{ChatID: chatID, UserID: userID}
	var ok bool
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &ok)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return false, err
	}
	return ok, nil
}

func (c *Context) PromoteReply(opts ...Option) (bool, error) {
	if c.Message == nil || c.Message.ReplyToMessage == nil || c.Message.ReplyToMessage.From == nil {
		return false, errors.New("no replied user found to promote")
	}
	return c.Promote(c.Message.ReplyToMessage.From.ID, opts...)
}

func (c *Context) SendInvoice(title, description, payload, providerToken string, prices []models.LabeledPrice, opts ...Option) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		c.err = err
		return nil, err
	}
	config := &SendOptions{}
	for _, opt := range opts {
		opt(config)
	}
	req := methods.SendInvoice{
		ChatID:        chatID,
		Title:         title,
		Description:   description,
		Payload:       payload,
		ProviderToken: providerToken,
		Currency:      "IRR",
		Prices:        prices,
	}
	var result models.Message
	err = c.Bot.ExecuteWithContext(c.activeCtx(), req, &result)
	if err != nil {
		c.err = err
		if c.Bot.OnError != nil {
			c.Bot.OnError(err, c)
		}
		return nil, err
	}
	return &result, err
}

func (c *Context) AnswerPreCheckout(ok bool, errorMessage string) (bool, error) {
	if c.Update == nil || c.Update.PreCheckoutQuery == nil {
		return false, errors.New("no pre_checkout_query found in this context")
	}
	req := methods.AnswerPreCheckoutQuery{
		PreCheckoutQueryID: c.Update.PreCheckoutQuery.ID,
		OK:                 ok,
		ErrorMessage:       errorMessage,
	}
	var result bool
	err := c.Bot.ExecuteWithContext(c.activeCtx(), req, &result)
	if err != nil && c.Bot.OnError != nil {
		c.Bot.OnError(err, c)
	}
	return result, err
}

func (c *Context) Session() *session.Session {
	chatID, _ := c.DetermineChatID()
	return c.Bot.Sessions.Get(chatID)
}

func (c *Context) SetState(state string) {
	c.Session().SetState(state)
}

func (c *Context) GetState() string {
	return c.Session().GetState()
}

func (c *Context) SetData(key string, val any) {
	c.Session().SetData(key, val)
}

func (c *Context) GetData(key string) (any, bool) {
	return c.Session().GetData(key)
}

func (c *Context) ClearSession() {
	chatID, _ := c.DetermineChatID()
	c.Bot.Sessions.Clear(chatID)
}

func (c *Context) Err() error {
	return c.err
}

func (c *Context) ClearErr() {
	c.err = nil
}

func (c *Context) MustJoin(channelUsername string, alertText string) bool {
	if c.Message == nil || c.Message.From == nil {
		return false
	}
	userID := c.Message.From.ID
	member, err := c.Bot.GetChatMember(channelUsername, userID)
	if err != nil || member.Status == "left" || member.Status == "kicked" {
		c.Reply(alertText)
		return false
	}
	return true
}

func (c *Context) PayRial(title string, description string, amount int64, payload string, providerToken string) (*models.Message, error) {
	prices := []models.LabeledPrice{
		{Label: title, Amount: amount},
	}
	return c.SendInvoice(title, description, payload, providerToken, prices)
}

func (c *Context) PinTemporary(duration time.Duration) (bool, error) {
	ok, err := c.PinCurrent()
	if err != nil {
		return false, err
	}
	messageID := c.Message.MessageID
	c.Bot.Schedule(duration, func() {
		c.Unpin(messageID)
	})
	return ok, nil
}

func (c *Context) GetTransaction(transactionID string) (*models.Transaction, error) {
	return c.Bot.GetTransaction(transactionID)
}

func (c *Context) DeepLinkParam() string {
	if c.Message == nil || !strings.HasPrefix(c.Message.Text, "/start ") {
		return ""
	}
	parts := strings.Fields(c.Message.Text)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func (c *Context) Edit(newText string, opts ...Option) (*models.Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message context to edit")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		return nil, err
	}
	msg, err := c.Bot.EditMessageText(chatID, c.Message.MessageID, newText, opts...)
	if err != nil {
		if strings.Contains(err.Error(), "can't parse entities") || strings.Contains(err.Error(), "bad description") {
			log.Printf("⚠️ [GoBale Fallback] Markdown parsing failed in Edit. Falling back to PLAIN TEXT...\n")
			msg, err = c.Bot.EditMessageText(chatID, c.Message.MessageID, newText)
		}
	}
	return msg, err
}

func (c *Context) EditCaption(newCaption string, opts ...Option) (*models.Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message context to edit caption")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		return nil, err
	}
	return c.Bot.EditMessageCaption(chatID, c.Message.MessageID, newCaption, opts...)
}

func (c *Context) EditMarkup(opts ...Option) (*models.Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message context to edit reply markup")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		return nil, err
	}
	return c.Bot.EditMessageReplyMarkup(chatID, c.Message.MessageID, opts...)
}

func (c *Context) ArchiveTo(archiveChatID any) (*models.Message, error) {
	if c.Message == nil {
		return nil, errors.New("no message in context to archive")
	}
	chatID, err := c.DetermineChatID()
	if err != nil {
		return nil, err
	}
	return c.Bot.ForwardMessage(c.activeCtx(), archiveChatID, chatID, c.Message.MessageID)
}

func (c *Context) EditToPage(text string, items []models.InlineKeyboardButton, page, itemsPerPage int, callbackPrefix string) (*models.Message, error) {
	markup := models.NewPaginatedKeyboard(items, page, itemsPerPage, callbackPrefix)
	return c.Edit(text, WithKeyboard(markup))
}

func (c *Context) CallbackArgs() []string {
	if c.Update == nil || c.Update.CallbackQuery == nil {
		return nil
	}
	parts := strings.Split(c.Update.CallbackQuery.Data, ":")
	if len(parts) <= 1 {
		return nil
	}
	return parts[1:]
}

func (c *Context) activeCtx() context.Context {
	if c.Logger {
		return context.WithValue(c.ctx, loggerKey, true)
	}
	return c.ctx
}

func (c *Context) DownloadFile(fileID string, destDir string) (string, error) {
	if c.err != nil {
		return "", c.err
	}

	fileInfo, err := c.GetFile(fileID)
	if err != nil {
		c.err = err
		return "", err
	}

	if fileInfo.FilePath == "" {
		err = errors.New("file path is empty in API response")
		c.err = err
		return "", err
	}

	downloadURL := c.GetDownloadURL(fileInfo)

	req, err := http.NewRequestWithContext(c.activeCtx(), http.MethodGet, downloadURL, nil)
	if err != nil {
		c.err = err
		return "", err
	}

	resp, err := c.Bot.Client.httpClient.Do(req)
	if err != nil {
		c.err = err
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("download failed with HTTP status: %d", resp.StatusCode)
		c.err = err
		return "", err
	}

	fileName := filepath.Base(fileInfo.FilePath)
	replacer := strings.NewReplacer(
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	fileName = replacer.Replace(fileName)

	disposition := resp.Header.Get("Content-Disposition")
	if disposition != "" {
		_, params, err := mime.ParseMediaType(disposition)
		if err == nil {
			if originalName, ok := params["filename"]; ok && originalName != "" {
				fileName = replacer.Replace(originalName)
			}
		}
	}

	if !strings.Contains(fileName, ".") {
		contentType := resp.Header.Get("Content-Type")
		if contentType != "" {
			mediaType, _, _ := mime.ParseMediaType(contentType)
			exts, _ := mime.ExtensionsByType(mediaType)
			if len(exts) > 0 {
				fileName = fileName + exts[0]
			}
		}
	}

	destPath := filepath.Join(destDir, fileName)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		c.err = err
		return "", err
	}

	out, err := os.Create(destPath)
	if err != nil {
		c.err = err
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		c.err = err
		return "", err
	}

	return destPath, nil
}

func (c *Context) SendTemp(text string, duration time.Duration, opts ...Option) (*models.Message, error) {
	msg, err := c.Send(text, opts...)
	if err != nil {
		return nil, err
	}

	c.Bot.Schedule(duration, func() {
		_, _ = c.Bot.DeleteMessage(msg.Chat.ID, msg.MessageID)
	})

	return msg, nil
}

func (c *Context) IsMember(channelID any) (bool, error) {
	if c.Message == nil || c.Message.From == nil {
		return false, errors.New("no user context to check membership")
	}

	resolvedID := c.Bot.ResolveChatID(channelID)
	member, err := c.Bot.GetChatMember(resolvedID, c.Message.From.ID)
	if err != nil {
		return false, err
	}

	isMember := member.Status == "member" || member.Status == "administrator" || member.Status == "creator"
	return isMember, nil
}

func (c *Context) SendProgress(title string, steps []string, delay time.Duration) (*models.Message, error) {
	if len(steps) == 0 {
		return nil, errors.New("no steps provided")
	}

	msg, err := c.Send(fmt.Sprintf("%s\n\n⏳ %s", title, steps[0]))
	if err != nil {
		return nil, err
	}

	go func() {
		for i := 1; i < len(steps); i++ {
			time.Sleep(delay)
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%s\n\n", title))
			for j := 0; j < i; j++ {
				sb.WriteString(fmt.Sprintf("✅ %s\n", steps[j]))
			}
			sb.WriteString(fmt.Sprintf("⏳ %s", steps[i]))
			_, _ = c.Bot.EditMessageText(msg.Chat.ID, msg.MessageID, sb.String())
		}

		time.Sleep(delay)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s\n\n", title))
		for _, step := range steps {
			sb.WriteString(fmt.Sprintf("✅ %s\n", step))
		}
		sb.WriteString("\n🎉 فرآیند با موفقیت به پایان رسید!")
		_, _ = c.Bot.EditMessageText(msg.Chat.ID, msg.MessageID, sb.String())
	}()

	return msg, nil
}

func (c *Context) Typing() {
	_, _ = c.SendChatAction("typing")
}

func (c *Context) UploadingPhoto() {
	_, _ = c.SendChatAction("upload_photo")
}

func (c *Context) UploadingDocument() {
	_, _ = c.SendChatAction("upload_document")
}

func (c *Context) Confirm(text string, yesCallbackData, noCallbackData string) (*models.Message, error) {
	markup := models.InlineMarkup().
		Row(
			models.Btn("✅ بله، مطمئنم").Callback(yesCallbackData),
			models.Btn("❌ خیر، لغو شود").Callback(noCallbackData),
		).
		Build()
	return c.Send(text, WithKeyboard(markup))
}

func (c *Context) T(key string) string {
	if c.Bot.i18n == nil || c.Message == nil || c.Message.From == nil {
		return key
	}

	lang := c.Message.From.LanguageCode
	if lang == "" {
		lang = "fa"
	}

	if langDict, exists := c.Bot.i18n[lang]; exists {
		if val, ok := langDict[key]; ok {
			return val
		}
	}

	if defaultDict, exists := c.Bot.i18n["fa"]; exists {
		if val, ok := defaultDict[key]; ok {
			return val
		}
	}

	return key
}

func (c *Context) SendSubMenu(text string, rows [][]models.InlineKeyboardButton, backCallbackData string) (*models.Message, error) {
	var finalRows [][]models.InlineKeyboardButton
	finalRows = append(finalRows, rows...)

	if backCallbackData != "" {
		backBtn := models.NewInlineKeyboardButtonData("⬅️ بازگشت به منوی اصلی", backCallbackData)
		finalRows = append(finalRows, []models.InlineKeyboardButton{backBtn})
	}

	markup := models.NewInlineKeyboardMarkup(finalRows...)
	return c.Send(text, WithKeyboard(markup))
}

func (c *Context) SendToggle(text string, label string, isEnabled bool, callbackData string) (*models.Message, error) {
	statusIcon := "🔴 خاموش"
	if isEnabled {
		statusIcon = "🟢 روشن"
	}
	markup := models.InlineMarkup().
		Row(
			models.Btn(fmt.Sprintf("%s: %s", label, statusIcon)).Callback(callbackData),
		).
		Build()
	return c.Send(text, WithKeyboard(markup))
}

func (c *Context) SendCleanMenu(text string, markup any) (*models.Message, error) {
	if lastIDVal, ok := c.GetData("last_menu_id"); ok {
		if lastID, ok := lastIDVal.(int64); ok && lastID > 0 {
			chatID, _ := c.DetermineChatID()
			_, _ = c.Bot.DeleteMessage(chatID, lastID)
		}
	}

	msg, err := c.Send(text, WithKeyboard(markup))
	if err != nil {
		return nil, err
	}

	c.SetData("last_menu_id", msg.MessageID)
	return msg, nil
}

func (c *Context) NavigateMenu(text string, markup any) (*models.Message, error) {
	if c.err != nil {
		return nil, c.err
	}

	if c.Update != nil && c.Update.CallbackQuery != nil && c.Update.CallbackQuery.Message != nil {
		msgID := c.Update.CallbackQuery.Message.MessageID
		c.SetData("last_menu_id", msgID)

		return c.Edit(text, WithKeyboard(markup))
	}

	return c.SendCleanMenu(text, markup)
}

var autoCallbackCounter uint64

func (c *Context) Btn(text string, handler HandlerFunc) models.InlineKeyboardButton {
	id := atomic.AddUint64(&autoCallbackCounter, 1)
	uniqueID := fmt.Sprintf("auto_cb_%d", id)
	c.Bot.OnCallbackData(uniqueID, handler)
	return models.NewInlineKeyboardButtonData(text, uniqueID)
}

var autoStateCounter uint64

func (c *Context) Ask(text string, handler HandlerFunc) (*models.Message, error) {
	id := atomic.AddUint64(&autoStateCounter, 1)
	uniqueState := fmt.Sprintf("auto_state_%d", id)
	c.Bot.OnState(uniqueState, func(c *Context) {
		c.SetState("")
		c.Bot.RemoveState(uniqueState)
		handler(c)
	})

	c.SetState(uniqueState)
	return c.Send(text)
}

var autoPageCounter uint64

type WizardStep struct {
	Prompt     string
	Validation func(string) bool
	OnError    string
	Handler    func(*Context)
}

type Wizard struct {
	ctx        *Context
	steps      []WizardStep
	onComplete func(*Context)
}

func (c *Context) NewWizard() *Wizard {
	return &Wizard{ctx: c}
}

func (w *Wizard) Step(prompt string, handler func(*Context)) *Wizard {
	w.steps = append(w.steps, WizardStep{Prompt: prompt, Handler: handler})
	return w
}

func (w *Wizard) StepWithValidation(prompt string, validator func(string) bool, onError string, handler func(*Context)) *Wizard {
	w.steps = append(w.steps, WizardStep{Prompt: prompt, Validation: validator, OnError: onError, Handler: handler})
	return w
}

func (w *Wizard) OnComplete(handler func(*Context)) *Wizard {
	w.onComplete = handler
	return w
}

func (w *Wizard) Run() {
	w.runStep(0)
}

func (w *Wizard) runStep(index int) {
	if index >= len(w.steps) {
		if w.onComplete != nil {
			w.onComplete(w.ctx)
		}
		return
	}

	step := w.steps[index]

	_, _ = w.ctx.Ask(step.Prompt, func(c *Context) {
		if step.Validation != nil && !step.Validation(c.MessageText()) {
			if step.OnError != "" {
				c.Reply(step.OnError)
			} else {
				c.Reply("⚠️ مقدار وارد شده نامعتبر است. لطفاً مجدداً ارسال کنید.")
			}
			w.runStep(index)
			return
		}

		step.Handler(c)

		w.runStep(index + 1)
	})
}

func (r *PaginationRegistry) Set(menuID string, provider func(page int, limit int) ([]models.InlineKeyboardButton, int, error), limit int, ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[menuID] = &paginatedCacheItem{
		provider:  provider,
		limit:     limit,
		expiresAt: time.Now().Add(ttl),
	}
}

func (r *PaginationRegistry) Get(menuID string) (func(page int, limit int) ([]models.InlineKeyboardButton, int, error), int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[menuID]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, 0, false
	}
	return item.provider, item.limit, true
}

func (r *PaginationRegistry) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, v := range r.items {
		if now.After(v.expiresAt) {
			delete(r.items, k)
		}
	}
}

var onceRegisterPagination sync.Once

func (c *Context) registerGlobalPagination() {
	onceRegisterPagination.Do(func() {
		c.Bot.OnCallbackData("_sys_page", func(c *Context) {
			args := c.CallbackArgs()
			if len(args) < 2 {
				_ = c.Answer("⚠️ ساختار صفحه‌بندی نامعتبر است.", true)
				return
			}

			menuID := args[0]
			page, err := strconv.Atoi(args[1])
			if err != nil || page < 1 {
				_ = c.Answer("⚠️ شماره صفحه نامعتبر است.", true)
				return
			}

			provider, limit, exists := globalPaginationRegistry.Get(menuID)
			if !exists {
				_ = c.Answer("⚠️ این منو منقضی شده است. لطفا مجدداً منو را باز کنید.", true)
				return
			}

			buttons, totalCount, err := provider(page, limit)
			if err != nil {
				_ = c.Answer("⚠️ خطا در بارگذاری داده‌ها", true)
				return
			}

			var rows [][]models.InlineKeyboardButton
			for _, btn := range buttons {
				rows = append(rows, []models.InlineKeyboardButton{btn})
			}

			var navRow []models.InlineKeyboardButton
			totalPages := (totalCount + limit - 1) / limit

			if page > 1 {
				navRow = append(navRow, models.NewInlineKeyboardButtonData("⬅️ قبلی", fmt.Sprintf("_sys_page:%s:%d", menuID, page-1)))
			}
			if page < totalPages {
				navRow = append(navRow, models.NewInlineKeyboardButtonData("بعدی ➡️", fmt.Sprintf("_sys_page:%s:%d", menuID, page+1)))
			}
			if len(navRow) > 0 {
				rows = append(rows, navRow)
			}

			markup := &models.InlineKeyboardMarkup{InlineKeyboard: rows}
			_, _ = c.Edit(c.MessageText(), WithKeyboard(markup))
			_ = c.Answer("", false)
		})

		go func() {
			ticker := time.NewTicker(3 * time.Minute)
			for range ticker.C {
				globalPaginationRegistry.Cleanup()
			}
		}()
	})
}

func (c *Context) SendPaginated(title string, items []models.InlineKeyboardButton, itemsPerPage int) (*models.Message, error) {
	c.registerGlobalPagination()

	id := atomic.AddUint64(&autoPageCounter, 1)
	menuID := fmt.Sprintf("p_%d", id)

	provider := func(page int, limit int) ([]models.InlineKeyboardButton, int, error) {
		start := (page - 1) * limit
		end := start + limit
		if start > len(items) {
			return nil, len(items), nil
		}
		if end > len(items) {
			end = len(items)
		}
		return items[start:end], len(items), nil
	}

	globalPaginationRegistry.Set(menuID, provider, itemsPerPage, 10*time.Minute)

	buttons, _, _ := provider(1, itemsPerPage)
	var rows [][]models.InlineKeyboardButton
	for _, btn := range buttons {
		rows = append(rows, []models.InlineKeyboardButton{btn})
	}

	var navRow []models.InlineKeyboardButton
	totalPages := (len(items) + itemsPerPage - 1) / itemsPerPage
	if totalPages > 1 {
		navRow = append(navRow, models.NewInlineKeyboardButtonData("بعدی ➡️", fmt.Sprintf("_sys_page:%s:2", menuID)))
		rows = append(rows, navRow)
	}

	markup := &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	return c.Send(title, WithKeyboard(markup))
}

func (c *Context) SendPaginatedDynamic(title string, itemsPerPage int, provider func(page int, limit int) ([]models.InlineKeyboardButton, int, error)) (*models.Message, error) {
	c.registerGlobalPagination()

	id := atomic.AddUint64(&autoPageCounter, 1)
	menuID := fmt.Sprintf("pd_%d", id)

	globalPaginationRegistry.Set(menuID, provider, itemsPerPage, 15*time.Minute)

	buttons, totalCount, err := provider(1, itemsPerPage)
	if err != nil {
		return nil, err
	}

	var rows [][]models.InlineKeyboardButton
	for _, btn := range buttons {
		rows = append(rows, []models.InlineKeyboardButton{btn})
	}

	var navRow []models.InlineKeyboardButton
	totalPages := (totalCount + itemsPerPage - 1) / itemsPerPage
	if totalPages > 1 {
		navRow = append(navRow, models.NewInlineKeyboardButtonData("بعدی ➡️", fmt.Sprintf("_sys_page:%s:2", menuID)))
		rows = append(rows, navRow)
	}

	markup := &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	return c.Send(title, WithKeyboard(markup))
}

func (c *Context) ReplyWithKeyboard(text string, buttons []string, cols int) (*models.Message, error) {
	markup := models.NewReplyKeyboardMarkupFromSlice(buttons, cols)
	return c.Send(text, WithReply(), WithKeyboard(markup))
}

func (c *Context) ReplyWithInline(text string, buttons []string, cols int, callbackPrefix string) (*models.Message, error) {
	markup := models.NewInlineKeyboardMarkupFromSlice(buttons, cols, callbackPrefix)
	return c.Send(text, WithReply(), WithKeyboard(markup))
}

func parseDurationWithDays(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}

	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if strings.HasSuffix(s, "w") {
		weeksStr := strings.TrimSuffix(s, "w")
		weeks, err := strconv.Atoi(weeksStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}

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
				return fmt.Errorf("argument %d is not a valid integer: %w", i+1, err)
			}
			*ptr = val
		case *int64:
			val, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return fmt.Errorf("argument %d is not a valid int64: %w", i+1, err)
			}
			*ptr = val
		case *float64:
			val, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				return fmt.Errorf("argument %d is not a valid float64: %w", i+1, err)
			}
			*ptr = val
		case *bool:
			val, err := strconv.ParseBool(arg)
			if err != nil {
				return fmt.Errorf("argument %d is not a valid boolean: %w", i+1, err)
			}
			*ptr = val
		case *time.Duration:
			val, err := parseDurationWithDays(arg)
			if err != nil {
				return fmt.Errorf("argument %d is not a valid duration: %w", i+1, err)
			}
			*ptr = val
		default:
			return fmt.Errorf("unsupported target type for argument %d: %T", i+1, target)
		}
	}

	return nil
}

func (c *Context) ScanArgs(targets ...any) error {
	return ScanValues(c.Args(), " ", targets...)
}

func (c *Context) ScanCallbackArgs(targets ...any) error {
	return ScanValues(c.CallbackArgs(), ":", targets...)
}

func (c *Context) ReplyMemoryStats() (*models.Message, error) {
	stats := c.Bot.GetMemoryStats()

	limitStr := "تعریف نشده (نامحدود)"
	if stats.MemoryLimitBytes != math.MaxInt64 && stats.MemoryLimitBytes > 0 {
		limitStr = fmt.Sprintf("%.2f مگابایت", float64(stats.MemoryLimitBytes)/(1024*1024))
	}

	report := fmt.Sprintf(`🖥️ %s

🔸 رم در حال استفاده (Heap): %.2f مگابایت
🔹 رم رزرو شده از سیستم (Sys): %.2f مگابایت
🔸 دفعات اجرای زباله‌روب (NumGC): %d بار
🔹 حد مجاز تعیین‌شده رم ربات: %s`,
		Bold("گزارش مصرف حافظه رم سرور"),
		stats.AllocMegabytes,
		stats.SysMegabytes,
		stats.NumGC,
		limitStr,
	)

	return c.Send(report, WithMarkdown())
}

func (c *Context) Go(task func()) {
	go func() {
		c.Bot.bgSemaphore <- struct{}{}
		defer func() {
			<-c.Bot.bgSemaphore
			if r := recover(); r != nil {
				if c.Bot.OnError != nil {
					c.Bot.OnError(fmt.Errorf("panic in background task: %v", r), c)
				}
			}
		}()
		task()
	}()
}

func (c *Context) EditToggle(text string, label string, isEnabled bool, callbackData string) (*models.Message, error) {
	statusIcon := "🔴 خاموش"
	if isEnabled {
		statusIcon = "🟢 روشن"
	}

	markup := models.InlineMarkup().
		Row(
			models.Btn(fmt.Sprintf("%s: %s", label, statusIcon)).Callback(callbackData),
		).
		Build()

	return c.Edit(text, WithKeyboard(markup))
}

func (c *Context) SendSettingsMenu(text string, adminID ...any) (*models.Message, error) {
	if len(adminID) > 0 {
		switch val := adminID[0].(type) {
		case int64:
			c.Bot.MaintenanceAdminID = val
		case int:
			c.Bot.MaintenanceAdminID = int64(val)
		case string:
			if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
				c.Bot.MaintenanceAdminID = parsed
			}
		}
	}

	builder := models.InlineMarkup()
	for _, entry := range c.Bot.settings {
		statusIcon := "🔴 خاموش"
		if *entry.Ptr {
			statusIcon = "🟢 روشن"
		}
		builder.Row(models.Btn(fmt.Sprintf("%s: %s", entry.Label, statusIcon)).Callback(fmt.Sprintf("_sys_cfg:%s", entry.Key)))
	}
	return c.Send(text, WithKeyboard(builder.Build()))
}

func (c *Context) EditSettingsMenu(text string, adminID ...any) (*models.Message, error) {
	if len(adminID) > 0 {
		switch val := adminID[0].(type) {
		case int64:
			c.Bot.MaintenanceAdminID = val
		case int:
			c.Bot.MaintenanceAdminID = int64(val)
		case string:
			if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
				c.Bot.MaintenanceAdminID = parsed
			}
		}
	}

	builder := models.InlineMarkup()
	for _, entry := range c.Bot.settings {
		statusIcon := "🔴 خاموش"
		if *entry.Ptr {
			statusIcon = "🟢 روشن"
		}
		builder.Row(models.Btn(fmt.Sprintf("%s: %s", entry.Label, statusIcon)).Callback(fmt.Sprintf("_sys_cfg:%s", entry.Key)))
	}
	return c.Edit(text, WithKeyboard(builder.Build()))
}

func (c *Context) SenderID() int64 {
	if c.Update == nil {
		return 0
	}
	if c.Update.Message != nil && c.Update.Message.From != nil {
		return c.Update.Message.From.ID
	}
	if c.Update.CallbackQuery != nil {
		return c.Update.CallbackQuery.From.ID
	}
	if c.Update.PreCheckoutQuery != nil {
		return c.Update.PreCheckoutQuery.From.ID
	}
	return 0
}

func (c *Context) Log() *ContextLogger {
	chatID, _ := c.DetermineChatID()
	var prefix string
	if chatID > 0 {
		prefix = fmt.Sprintf("[Chat: %d] ", chatID)
	}
	return &ContextLogger{
		logger: c.Bot.Log,
		prefix: prefix,
	}
}

func (c *Context) IsSuperGroup() bool {
	if c.Message == nil {
		return false
	}
	return c.Message.Chat.Type == "supergroup"
}
