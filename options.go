package gobale

import (
	"github.com/PHX-Go/GoBale/models"
)

const (
	ActionTyping        = "typing"
	ActionUploadPhoto   = "upload_photo"
	ActionRecordVideo   = "record_video"
	ActionUploadVideo   = "upload_video"
	ActionRecordVoice   = "record_voice"
	ActionUploadVoice   = "upload_voice"
	ActionChooseSticker = "choose_sticker"
	AutoWorkers         = 0
)

type SendOptions struct {
	ReplyToMessageID   int64
	ParseMode          string
	ReplyMarkup        any
	Caption            string
	FromChatID         any
	LastName           string
	HorizontalAccuracy float64
	OnlyIfBanned       bool
	CanChangeInfo       bool
	CanPostMessages     bool
	CanEditMessages     bool
	CanDeleteMessages   bool
	CanManageVideoChats bool
	CanInviteUsers      bool
	CanRestrictMembers  bool
	PhotoURL           string
}

type Option func(*SendOptions)

func WithReply() Option {
	return func(o *SendOptions) {
		o.ReplyToMessageID = -1
	}
}

func WithReplyTo(id int64) Option {
	return func(o *SendOptions) {
		o.ReplyToMessageID = id
	}
}

func WithMarkdown() Option {
	return func(o *SendOptions) {
		o.ParseMode = "Markdown"
	}
}

func WithKeyboard(markup any) Option {
	return func(o *SendOptions) {
		o.ReplyMarkup = markup
	}
}

func WithCaption(caption string) Option {
	return func(o *SendOptions) {
		o.Caption = caption
	}
}

func WithFromChat(fromChatID any) Option {
	return func(o *SendOptions) {
		o.FromChatID = fromChatID
	}
}

func WithKeyboardRemove() Option {
	return func(o *SendOptions) {
		o.ReplyMarkup = &models.ReplyKeyboardRemove{RemoveKeyboard: true}
	}
}

func WithLastName(lastName string) Option {
	return func(o *SendOptions) {
		o.LastName = lastName
	}
}

func WithHorizontalAccuracy(accuracy float64) Option {
	return func(o *SendOptions) {
		o.HorizontalAccuracy = accuracy
	}
}

func WithOnlyIfBanned(onlyIfBanned bool) Option {
	return func(o *SendOptions) {
		o.OnlyIfBanned = onlyIfBanned
	}
}

func WithAdminCanChangeInfo(val bool) Option {
	return func(o *SendOptions) { o.CanChangeInfo = val }
}

func WithAdminCanPostMessages(val bool) Option {
	return func(o *SendOptions) { o.CanPostMessages = val }
}

func WithAdminCanEditMessages(val bool) Option {
	return func(o *SendOptions) { o.CanEditMessages = val }
}

func WithAdminCanDeleteMessages(val bool) Option {
	return func(o *SendOptions) { o.CanDeleteMessages = val }
}

func WithAdminCanManageVideoChats(val bool) Option {
	return func(o *SendOptions) { o.CanManageVideoChats = val }
}

func WithAdminCanInviteUsers(val bool) Option {
	return func(o *SendOptions) { o.CanInviteUsers = val }
}

func WithAdminCanRestrictMembers(val bool) Option {
	return func(o *SendOptions) { o.CanRestrictMembers = val }
}

func WithPhotoURL(url string) Option {
	return func(o *SendOptions) {
		o.PhotoURL = url
	}
}