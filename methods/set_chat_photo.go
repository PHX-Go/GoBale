package methods

type SetChatPhoto struct {
	ChatID any `json:"chat_id"`
}

func (s SetChatPhoto) Method() string {
	return "setChatPhoto"
}

func (s SetChatPhoto) Params() any {
	return s
}