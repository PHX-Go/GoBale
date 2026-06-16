package methods

type PinChatMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

func (p PinChatMessage) Method() string {
	return "pinChatMessage"
}

func (p PinChatMessage) Params() any {
	return p
}