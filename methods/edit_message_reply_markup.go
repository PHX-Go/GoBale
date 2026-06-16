package methods

type EditMessageReplyMarkup struct {
	ChatID      any   `json:"chat_id"`
	MessageID   int64 `json:"message_id"`
	ReplyMarkup any   `json:"reply_markup,omitempty"`
}

func (e EditMessageReplyMarkup) Method() string { return "editMessageReplyMarkup" }
func (e EditMessageReplyMarkup) Params() any    { return e }