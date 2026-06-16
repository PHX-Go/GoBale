package models

type InvoiceBuilder struct {
	prices []LabeledPrice
}

func NewInvoice() *InvoiceBuilder {
	return &InvoiceBuilder{
		prices: make([]LabeledPrice, 0),
	}
}

func (b *InvoiceBuilder) AddPrice(label string, amount int64) *InvoiceBuilder {
	b.prices = append(b.prices, LabeledPrice{Label: label, Amount: amount})
	return b
}

func (b *InvoiceBuilder) Build() []LabeledPrice {
	return b.prices
}