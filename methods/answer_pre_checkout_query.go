package methods

type AnswerPreCheckoutQuery struct {
	PreCheckoutQueryID string `json:"pre_checkout_query_id"`
	OK                 bool   `json:"ok"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

func (a AnswerPreCheckoutQuery) Method() string { return "answerPreCheckoutQuery" }
func (a AnswerPreCheckoutQuery) Params() any    { return a }