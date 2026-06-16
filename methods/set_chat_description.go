package methods

type SetChatDescription struct {
	ChatID      any    `json:"chat_id"`
	Description string `json:"description"`
}

func (s SetChatDescription) Method() string { return "setChatDescription" }
func (s SetChatDescription) Params() any    { return s }