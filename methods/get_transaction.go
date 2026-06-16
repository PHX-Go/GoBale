package methods

type GetTransaction struct {
	TransactionID string `json:"transaction_id"`
}

func (g GetTransaction) Method() string {
	return "getTransaction"
}

func (g GetTransaction) Params() any {
	return g
}