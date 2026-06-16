package methods

type CopyMessage struct {
	ChatID     any   `json:"chat_id"`
	FromChatID any   `json:"from_chat_id"`
	MessageID  int64 `json:"message_id"`
}

func (c CopyMessage) Method() string {
	return "copyMessage"
}

func (c CopyMessage) Params() any {
	return c
}