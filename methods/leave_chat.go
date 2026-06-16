package methods

type LeaveChat struct {
	ChatID any `json:"chat_id"`
}

func (l LeaveChat) Method() string {
	return "leaveChat"
}

func (l LeaveChat) Params() any {
	return l
}