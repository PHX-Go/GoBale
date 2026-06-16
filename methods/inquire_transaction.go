package methods

type InquireTransaction struct {
	TransactionID string `json:"transaction_id"`
}

func (i InquireTransaction) Method() string { return "inquireTransaction" }
func (i InquireTransaction) Params() any    { return i }