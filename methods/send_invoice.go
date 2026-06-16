package methods

import "github.com/PHX-Go/GoBale/models"

type SendInvoice struct {
	ChatID        any                   `json:"chat_id"`
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Payload       string                `json:"payload"`
	ProviderToken string                `json:"provider_token"`
	Currency      string                `json:"currency"`
	Prices        []models.LabeledPrice `json:"prices"`
}

func (s SendInvoice) Method() string {
	return "sendInvoice"
}

func (s SendInvoice) Params() any {
	return s
}