package methods

import "github.com/PHX-Go/GoBale/models"

type CreateInvoiceLink struct {
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Payload       string                `json:"payload"`
	ProviderToken string                `json:"provider_token"`
	Prices        []models.LabeledPrice `json:"prices"`
}

func (c CreateInvoiceLink) Method() string { return "createInvoiceLink" }
func (c CreateInvoiceLink) Params() any    { return c }