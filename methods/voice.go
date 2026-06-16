package methods

type SendVoice struct {
	ChatID           any    `json:"chat_id"`
	Voice            string `json:"voice,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendVoice) Method() string {
	return "sendVoice"
}

func (s SendVoice) Params() any {
	return s
}