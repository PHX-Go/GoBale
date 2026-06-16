package methods

type DeleteMessage struct {
	ChatID    any   `json:"chat_id"`
	MessageID int64 `json:"message_id"`
}

func (d DeleteMessage) Method() string {
	return "deleteMessage"
}

func (d DeleteMessage) Params() any {
	return d
}