package methods

type GetChat struct {
	ChatID any `json:"chat_id"`
}

func (g GetChat) Method() string {
	return "getChat"
}

func (g GetChat) Params() any {
	return g
}