package methods

type SendPhoto struct {
	ChatID           any    `json:"chat_id"`
	FromChatID       any    `json:"from_chat_id,omitempty"`
	Photo            string `json:"photo,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendPhoto) Method() string {
	return "sendPhoto"
}

func (s SendPhoto) Params() any {
	return s
}