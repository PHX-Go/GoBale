package methods

type SendVideo struct {
	ChatID           any    `json:"chat_id"`
	Video            string `json:"video,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendVideo) Method() string {
	return "sendVideo"
}

func (s SendVideo) Params() any {
	return s
}