package methods

type EditMessageCaption struct {
	ChatID      any    `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	Caption     string `json:"caption,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

func (e EditMessageCaption) Method() string { return "editMessageCaption" }
func (e EditMessageCaption) Params() any    { return e }