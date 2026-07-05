package gobale

import (
	"context"
	"errors"
)

// PayChain provides payment services using the unified dot system
type PayChain struct {
	bot  *Bot
	ctx  context.Context
	chat any
}

// Pay opens the billing management dot chain from the Bot context
func (b *Bot) Pay(chat any) *PayChain {
	return &PayChain{
		bot:  b,
		ctx:  context.Background(),
		chat: chat,
	}
}

// Pay opens the billing management dot chain from the Handler context
func (c *Ctx) Pay() *PayChain {
	id, _ := c.ChatID()
	return &PayChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		chat: id,
	}
}

// Invoice initializes an invoice fluent delivery chain
func (p *PayChain) Invoice(title, desc, payload, providerToken string) *InvoiceChain {
	return &InvoiceChain{
		pc:      p,
		title:   title,
		desc:    desc,
		payload: payload,
		token:   providerToken,
	}
}

// InvoiceChain holds parameters for a single invoice delivery with advanced e-commerce fields
type InvoiceChain struct {
	pc           *PayChain
	title        string
	desc         string
	payload      string
	token        string
	prices       []LabeledPrice
	replyTo      int64
	markup       any
	providerData string
	photoURL     string
	photoSize    int
	photoWidth   int
	photoHeight  int
	needName     bool
	needPhone    bool
	needEmail    bool
	needShipping bool
	isFlexible   bool
	disableNotif bool
}

// Price appends a labeled price item directly into the invoice list
func (i *InvoiceChain) Price(label string, amount int64) *InvoiceChain {
	i.prices = append(i.prices, LabeledPrice{
		Label:  label,
		Amount: amount,
	})
	return i
}

// Reply sets the reply target message ID
func (i *InvoiceChain) Reply(id int64) *InvoiceChain {
	i.replyTo = id
	return i
}

// Markup appends keyboard markup structure to the invoice message
func (i *InvoiceChain) Markup(m any) *InvoiceChain {
	i.markup = m
	return i
}

// ProviderData sets custom provider data for the invoice
func (i *InvoiceChain) ProviderData(data string) *InvoiceChain {
	i.providerData = data
	return i
}

// PhotoURL sets dynamic product photo URL for the invoice
func (i *InvoiceChain) PhotoURL(url string) *InvoiceChain {
	i.photoURL = url
	return i
}

// PhotoSize sets dynamic product photo size for the invoice
func (i *InvoiceChain) PhotoSize(size int) *InvoiceChain {
	i.photoSize = size
	return i
}

// PhotoWidth sets dynamic product photo width for the invoice
func (i *InvoiceChain) PhotoWidth(width int) *InvoiceChain {
	i.photoWidth = width
	return i
}

// PhotoHeight sets dynamic product photo height for the invoice
func (i *InvoiceChain) PhotoHeight(height int) *InvoiceChain {
	i.photoHeight = height
	return i
}

// NeedName configures if the buyer's full name is required to complete the order
func (i *InvoiceChain) NeedName(val bool) *InvoiceChain {
	i.needName = val
	return i
}

// NeedPhone configures if the buyer's phone number is required to complete the order
func (i *InvoiceChain) NeedPhone(val bool) *InvoiceChain {
	i.needPhone = val
	return i
}

// NeedEmail configures if the buyer's email address is required to complete the order
func (i *InvoiceChain) NeedEmail(val bool) *InvoiceChain {
	i.needEmail = val
	return i
}

// NeedShipping configures if the buyer's shipping address is required to complete the order
func (i *InvoiceChain) NeedShipping(val bool) *InvoiceChain {
	i.needShipping = val
	return i
}

// IsFlexible configures if the final price depends on the shipping method
func (i *InvoiceChain) IsFlexible(val bool) *InvoiceChain {
	i.isFlexible = val
	return i
}

// DisableNotification configures silent invoice delivery without sound alert
func (i *InvoiceChain) DisableNotification(val bool) *InvoiceChain {
	i.disableNotif = val
	return i
}

// Go executes the invoice sending process with auto error logging
func (i *InvoiceChain) Go() (*Message, error) {
	if len(i.prices) == 0 {
		return nil, errors.New("invoice must have prices")
	}
	resolved := i.pc.bot.ResolveChatID(i.pc.chat)
	var msg Message

	payload := map[string]any{
		"chat_id":               resolved,
		"title":                 i.title,
		"description":           i.desc,
		"payload":               i.payload,
		"provider_token":        i.token,
		"start_parameter":       "pay",
		"currency":              "IRR",
		"prices":                i.prices,
		"reply_to_message_id":   i.replyTo,
		"reply_markup":          i.markup,
		"need_name":             i.needName,
		"need_phone_number":     i.needPhone,
		"need_email":            i.needEmail,
		"need_shipping_address": i.needShipping,
		"is_flexible":           i.isFlexible,
		"disable_notification":  i.disableNotif,
	}

	if i.providerData != "" {
		payload["provider_data"] = i.providerData
	}
	if i.photoURL != "" {
		payload["photo_url"] = i.photoURL
	}
	if i.photoSize > 0 {
		payload["photo_size"] = i.photoSize
	}
	if i.photoWidth > 0 {
		payload["photo_width"] = i.photoWidth
	}
	if i.photoHeight > 0 {
		payload["photo_height"] = i.photoHeight
	}

	err := i.pc.bot.BaseRequest(i.pc.ctx, "sendInvoice", payload, &msg)
	if err != nil {
		logErr(i.pc.bot, "[Invoice Send Error] ", err)
	}
	return &msg, err
}

// Link initializes a fluent invoice link creation chain
func (p *PayChain) Link(title, desc, payload, providerToken string) *InvoiceLinkChain {
	return &InvoiceLinkChain{
		pc:      p,
		title:   title,
		desc:    desc,
		payload: payload,
		token:   providerToken,
	}
}

// InvoiceLinkChain holds parameters for creating a payment gateway url
type InvoiceLinkChain struct {
	pc      *PayChain
	title   string
	desc    string
	payload string
	token   string
	prices  []LabeledPrice
}

// Price appends a labeled price item to the gateway link generator
func (il *InvoiceLinkChain) Price(label string, amount int64) *InvoiceLinkChain {
	il.prices = append(il.prices, LabeledPrice{
		Label:  label,
		Amount: amount,
	})
	return il
}

// Go executes the creation of payment link with auto error logging
func (il *InvoiceLinkChain) Go() (string, error) {
	if len(il.prices) == 0 {
		return "", errors.New("invoice must have prices")
	}
	var link string
	err := il.pc.bot.BaseRequest(il.pc.ctx, "createInvoiceLink", map[string]any{
		"title":          il.title,
		"description":    il.desc,
		"payload":        il.payload,
		"provider_token": il.token,
		"currency":       "IRR",
		"prices":         il.prices,
	}, &link)
	if err != nil {
		logErr(il.pc.bot, "[Invoice Link Create Error] ", err)
	}
	return link, err
}

// PreCheckout answers pre checkout transaction requests fluidly
func (p *PayChain) PreCheckout(id ...string) *PreCheckoutChain {
	queryID := ""
	if len(id) > 0 {
		queryID = id[0]
	}
	return &PreCheckoutChain{
		pc: p,
		id: queryID,
	}
}

// PreCheckout answers pre checkout transaction requests fluidly inside context
func (c *Ctx) PreCheckout() *PreCheckoutChain {
	id := ""
	if c.Update != nil && c.Update.PreCheckoutQuery != nil {
		id = c.Update.PreCheckoutQuery.ID
	}
	return &PreCheckoutChain{
		pc: &PayChain{bot: c.Bot, ctx: c.ctx},
		id: id,
	}
}

// PreCheckoutChain holds parameters for pre checkout validation replies
type PreCheckoutChain struct {
	pc  *PayChain
	id  string
	ok  bool
	err string
}

// OK sets the success state of the checkout validation
func (p *PreCheckoutChain) OK(val bool) *PreCheckoutChain {
	p.ok = val
	return p
}

// Error appends failure reasons to the payment gateway response
func (p *PreCheckoutChain) Error(msg string) *PreCheckoutChain {
	p.err = msg
	return p
}

// Go executes the pre checkout query resolution with auto error logging
func (p *PreCheckoutChain) Go() (bool, error) {
	if p.id == "" {
		return false, errors.New("missing pre_checkout ID")
	}
	var res bool
	err := p.pc.bot.BaseRequest(p.pc.ctx, "answerPreCheckoutQuery", map[string]any{
		"pre_checkout_query_id": p.id,
		"ok":                    p.ok,
		"error_message":         p.err,
	}, &res)
	if err != nil {
		logErr(p.pc.bot, "[PreCheckout Answer Error] ", err)
	}
	return res, err
}

// Tx initiates a transaction retrieval chain
func (p *PayChain) Tx(transactionID string) *TxChain {
	return &TxChain{pc: p, id: transactionID}
}

// TxChain handles fluent transaction retrieval
type TxChain struct {
	pc *PayChain
	id string
}

// Go executes transaction info retrieval and returns Transaction with auto error logging
func (t *TxChain) Go() (*Transaction, error) {
	if t.id == "" {
		return nil, errors.New("missing transaction ID")
	}
	var tx Transaction
	err := t.pc.bot.BaseRequest(t.pc.ctx, "getTransaction", map[string]any{
		"transaction_id": t.id,
	}, &tx)
	if err != nil {
		logErr(t.pc.bot, "[Transaction Query Error] ", err)
	}
	return &tx, err
}

// InquireTx initiates a transaction inquiry chain
func (p *PayChain) InquireTx(transactionID string) *InquireTxChain {
	return &InquireTxChain{pc: p, id: transactionID}
}

// InquireTxChain handles fluent transaction inquiry
type InquireTxChain struct {
	pc *PayChain
	id string
}

// Go executes inquiry on Bale servers and returns Transaction with auto error logging
func (i *InquireTxChain) Go() (*Transaction, error) {
	if i.id == "" {
		return nil, errors.New("missing transaction ID")
	}
	var tx Transaction
	err := i.pc.bot.BaseRequest(i.pc.ctx, "inquireTransaction", map[string]any{
		"transaction_id": i.id,
	}, &tx)
	if err != nil {
		logErr(i.pc.bot, "[Transaction Inquire Error] ", err)
	}
	return &tx, err
}
