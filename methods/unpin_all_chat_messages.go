package methods

type UnpinAllChatMessages struct {
	ChatID any `json:"chat_id"`
}

func (u UnpinAllChatMessages) Method() string {
	return "unpinAllChatMessages"
}

func (u UnpinAllChatMessages) Params() any {
	return u
}