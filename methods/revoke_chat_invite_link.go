package methods

type RevokeChatInviteLink struct {
	ChatID     any    `json:"chat_id"`
	InviteLink string `json:"invite_link"`
}

func (r RevokeChatInviteLink) Method() string { return "revokeChatInviteLink" }
func (r RevokeChatInviteLink) Params() any    { return r }