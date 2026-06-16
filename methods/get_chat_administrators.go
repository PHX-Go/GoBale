package methods

type GetChatAdministrators struct {
	ChatID any `json:"chat_id"`
}

func (g GetChatAdministrators) Method() string {
	return "getChatAdministrators"
}

func (g GetChatAdministrators) Params() any {
	return g
}