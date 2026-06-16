package methods

type SetChatTitle struct {
	ChatID any    `json:"chat_id"`
	Title  string `json:"title"`
}

func (s SetChatTitle) Method() string { return "setChatTitle" }
func (s SetChatTitle) Params() any    { return s }