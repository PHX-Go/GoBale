package methods

type UnbanChatMember struct {
	ChatID       any   `json:"chat_id"`
	UserID       int64 `json:"user_id"`
	OnlyIfBanned bool  `json:"only_if_banned,omitempty"`
}

func (u UnbanChatMember) Method() string {
	return "unbanChatMember"
}

func (u UnbanChatMember) Params() any {
	return u
}