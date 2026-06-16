package methods

type ForwardMessage struct {
	ChatID     any   `json:"chat_id"`
	FromChatID any   `json:"from_chat_id"`
	MessageID  int64 `json:"message_id"`
}

func (f ForwardMessage) Method() string {
	return "forwardMessage"
}

func (f ForwardMessage) Params() any {
	return f
}