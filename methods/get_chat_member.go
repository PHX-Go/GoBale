package methods

type GetChatMember struct {
	ChatID any `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

func (g GetChatMember) Method() string {
	return "getChatMember"
}

func (g GetChatMember) Params() any {
	return g
}