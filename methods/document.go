package methods

type SendDocument struct {
	ChatID           any    `json:"chat_id"`
	Document         string `json:"document,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendDocument) Method() string {
	return "sendDocument"
}

func (s SendDocument) Params() any {
	return s
}