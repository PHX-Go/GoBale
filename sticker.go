package gobale

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// StickerChain handles sticker operations using the unified dot system
type StickerChain struct {
	bot *Bot
	ctx context.Context
}

// Sticker opens the sticker management dot chain from the Bot context
func (b *Bot) Sticker() *StickerChain {
	return &StickerChain{
		bot: b,
		ctx: context.Background(),
	}
}

// Sticker opens the sticker management dot chain from the Handler context
func (c *Ctx) Sticker() *StickerChain {
	return &StickerChain{
		bot: c.Bot,
		ctx: c.ctx,
	}
}

// Upload initiates a sticker file upload chain using polymorphic StickerInput
func (s *StickerChain) Upload(userID int64, sticker StickerInput) *UploadStickerChain {
	return &UploadStickerChain{sc: s, user: userID, stk: sticker}
}

// UploadStickerChain handles fluent sticker file uploads safely
type UploadStickerChain struct {
	sc   *StickerChain
	user int64
	stk  StickerInput
}

// Go executes the sticker file upload with auto error logging and file close safety
func (u *UploadStickerChain) Go() (*File, error) {
	var out File
	var err error

	payload := map[string]any{
		"user_id": u.user,
	}

	// Dynamically determine whether to upload binary multipart data or send raw JSON
	if u.stk.IsLocal() {
		path := u.stk.StickerValue()
		file, errOpen := os.Open(path)
		if errOpen != nil {
			return nil, errOpen
		}
		defer file.Close()

		inputFile := InputFile{
			FileName: filepath.Base(path),
			Reader:   file,
			Field:    "sticker",
		}
		err = u.sc.bot.BaseRequestMultipart(u.sc.ctx, "uploadStickerFile", payload, []InputFile{inputFile}, &out)
	} else {
		payload["sticker"] = u.stk.StickerValue()
		err = u.sc.bot.BaseRequest(u.sc.ctx, "uploadStickerFile", payload, &out)
	}

	if err != nil {
		logErr(u.sc.bot, "[Sticker Upload Error] ", err)
	}
	return &out, err
}

// Create initializes a sticker set creation chain
func (s *StickerChain) Create(userID int64, name, title string) *CreateStickerSetChain {
	return &CreateStickerSetChain{
		sc:    s,
		user:  userID,
		name:  name,
		title: title,
	}
}

// CreateStickerSetChain holds sticker payloads for a new set sequence
type CreateStickerSetChain struct {
	sc       *StickerChain
	user     int64
	name     string
	title    string
	stickers []InputSticker
}

// Add appends sticker item parameters to the creation sequence safely using StickerInput interface
func (c *CreateStickerSetChain) Add(sticker StickerInput, emojis []string) *CreateStickerSetChain {
	c.stickers = append(c.stickers, InputSticker{
		Sticker:   sticker,
		EmojiList: emojis,
	})
	return c
}

// Go executes the sticker set creation process with auto error logging
func (c *CreateStickerSetChain) Go() (bool, error) {
	if len(c.stickers) == 0 {
		return false, errors.New("must add at least one sticker to create set")
	}
	var res bool
	err := c.sc.bot.BaseRequest(c.sc.ctx, "createNewStickerSet", map[string]any{
		"user_id":  c.user,
		"name":     c.name,
		"title":    c.title,
		"stickers": c.stickers,
	}, &res)
	if err != nil {
		logErr(c.sc.bot, "[Sticker Set Create Error] ", err)
	}
	return res, err
}

// Add initiates a sticker addition chain
func (s *StickerChain) Add(userID int64, name string, sticker InputSticker) *AddStickerChain {
	return &AddStickerChain{
		sc:   s,
		user: userID,
		name: name,
		stk:  sticker,
	}
}

// AddStickerChain handles fluent sticker addition to active sets
type AddStickerChain struct {
	sc   *StickerChain
	user int64
	name string
	stk  InputSticker
}

// Go executes the addition of sticker to set with auto error logging
func (a *AddStickerChain) Go() (bool, error) {
	if a.name == "" {
		return false, errors.New("missing sticker set name")
	}
	var res bool
	err := a.sc.bot.BaseRequest(a.sc.ctx, "addStickerToSet", map[string]any{
		"user_id": a.user,
		"name":    a.name,
		"sticker": a.stk,
	}, &res)
	if err != nil {
		logErr(a.sc.bot, "[Sticker Add Error] ", err)
	}
	return res, err
}

// Context registers a custom parent context to control deadlines or cancellation propagation
func (s *StickerChain) Context(ctx context.Context) *StickerChain {
	if ctx != nil {
		s.ctx = ctx
	}
	return s
}

// Del initiates a sticker deletion chain from its set using its file ID
func (s *StickerChain) Del(fileID string) *DelStickerChain {
	return &DelStickerChain{sc: s, id: fileID}
}

// DelStickerChain handles fluent sticker deletion from active sets
type DelStickerChain struct {
	sc *StickerChain
	id string
}

// Go executes the deletion of sticker from set with auto error logging
func (d *DelStickerChain) Go() (bool, error) {
	if d.id == "" {
		return false, errors.New("empty sticker file ID")
	}
	var res bool
	err := d.sc.bot.BaseRequest(d.sc.ctx, "deleteStickerFromSet", map[string]any{
		"sticker": d.id,
	}, &res)
	if err != nil {
		logErr(d.sc.bot, "[Sticker Delete Error] ", err)
	}
	return res, err
}

// Get initiates a sticker set retrieval chain using its unique name
func (s *StickerChain) Get(name string) *StickerGetChain {
	return &StickerGetChain{sc: s, name: name}
}

// StickerGetChain handles fluent sticker set queries ending with terminal Go
type StickerGetChain struct {
	sc   *StickerChain
	name string
}

// Go executes the sticker set query on Bale servers and returns StickerSet
func (sg *StickerGetChain) Go() (*StickerSet, error) {
	if sg.name == "" {
		return nil, errors.New("empty sticker set name")
	}
	var out StickerSet
	err := sg.sc.bot.BaseRequest(sg.sc.ctx, "getStickerSet", map[string]any{
		"name": sg.name,
	}, &out)
	if err != nil {
		logErr(sg.sc.bot, "[Sticker Set Get Error] ", err)
	}
	return &out, err
}
