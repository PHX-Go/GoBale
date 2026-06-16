package models

import (
	"io"
)

type Update struct {
	UpdateID         int               `json:"update_id"`
	Message          *Message          `json:"message,omitempty"`
	EditedMessage    *Message          `json:"edited_message,omitempty"`
	CallbackQuery    *CallbackQuery    `json:"callback_query,omitempty"`
	PreCheckoutQuery *PreCheckoutQuery `json:"pre_checkout_query,omitempty"`
}

type Message struct {
	MessageID            int64                 `json:"message_id"`
	Date                 int64                 `json:"date"`
	Chat                 Chat                  `json:"chat"`
	From                 *User                 `json:"from,omitempty"`
	SenderChat           *Chat                 `json:"sender_chat,omitempty"`
	ForwardFrom          *User                 `json:"forward_from,omitempty"`
	ForwardFromChat      *Chat                 `json:"forward_from_chat,omitempty"`
	ForwardFromMessageID int64                 `json:"forward_from_message_id,omitempty"`
	ForwardDate          int64                 `json:"forward_date,omitempty"`
	ReplyToMessage       *Message              `json:"reply_to_message,omitempty"`
	EditDate             int64                 `json:"edit_date,omitempty"`
	MediaGroupID         string                `json:"media_group_id,omitempty"`
	Text                 string                `json:"text,omitempty"`
	Entities             []MessageEntity       `json:"entities,omitempty"`
	Animation            *Animation            `json:"animation,omitempty"`
	Audio                *Audio                `json:"audio,omitempty"`
	Document             *Document             `json:"document,omitempty"`
	Photo                []PhotoSize           `json:"photo,omitempty"`
	Sticker              *Sticker              `json:"sticker,omitempty"`
	Video                *Video                `json:"video,omitempty"`
	Voice                *Voice                `json:"voice,omitempty"`
	Caption              string                `json:"caption,omitempty"`
	CaptionEntities      []MessageEntity       `json:"caption_entities,omitempty"`
	Contact              *Contact              `json:"contact,omitempty"`
	Location             *Location             `json:"location,omitempty"`
	NewChatMembers       []User                `json:"new_chat_members,omitempty"`
	LeftChatMember       *User                 `json:"left_chat_member,omitempty"`
	Invoice              *Invoice              `json:"invoice,omitempty"`
	SuccessfulPayment    *SuccessfulPayment    `json:"successful_payment,omitempty"`
	WebAppData           *WebAppData           `json:"web_app_data,omitempty"`
	ReplyMarkup          *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type ChatFullInfo struct {
	ID           int64      `json:"id"`
	Type         string     `json:"type"`
	Title        string     `json:"title,omitempty"`
	Username     string     `json:"username,omitempty"`
	FirstName    string     `json:"first_name,omitempty"`
	LastName     string     `json:"last_name,omitempty"`
	Photo        *ChatPhoto `json:"photo,omitempty"`
	Bio          string     `json:"bio,omitempty"`
	Description  string     `json:"description,omitempty"`
	InviteLink   string     `json:"invite_link,omitempty"`
	LinkedChatID string     `json:"linked_chat_id,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

type Animation struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Title        string `json:"title,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type Sticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
}

type Location struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type Invoice struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TotalAmount int64  `json:"total_amount"`
}

type SuccessfulPayment struct {
	Currency                string `json:"currency"`
	TotalAmount             int64  `json:"total_amount"`
	InvoicePayload          string `json:"invoice_payload"`
	ShippingOptionID        string `json:"shipping_option_id,omitempty"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id,omitempty"`
	ProviderPaymentChargeID string `json:"provider_payment_charge_id,omitempty"`
}

type MessageId struct {
	MessageID int64 `json:"message_id"`
}

type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool               `json:"one_time_keyboard,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type KeyboardButton struct {
	Text            string      `json:"text"`
	RequestContact  bool        `json:"request_contact,omitempty"`
	RequestLocation bool        `json:"request_location,omitempty"`
	WebApp          *WebAppInfo `json:"web_app,omitempty"`
}

type CopyTextButton struct {
	Text string `json:"text"`
}

type InlineKeyboardButton struct {
	Text         string          `json:"text"`
	URL          string          `json:"url,omitempty"`
	CallbackData string          `json:"callback_data,omitempty"`
	WebApp       *WebAppInfo     `json:"web_app,omitempty"`
	CopyText     *CopyTextButton `json:"copy_text,omitempty"`
}

type ReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
}

type WebAppData struct {
	Data string `json:"data"`
}

type WebAppInfo struct {
	URL string `json:"url"`
}

type ChatMember struct {
	Status              string `json:"status"`
	User                User   `json:"user"`
	CanDeleteMessages   bool   `json:"can_delete_messages,omitempty"`
	CanManageVideoChats bool   `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers  bool   `json:"can_restrict_members,omitempty"`
	CanPromoteMembers   bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo       bool   `json:"can_change_info,omitempty"`
	CanInviteUsers      bool   `json:"can_invite_users,omitempty"`
	CanPostStories      bool   `json:"can_post_stories,omitempty"`
	CanPostMessages     bool   `json:"can_post_messages,omitempty"`
	CanEditMessages     bool   `json:"can_edit_messages,omitempty"`
	CanPinMessages      bool   `json:"can_pin_messages,omitempty"`
	IsMember            bool   `json:"is_member,omitempty"`
	CanSendMessages     bool   `json:"can_send_messages,omitempty"`
	CanSendAudios       bool   `json:"can_send_audios,omitempty"`
	CanSendDocuments    bool   `json:"can_send_documents,omitempty"`
	CanSendPhotos       bool   `json:"can_send_photos,omitempty"`
	CanSendVideos       bool   `json:"can_send_videos,omitempty"`
}

type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}

type ResponseParameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}

type InputMedia interface {
	MediaType() string
}

type InputMediaPhoto struct {
	Type    string `json:"type"`
	Media   string `json:"media"`
	Caption string `json:"caption,omitempty"`
}

func (m InputMediaPhoto) MediaType() string { return "photo" }

type InputMediaVideo struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

func (m InputMediaVideo) MediaType() string { return "video" }

type InputMediaAnimation struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

func (m InputMediaAnimation) MediaType() string { return "animation" }

type InputMediaAudio struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Duration  int    `json:"duration,omitempty"`
	Title     string `json:"title,omitempty"`
}

func (m InputMediaAudio) MediaType() string { return "audio" }

type InputMediaDocument struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
}

func (m InputMediaDocument) MediaType() string { return "document" }

type InputFile struct {
	Field    string
	FileName string
	Reader   io.Reader
}

type ChatInviteLink struct {
	InviteLink string `json:"invite_link"`
	Creator    User   `json:"creator"`
	IsPrimary  bool   `json:"is_primary"`
	IsRevoked  bool   `json:"is_revoked"`
}

type StickerSet struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Stickers  []Sticker  `json:"stickers"`
	Thumbnail *PhotoSize `json:"thumbnail,omitempty"`
}

type InputSticker struct {
	Sticker   any      `json:"sticker"`
	EmojiList []string `json:"emoji_list"`
	Keywords  []string `json:"keywords,omitempty"`
}

type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}

type Transaction struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UserID    int64  `json:"userID"`
	Amount    int64  `json:"amount"`
	CreatedAt int64  `json:"createdAt"`
}

type PreCheckoutQuery struct {
	ID             string `json:"id"`
	From           User   `json:"from"`
	Currency       string `json:"currency"`
	TotalAmount    int64  `json:"total_amount"`
	InvoicePayload string `json:"invoice_payload"`
}