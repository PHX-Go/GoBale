package methods

type AskReview struct {
	UserID       int64 `json:"user_id"`
	DelaySeconds int   `json:"delay_seconds"`
}

func (a AskReview) Method() string {
	return "askReview"
}

func (a AskReview) Params() any {
	return a
}