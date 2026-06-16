package methods

type GetChatMembersCount struct {
	ChatID any `json:"chat_id"`
}

func (g GetChatMembersCount) Method() string {
	return "getChatMembersCount"
}

func (g GetChatMembersCount) Params() any {
	return g
}