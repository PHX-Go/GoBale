package methods

type BanChatMember struct {
	ChatID any   `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

func (b BanChatMember) Method() string {
	return "banChatMember"
}

func (b BanChatMember) Params() any {
	return b
}