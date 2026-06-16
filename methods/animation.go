package methods

type SendAnimation struct {
	ChatID           any    `json:"chat_id"`
	Animation        string `json:"animation,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendAnimation) Method() string {
	return "sendAnimation"
}

func (s SendAnimation) Params() any {
	return s
}