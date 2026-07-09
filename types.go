package gobale

import (
	"encoding/json"
	"io"
	"math"
)

// Handler represents the singular signature for all middlewares and closures
type Handler func(*Ctx)

// Photo represents a slice of PhotoSize objects (for different resolutions)
type Photo []PhotoSize

// SettingEntry stores system configuration toggle elements
type SettingEntry struct {
	Key         string
	Label       string
	Ptr         *bool
	Default     bool
	IsLocal     bool
	ConfirmText string
}

// Update represents the incoming update envelope from Bale servers
type Update struct {
	UpdateID         int               `json:"update_id"`
	Message          *Message          `json:"message,omitempty"`
	EditedMessage    *Message          `json:"edited_message,omitempty"`
	CallbackQuery    *CallbackQuery    `json:"callback_query,omitempty"`
	PreCheckoutQuery *PreCheckoutQuery `json:"pre_checkout_query,omitempty"`
}

// Message represents a single chat message containing text or media
type Message struct {
	MessageID            int64                 `json:"message_id"`
	Date                 int64                 `json:"date"`
	Chat                 Chat                  `json:"chat"`
	From                 *User                 `json:"from,omitempty"`
	SenderChat           *Chat                 `json:"sender_chat,omitempty"`
	ForwardFrom          *User                 `json:"forward_from,omitempty"`
	ForwardFromChat      *Chat                 `json:"forward_from_chat,omitempty"`
	ForwardFromMessageID int64                 `json:"forward_from_message_id,omitempty"`
	ForwardSignature     string                `json:"forward_signature,omitempty"`   // New
	ForwardSenderName    string                `json:"forward_sender_name,omitempty"` // New
	ForwardFromName      string                `json:"forward_from_name,omitempty"`   // New
	ForwardDate          int64                 `json:"forward_date,omitempty"`
	ReplyToMessage       *Message              `json:"reply_to_message,omitempty"`
	EditDate             int64                 `json:"edit_date,omitempty"`
	MediaGroupID         string                `json:"media_group_id,omitempty"`
	Text                 string                `json:"text,omitempty"`
	Entities             []MessageEntity       `json:"entities,omitempty"`
	Animation            *Animation            `json:"animation,omitempty"`
	Audio                *Audio                `json:"audio,omitempty"`
	Document             *Document             `json:"document,omitempty"`
	Photo                Photo                 `json:"photo,omitempty"`
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
	Poll                 *Poll                 `json:"poll,omitempty"`           // EXPERIMENTAL
	ForwardOrigin        *MessageOrigin        `json:"forward_origin,omitempty"` // EXPERIMENTAL
}

// User represents a Bale user account structure
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// Chat represents a private or group chat window
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// ChatFullInfo represents detailed metadata returned for specific chats
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
	LinkedChatID int64      `json:"linked_chat_id,omitempty"`
}

// CallbackQuery represents interactive inline button event payloads
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`             // New: for text_link
	User          *User  `json:"user,omitempty"`            // New: for text_mention
	Language      string `json:"language,omitempty"`        // New: for pre
	CustomEmojiID string `json:"custom_emoji_id,omitempty"` // New: for custom_emoji
}

// Animation represents silent loop video animation parameters
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

// Audio represents audio parameters
type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Title        string `json:"title,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// Document represents generic document attachments
type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

// PhotoSize represents image dimensions and storage indicators
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

// Sticker represents static sticker metadata
type Sticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

// Video represents video attachment characteristics
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

// Voice represents voice notes metadata
type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// Contact represents shared phone directory contacts
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
}

// Location represents geocoordinates parameters
type Location struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

// Invoice represents billing data
type Invoice struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TotalAmount int64  `json:"total_amount"`
}

// SuccessfulPayment represents completed payment structures
type SuccessfulPayment struct {
	Currency                string `json:"currency"`
	TotalAmount             int64  `json:"total_amount"`
	InvoicePayload          string `json:"invoice_payload"`
	ShippingOptionID        string `json:"shipping_option_id,omitempty"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id,omitempty"`
	BalePaymentChargeID     string `json:"bale_payment_charge_id,omitempty"`
	ProviderPaymentChargeID string `json:"provider_payment_charge_id,omitempty"`
}

// MessageId represents created message identification envelopes
type MessageId struct {
	MessageID int64 `json:"message_id"`
}

// File represents file metadata
type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// ReplyKeyboardMarkup represents custom static reply keyboard menus
type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool               `json:"one_time_keyboard,omitempty"`
}

// InlineKeyboardMarkup represents interactive click callback keyboard layouts
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// KeyboardButton represents structured click elements inside reply keyboards
type KeyboardButton struct {
	Text            string      `json:"text"`
	RequestContact  bool        `json:"request_contact,omitempty"`
	RequestLocation bool        `json:"request_location,omitempty"`
	WebApp          *WebAppInfo `json:"web_app,omitempty"`
}

// CopyTextButton represents text copy command wrapper
type CopyTextButton struct {
	Text string `json:"text"`
}

// InlineKeyboardButton represents interactive elements in glass inline keyboards
type InlineKeyboardButton struct {
	Text         string          `json:"text"`
	URL          string          `json:"url,omitempty"`
	CallbackData string          `json:"callback_data,omitempty"`
	WebApp       *WebAppInfo     `json:"web_app,omitempty"`
	CopyText     *CopyTextButton `json:"copy_text,omitempty"`
}

// ReplyKeyboardRemove triggers keyboard destruction layouts
type ReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
}

// WebAppData represents web app returned data payloads
type WebAppData struct {
	Data string `json:"data"`
}

// WebAppInfo contains webapp rendering URLs
type WebAppInfo struct {
	URL string `json:"url"`
}

// ChatMember represents a user membership context inside chats
type ChatMember struct {
	Status              string `json:"status"`
	User                User   `json:"user"`
	IsAnonymous         bool   `json:"is_anonymous,omitempty"`      // New
	CanManageChat       bool   `json:"can_manage_chat,omitempty"`   // New
	CanManageTopics     bool   `json:"can_manage_topics,omitempty"` // New
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

// ChatPhoto represents chat avatar file references
type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}

// ResponseParameters holds retry directives returned by the API
type ResponseParameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}

// InputMedia represents parent abstraction interface for media groups
type InputMedia interface {
	MediaType() string
}

// InputMediaPhoto represents a photo element inside album arrays
type InputMediaPhoto struct {
	Type    string `json:"type"`
	Media   string `json:"media"`
	Caption string `json:"caption,omitempty"`
}

// MediaType returns element type string
func (m InputMediaPhoto) MediaType() string { return "photo" }

// InputMediaVideo represents a video element inside album arrays
type InputMediaVideo struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

// MediaType returns element type string
func (m InputMediaVideo) MediaType() string { return "video" }

// InputMediaAnimation represents an animation element inside album arrays
type InputMediaAnimation struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

// MediaType returns element type string
func (m InputMediaAnimation) MediaType() string { return "animation" }

// InputMediaAudio represents an audio element inside album arrays
type InputMediaAudio struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Duration  int    `json:"duration,omitempty"`
	Title     string `json:"title,omitempty"`
}

// MediaType returns element type string
func (m InputMediaAudio) MediaType() string { return "audio" }

// InputMediaDocument represents a document element inside album arrays
type InputMediaDocument struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
}

// MediaType returns element type string
func (m InputMediaDocument) MediaType() string { return "document" }

// InputFile represents physical disk file streaming parameters
type InputFile struct {
	Field    string
	FileName string
	Reader   io.Reader
}

// ChatInviteLink represents chat invitation links
type ChatInviteLink struct {
	InviteLink string `json:"invite_link"`
	Creator    User   `json:"creator"`
	IsPrimary  bool   `json:"is_primary"`
	IsRevoked  bool   `json:"is_revoked"`
}

// StickerSet represents sticker package sets
type StickerSet struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Stickers  []Sticker  `json:"stickers"`
	Thumbnail *PhotoSize `json:"thumbnail,omitempty"`
}

// StickerInput defines the interface for different types of sticker inputs
type StickerInput interface {
	StickerValue() string
	IsLocal() bool
}

// StickerFileID represents a pre-uploaded sticker file ID
type StickerFileID string

// StickerValue returns the string value of the file ID
func (s StickerFileID) StickerValue() string {
	return string(s)
}

// IsLocal returns false since file ID is already on Bale servers
func (s StickerFileID) IsLocal() bool {
	return false
}

// MarshalJSON serializes the file ID into a clean JSON string
func (s StickerFileID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// StickerURL represents an online HTTP URL pointing to a sticker image
type StickerURL string

// StickerValue returns the string value of the URL
func (s StickerURL) StickerValue() string {
	return string(s)
}

// IsLocal returns false since the URL is an online link
func (s StickerURL) IsLocal() bool {
	return false
}

// MarshalJSON serializes the URL into a clean JSON string
func (s StickerURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// StickerFilePath represents a local physical file path of a sticker
type StickerFilePath string

// StickerValue returns the string path of the local file
func (s StickerFilePath) StickerValue() string {
	return string(s)
}

// IsLocal returns true since this represents a local physical file path
func (s StickerFilePath) IsLocal() bool {
	return true
}

// MarshalJSON serializes the file path into a clean JSON string
func (s StickerFilePath) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// InputSticker represents sticker configuration for custom sets with strict typing
type InputSticker struct {
	Sticker   StickerInput `json:"sticker"` // Refactored: strict StickerInput interface
	EmojiList []string     `json:"emoji_list"`
	Keywords  []string     `json:"keywords,omitempty"`
}

// LabeledPrice represents billing price elements
type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}

// Transaction represents transaction logs on Bale database
type Transaction struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UserID    int64  `json:"userID"`
	Amount    int64  `json:"amount"`
	CreatedAt int64  `json:"createdAt"`
}

// PreCheckoutQuery represents payment validation requests
type PreCheckoutQuery struct {
	ID             string `json:"id"`
	From           User   `json:"from"`
	Currency       string `json:"currency"`
	TotalAmount    int64  `json:"total_amount"`
	InvoicePayload string `json:"invoice_payload"`
}

// IsCreator checks if the member is the owner/creator of the chat
func (cm *ChatMember) IsCreator() bool {
	return cm.Status == "creator"
}

// IsAdmin checks if the member has administrator or creator privileges
func (cm *ChatMember) IsAdmin() bool {
	return cm.Status == "administrator" || cm.Status == "creator"
}

// IsRegularMember checks if the member is a standard regular group member
func (cm *ChatMember) IsRegularMember() bool {
	return cm.Status == "member"
}

// Mention returns the formatted mention string for the user (prefixed with @ if username is present)
func (u *User) Mention() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return Bold(u.FirstName)
}

// WebhookInfo contains information about the current status of a webhook on Bale servers
type WebhookInfo struct {
	URL                  string   `json:"url"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	IPAddress            string   `json:"ip_address,omitempty"`
	LastErrorDate        int64    `json:"last_error_date,omitempty"`
	LastErrorMessage     string   `json:"last_error_message,omitempty"`
	MaxConnections       int      `json:"max_connections,omitempty"`
	AllowedUpdates       []string `json:"allowed_updates,omitempty"`
}

// SafirResponse represents the JSON output envelope returned by Bale Safir servers
type SafirResponse struct {
	MessageID string     `json:"message_id"`
	ErrorData []SafirErr `json:"error_data"`
}

// SafirErr represents individual recipient validation errors returned by Safir
type SafirErr struct {
	PhoneNumber string `json:"phone_number"`
	Code        int    `json:"code"`
	Description string `json:"description"`
}

// MessageOrigin represents the origin of a forwarded message (EXPERIMENTAL)
type MessageOrigin struct {
	Type            string `json:"type"`                       // "user", "hidden_user", "chat", or "channel"
	Date            int64  `json:"date"`                       // Date the message was originally sent
	SenderUser      *User  `json:"sender_user,omitempty"`      // Present if Type is "user"
	SenderUserName  string `json:"sender_user_name,omitempty"` // Present if Type is "hidden_user"
	SenderChat      *Chat  `json:"sender_chat,omitempty"`      // Present if Type is "chat"
	AuthorSignature string `json:"author_signature,omitempty"` // Present if Type is "chat" or "channel"
	Chat            *Chat  `json:"chat,omitempty"`             // Present if Type is "channel"
	MessageID       int64  `json:"message_id,omitempty"`       // Present if Type is "channel"
}

// Largest returns the PhotoSize with the highest resolution (usually the last element)
func (p Photo) Largest() *PhotoSize {
	if len(p) == 0 {
		return nil
	}
	// Bale and Telegram put the largest photo at the end of the slice
	return &p[len(p)-1]
}

// DistanceTo calculates the distance in kilometers between two locations
func (l *Location) DistanceTo(other *Location) float64 {
	const earthRadius = 6371.0 // Earth radius in kilometers

	lat1 := l.Latitude * math.Pi / 180
	lon1 := l.Longitude * math.Pi / 180
	lat2 := other.Latitude * math.Pi / 180
	lon2 := other.Longitude * math.Pi / 180

	dlat := lat2 - lat1
	dlon := lon2 - lon1

	// Haversine formula implementation
	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(dlon/2), 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// EXPERIMENTAL (Not Implemented)

// PollOption represents a single choice in a poll
type PollOption struct {
	Text         string `json:"text"`
	VoterCount   int    `json:"voter_count"`
	PersistentID int    `json:"persistent_id"` // Detected: Bale specific
}

// Poll contains all information about a poll
type Poll struct {
	ID                    string       `json:"id"`
	Question              string       `json:"question"`
	Options               []PollOption `json:"options"`
	TotalVoterCount       int          `json:"total_voter_count"`
	IsClosed              bool         `json:"is_closed"`
	IsAnonymous           bool         `json:"is_anonymous"`
	Type                  string       `json:"type"`
	AllowsMultipleAnswers bool         `json:"allows_multiple_answers"`
	AllowsRevoting        bool         `json:"allows_revoting"` // Detected: Bale specific
	MembersOnly           bool         `json:"members_only"`    // Detected: Bale specific
}
