package methods

type ExportChatInviteLink struct {
	ChatID any `json:"chat_id"`
}

func (e ExportChatInviteLink) Method() string { return "exportChatInviteLink" }
func (e ExportChatInviteLink) Params() any    { return e }