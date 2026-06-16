package methods

type UnPinChatMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

func (u UnPinChatMessage) Method() string {
	return "unPinChatMessage"
}

func (u UnPinChatMessage) Params() any {
	return u
}