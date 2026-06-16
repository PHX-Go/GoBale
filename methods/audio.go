package methods

type SendAudio struct {
	ChatID           any    `json:"chat_id"`
	Audio            string `json:"audio,omitempty"`
	Caption          string `json:"caption,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendAudio) Method() string {
	return "sendAudio"
}

func (s SendAudio) Params() any {
	return s
}