package methods

type EditMessageText struct {
	ChatID      any    `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

func (e EditMessageText) Method() string { return "editMessageText" }
func (e EditMessageText) Params() any    { return e }