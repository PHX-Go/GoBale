package methods

type DeleteChatPhoto struct {
	ChatID any `json:"chat_id"`
}

func (d DeleteChatPhoto) Method() string { return "deleteChatPhoto" }
func (d DeleteChatPhoto) Params() any    { return d }