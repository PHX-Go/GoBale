package methods

type CreateChatInviteLink struct {
	ChatID any `json:"chat_id"`
}

func (c CreateChatInviteLink) Method() string { return "createChatInviteLink" }
func (c CreateChatInviteLink) Params() any    { return c }