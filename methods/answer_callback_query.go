package methods

type AnswerCallbackQuery struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

func (a AnswerCallbackQuery) Method() string {
	return "answerCallbackQuery"
}

func (a AnswerCallbackQuery) Params() any {
	return a
}