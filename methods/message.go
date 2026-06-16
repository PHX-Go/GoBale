package methods

type SendMessage struct {
	ChatID           any    `json:"chat_id"`
	Text             string `json:"text"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (m SendMessage) Method() string {
	return "sendMessage"
}

func (m SendMessage) Params() any {
	return m
}