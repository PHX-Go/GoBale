package gobale

// SendMessage holds text message sending parameters
type SendMessage struct {
	ChatID           any    `json:"chat_id"`
	Text             string `json:"text"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (m SendMessage) Method() string {
	return "sendMessage"
}

// Params returns the request parameters
func (m SendMessage) Params() any {
	return m
}

// SendAnimation holds animation sending parameters
type SendAnimation struct {
	ChatID           any    `json:"chat_id"`
	Animation        string `json:"animation,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendAnimation) Method() string {
	return "sendAnimation"
}

// Params returns the request parameters
func (s SendAnimation) Params() any {
	return s
}

// SendAudio holds audio sending parameters
type SendAudio struct {
	ChatID           any    `json:"chat_id"`
	Audio            string `json:"audio,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendAudio) Method() string {
	return "sendAudio"
}

// Params returns the request parameters
func (s SendAudio) Params() any {
	return s
}

// SendDocument holds document sending parameters
type SendDocument struct {
	ChatID           any    `json:"chat_id"`
	Document         string `json:"document,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendDocument) Method() string {
	return "sendDocument"
}

// Params returns the request parameters
func (s SendDocument) Params() any {
	return s
}

// SendVideo holds video sending parameters
type SendVideo struct {
	ChatID           any    `json:"chat_id"`
	Video            string `json:"video,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendVideo) Method() string {
	return "sendVideo"
}

// Params returns the request parameters
func (s SendVideo) Params() any {
	return s
}

// SendVoice holds voice sending parameters
type SendVoice struct {
	ChatID           any    `json:"chat_id"`
	Voice            string `json:"voice,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendVoice) Method() string {
	return "sendVoice"
}

// Params returns the request parameters
func (s SendVoice) Params() any {
	return s
}

// SendLocation holds location sending parameters
type SendLocation struct {
	ChatID             any     `json:"chat_id"`
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	HorizontalAccuracy float64 `json:"horizontal_accuracy,omitempty"`
	ReplyToMessageID   int64   `json:"reply_to_message_id,omitempty"`
	ReplyMarkup        any     `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendLocation) Method() string {
	return "sendLocation"
}

// Params returns the request parameters
func (s SendLocation) Params() any {
	return s
}

// SendContact holds contact sending parameters
type SendContact struct {
	ChatID           any    `json:"chat_id"`
	PhoneNumber      any    `json:"phone_number"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (s SendContact) Method() string {
	return "sendContact"
}

// Params returns the request parameters
func (s SendContact) Params() any {
	return s
}

// SendChatAction holds chat action parameters
type SendChatAction struct {
	ChatID any    `json:"chat_id"`
	Action string `json:"action"`
}

// Method returns the API method name
func (s SendChatAction) Method() string {
	return "sendChatAction"
}

// Params returns the request parameters
func (s SendChatAction) Params() any {
	return s
}

// SendMediaGroup holds album sending parameters
type SendMediaGroup struct {
	ChatID           any   `json:"chat_id"`
	Media            any   `json:"media"`
	ReplyToMessageID int64 `json:"reply_to_message_id,omitempty"`
}

// Method returns the API method name
func (s SendMediaGroup) Method() string {
	return "sendMediaGroup"
}

// Params returns the request parameters
func (s SendMediaGroup) Params() any {
	return s
}

// ForwardMessage holds forwarding parameters
type ForwardMessage struct {
	ChatID     any   `json:"chat_id"`
	FromChatID any   `json:"from_chat_id"`
	MessageID  int64 `json:"message_id"`
}

// Method returns the API method name
func (f ForwardMessage) Method() string {
	return "forwardMessage"
}

// Params returns the request parameters
func (f ForwardMessage) Params() any {
	return f
}

// CopyMessage holds copying parameters
type CopyMessage struct {
	ChatID     any   `json:"chat_id"`
	FromChatID any   `json:"from_chat_id"`
	MessageID  int64 `json:"message_id"`
}

// Method returns the API method name
func (c CopyMessage) Method() string {
	return "copyMessage"
}

// Params returns the request parameters
func (c CopyMessage) Params() any {
	return c
}

// DeleteMessage holds deletion parameters
type DeleteMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

// Method returns the API method name
func (d DeleteMessage) Method() string {
	return "deleteMessage"
}

// Params returns the request parameters
func (d DeleteMessage) Params() any {
	return d
}

// EditMessageText holds text edit parameters
type EditMessageText struct {
	ChatID      any    `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (e EditMessageText) Method() string {
	return "editMessageText"
}

// Params returns the request parameters
func (e EditMessageText) Params() any {
	return e
}

// EditMessageCaption holds caption edit parameters
type EditMessageCaption struct {
	ChatID      any    `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	Caption     string `json:"caption,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (e EditMessageCaption) Method() string {
	return "editMessageCaption"
}

// Params returns the request parameters
func (e EditMessageCaption) Params() any {
	return e
}

// EditMessageReplyMarkup holds markup edit parameters
type EditMessageReplyMarkup struct {
	ChatID      any   `json:"chat_id"`
	MessageID   int64 `json:"message_id"`
	ReplyMarkup any   `json:"reply_markup,omitempty"`
}

// Method returns the API method name
func (e EditMessageReplyMarkup) Method() string {
	return "editMessageReplyMarkup"
}

// Params returns the request parameters
func (e EditMessageReplyMarkup) Params() any {
	return e
}

// GetChat holds chat information retrieval parameters
type GetChat struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (g GetChat) Method() string {
	return "getChat"
}

// Params returns the request parameters
func (g GetChat) Params() any {
	return g
}

// GetChatAdministrators holds admin retrieval parameters
type GetChatAdministrators struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (g GetChatAdministrators) Method() string {
	return "getChatAdministrators"
}

// Params returns the request parameters
func (g GetChatAdministrators) Params() any {
	return g
}

// GetChatMember holds chat member retrieval parameters
type GetChatMember struct {
	ChatID any   `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

// Method returns the API method name
func (g GetChatMember) Method() string {
	return "getChatMember"
}

// Params returns the request parameters
func (g GetChatMember) Params() any {
	return g
}

// GetChatMembersCount holds member count parameters
type GetChatMembersCount struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (g GetChatMembersCount) Method() string {
	return "getChatMembersCount"
}

// Params returns the request parameters
func (g GetChatMembersCount) Params() any {
	return g
}

// BanChatMember holds user banning parameters
type BanChatMember struct {
	ChatID any   `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

// Method returns the API method name
func (b BanChatMember) Method() string {
	return "banChatMember"
}

// Params returns the request parameters
func (b BanChatMember) Params() any {
	return b
}

// UnbanChatMember holds user unbanning parameters
type UnbanChatMember struct {
	ChatID       any   `json:"chat_id"`
	UserID       int64 `json:"user_id"`
	OnlyIfBanned bool  `json:"only_if_banned,omitempty"`
}

// Method returns the API method name
func (u UnbanChatMember) Method() string {
	return "unbanChatMember"
}

// Params returns the request parameters
func (u UnbanChatMember) Params() any {
	return u
}

// PromoteChatMember holds admin promotion parameters
type PromoteChatMember struct {
	ChatID              any   `json:"chat_id"`
	UserID              int64 `json:"user_id"`
	CanChangeInfo       bool  `json:"can_change_info,omitempty"`
	CanPostMessages     bool  `json:"can_post_messages,omitempty"`
	CanEditMessages     bool  `json:"can_edit_messages,omitempty"`
	CanDeleteMessages   bool  `json:"can_delete_messages,omitempty"`
	CanManageVideoChats bool  `json:"can_manage_video_chats,omitempty"`
	CanInviteUsers      bool  `json:"can_invite_users,omitempty"`
	CanRestrictMembers  bool  `json:"can_restrict_members,omitempty"`
}

// Method returns the API method name
func (p PromoteChatMember) Method() string {
	return "promoteChatMember"
}

// Params returns the request parameters
func (p PromoteChatMember) Params() any {
	return p
}

// SetChatTitle holds title editing parameters
type SetChatTitle struct {
	ChatID any    `json:"chat_id"`
	Title  string `json:"title"`
}

// Method returns the API method name
func (s SetChatTitle) Method() string {
	return "setChatTitle"
}

// Params returns the request parameters
func (s SetChatTitle) Params() any {
	return s
}

// SetChatDescription holds description editing parameters
type SetChatDescription struct {
	ChatID      any    `json:"chat_id"`
	Description string `json:"description"`
}

// Method returns the API method name
func (s SetChatDescription) Method() string {
	return "setChatDescription"
}

// Params returns the request parameters
func (s SetChatDescription) Params() any {
	return s
}

// SetChatPhoto holds photo editing parameters
type SetChatPhoto struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (s SetChatPhoto) Method() string {
	return "setChatPhoto"
}

// Params returns the request parameters
func (s SetChatPhoto) Params() any {
	return s
}

// DeleteChatPhoto holds photo deletion parameters
type DeleteChatPhoto struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (d DeleteChatPhoto) Method() string {
	return "deleteChatPhoto"
}

// Params returns the request parameters
func (d DeleteChatPhoto) Params() any {
	return d
}

// LeaveChat holds chat leaving parameters
type LeaveChat struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (l LeaveChat) Method() string {
	return "leaveChat"
}

// Params returns the request parameters
func (l LeaveChat) Params() any {
	return l
}

// CreateChatInviteLink holds invite link creation parameters
type CreateChatInviteLink struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (c CreateChatInviteLink) Method() string {
	return "createChatInviteLink"
}

// Params returns the request parameters
func (c CreateChatInviteLink) Params() any {
	return c
}

// RevokeChatInviteLink holds invite link revocation parameters
type RevokeChatInviteLink struct {
	ChatID     any    `json:"chat_id"`
	InviteLink string `json:"invite_link"`
}

// Method returns the API method name
func (r RevokeChatInviteLink) Method() string {
	return "revokeChatInviteLink"
}

// Params returns the request parameters
func (r RevokeChatInviteLink) Params() any {
	return r
}

// ExportChatInviteLink holds invite link export parameters
type ExportChatInviteLink struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (e ExportChatInviteLink) Method() string {
	return "exportChatInviteLink"
}

// Params returns the request parameters
func (e ExportChatInviteLink) Params() any {
	return e
}

// PinChatMessage holds message pinning parameters
type PinChatMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

// Method returns the API method name
func (p PinChatMessage) Method() string {
	return "pinChatMessage"
}

// Params returns the request parameters
func (p PinChatMessage) Params() any {
	return p
}

// UnPinChatMessage holds message unpinning parameters
type UnPinChatMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

// Method returns the API method name
func (u UnPinChatMessage) Method() string {
	return "unPinChatMessage"
}

// Params returns the request parameters
func (u UnPinChatMessage) Params() any {
	return u
}

// UnpinAllChatMessages holds multiple unpinning parameters
type UnpinAllChatMessages struct {
	ChatID any `json:"chat_id"`
}

// Method returns the API method name
func (u UnpinAllChatMessages) Method() string {
	return "unpinAllChatMessages"
}

// Params returns the request parameters
func (u UnpinAllChatMessages) Params() any {
	return u
}

// GetMe holds bot identity retrieval parameters
type GetMe struct{}

// Method returns the API method name
func (g GetMe) Method() string {
	return "getMe"
}

// Params returns the request parameters
func (g GetMe) Params() any {
	return nil
}

// GetFile holds file information retrieval parameters
type GetFile struct {
	FileID string `json:"file_id"`
}

// Method returns the API method name
func (g GetFile) Method() string {
	return "getFile"
}

// Params returns the request parameters
func (g GetFile) Params() any {
	return g
}

// UploadStickerFile holds sticker file upload parameters
type UploadStickerFile struct {
	UserID int64 `json:"user_id"`
}

// Method returns the API method name
func (u UploadStickerFile) Method() string {
	return "uploadStickerFile"
}

// Params returns the request parameters
func (u UploadStickerFile) Params() any {
	return u
}

// CreateNewStickerSet holds sticker set creation parameters
type CreateNewStickerSet struct {
	UserID  int64          `json:"user_id"`
	Name    string         `json:"name"`
	Title   string         `json:"title"`
	Sticker []InputSticker `json:"sticker"`
}

// Method returns the API method name
func (c CreateNewStickerSet) Method() string {
	return "createNewStickerSet"
}

// Params returns the request parameters
func (c CreateNewStickerSet) Params() any {
	return c
}

// AddStickerToSet holds sticker addition parameters
type AddStickerToSet struct {
	UserID  int64        `json:"user_id"`
	Name    string       `json:"name"`
	Sticker InputSticker `json:"sticker"`
}

// Method returns the API method name
func (a AddStickerToSet) Method() string {
	return "addStickerToSet"
}

// Params returns the request parameters
func (a AddStickerToSet) Params() any {
	return a
}

// SendInvoice holds invoice sending parameters
type SendInvoice struct {
	ChatID        any            `json:"chat_id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Payload       string         `json:"payload"`
	ProviderToken string         `json:"provider_token"`
	Currency      string         `json:"currency"`
	Prices        []LabeledPrice `json:"prices"`
}

// Method returns the API method name
func (s SendInvoice) Method() string {
	return "sendInvoice"
}

// Params returns the request parameters
func (s SendInvoice) Params() any {
	return s
}

// CreateInvoiceLink holds invoice link creation parameters
type CreateInvoiceLink struct {
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Payload       string         `json:"payload"`
	ProviderToken string         `json:"provider_token"`
	Prices        []LabeledPrice `json:"prices"`
}

// Method returns the API method name
func (c CreateInvoiceLink) Method() string {
	return "createInvoiceLink"
}

// Params returns the request parameters
func (c CreateInvoiceLink) Params() any {
	return c
}

// AnswerPreCheckoutQuery holds pre-checkout query response parameters
type AnswerPreCheckoutQuery struct {
	PreCheckoutQueryID string `json:"pre_checkout_query_id"`
	OK                 bool   `json:"ok"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

// Method returns the API method name
func (a AnswerPreCheckoutQuery) Method() string {
	return "answerPreCheckoutQuery"
}

// Params returns the request parameters
func (a AnswerPreCheckoutQuery) Params() any {
	return a
}

// AnswerCallbackQuery holds callback query response parameters
type AnswerCallbackQuery struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

// Method returns the API method name
func (a AnswerCallbackQuery) Method() string {
	return "answerCallbackQuery"
}

// Params returns the request parameters
func (a AnswerCallbackQuery) Params() any {
	return a
}

// GetTransaction holds transaction retrieval parameters
type GetTransaction struct {
	TransactionID string `json:"transaction_id"`
}

// Method returns the API method name
func (g GetTransaction) Method() string {
	return "getTransaction"
}

// Params returns the request parameters
func (g GetTransaction) Params() any {
	return g
}

// InquireTransaction holds transaction inquiry parameters
type InquireTransaction struct {
	TransactionID string `json:"transaction_id"`
}

// Method returns the API method name
func (i InquireTransaction) Method() string {
	return "inquireTransaction"
}

// Params returns the request parameters
func (i InquireTransaction) Params() any {
	return i
}

// AskReview holds review request parameters
type AskReview struct {
	UserID       int64 `json:"user_id"`
	DelaySeconds int   `json:"delay_seconds"`
}

// Method returns the API method name
func (a AskReview) Method() string {
	return "askReview"
}

// Params returns the request parameters
func (a AskReview) Params() any {
	return a
}
