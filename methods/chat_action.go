package methods

type SendChatAction struct {
	ChatID any    `json:"chat_id"`
	Action string `json:"action"`
}

func (s SendChatAction) Method() string {
	return "sendChatAction"
}

func (s SendChatAction) Params() any {
	return s
}