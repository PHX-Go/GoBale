package models

import (
	"fmt"
)

func NewKeyboardButton(text string) KeyboardButton {
	return KeyboardButton{Text: text}
}

func NewContactButton(text string) KeyboardButton {
	return KeyboardButton{Text: text, RequestContact: true}
}

func NewLocationButton(text string) KeyboardButton {
	return KeyboardButton{Text: text, RequestLocation: true}
}

func NewReplyKeyboardMarkup(rows ...[]KeyboardButton) *ReplyKeyboardMarkup {
	return &ReplyKeyboardMarkup{
		Keyboard:       rows,
		ResizeKeyboard: true,
	}
}

func NewInlineKeyboardButtonURL(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, URL: url}
}

func NewInlineKeyboardButtonData(text, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, CallbackData: callbackData}
}

func NewInlineKeyboardMarkup(rows ...[]InlineKeyboardButton) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

func NewWebAppButton(text string, url string) KeyboardButton {
	return KeyboardButton{
		Text:   text,
		WebApp: &WebAppInfo{URL: url},
	}
}

func NewInlineKeyboardButtonWebApp(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:   text,
		WebApp: &WebAppInfo{URL: url},
	}
}

func NewInlineKeyboardButtonCopy(text, textToCopy string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:     text,
		CopyText: &CopyTextButton{Text: textToCopy},
	}
}

func NewReplyKeyboardRemove() *ReplyKeyboardRemove {
	return &ReplyKeyboardRemove{
		RemoveKeyboard: true,
	}
}

type InlineButtonBuilder struct {
	btn InlineKeyboardButton
}

func Btn(text string) *InlineButtonBuilder {
	return &InlineButtonBuilder{
		btn: InlineKeyboardButton{
			Text:         text,
			CallbackData: text,
		},
	}
}

func (b *InlineButtonBuilder) Callback(data string) *InlineButtonBuilder {
	b.btn.CallbackData = data
	return b
}

func (b *InlineButtonBuilder) URL(link string) *InlineButtonBuilder {
	b.btn.URL = link
	b.btn.CallbackData = ""
	return b
}

func (b *InlineButtonBuilder) Copy(textToCopy string) *InlineButtonBuilder {
	b.btn.CopyText = &CopyTextButton{Text: textToCopy}
	b.btn.CallbackData = ""
	return b
}

func (b *InlineButtonBuilder) WebApp(url string) *InlineButtonBuilder {
	b.btn.WebApp = &WebAppInfo{URL: url}
	b.btn.CallbackData = ""
	return b
}

type ReplyButtonBuilder struct {
	btn KeyboardButton
}

func ReplyBtn(text string) *ReplyButtonBuilder {
	return &ReplyButtonBuilder{
		btn: KeyboardButton{Text: text},
	}
}

func (b *ReplyButtonBuilder) Contact() *ReplyButtonBuilder {
	b.btn.RequestContact = true
	return b
}

func (b *ReplyButtonBuilder) Location() *ReplyButtonBuilder {
	b.btn.RequestLocation = true
	return b
}

type InlineMarkupBuilder struct {
	markup *InlineKeyboardMarkup
}

func InlineMarkup() *InlineMarkupBuilder {
	return &InlineMarkupBuilder{
		markup: &InlineKeyboardMarkup{
			InlineKeyboard: make([][]InlineKeyboardButton, 0),
		},
	}
}

func (b *InlineMarkupBuilder) Row(buttons ...any) *InlineMarkupBuilder {
	var row []InlineKeyboardButton
	for _, item := range buttons {
		switch val := item.(type) {
		case string:
			row = append(row, NewInlineKeyboardButtonData(val, val))
		case *InlineButtonBuilder:
			row = append(row, val.btn)
		case InlineKeyboardButton:
			row = append(row, val)
		}
	}
	b.markup.InlineKeyboard = append(b.markup.InlineKeyboard, row)
	return b
}

func (b *InlineMarkupBuilder) Build() *InlineKeyboardMarkup {
	return b.markup
}

type ReplyMarkupBuilder struct {
	markup *ReplyKeyboardMarkup
}

func ReplyMarkup() *ReplyMarkupBuilder {
	return &ReplyMarkupBuilder{
		markup: &ReplyKeyboardMarkup{
			Keyboard:       make([][]KeyboardButton, 0),
			ResizeKeyboard: true,
		},
	}
}

func (b *ReplyMarkupBuilder) Row(buttons ...any) *ReplyMarkupBuilder {
	var row []KeyboardButton
	for _, item := range buttons {
		switch val := item.(type) {
		case string:
			row = append(row, NewKeyboardButton(val))
		case *ReplyButtonBuilder:
			row = append(row, val.btn)
		case KeyboardButton:
			row = append(row, val)
		}
	}
	b.markup.Keyboard = append(b.markup.Keyboard, row)
	return b
}

func (b *ReplyMarkupBuilder) OneTime(val bool) *ReplyMarkupBuilder {
	b.markup.OneTimeKeyboard = val
	return b
}

func (b *ReplyMarkupBuilder) Build() *ReplyKeyboardMarkup {
	return b.markup
}

func NewPaginatedKeyboard(items []InlineKeyboardButton, page int, itemsPerPage int, callbackPrefix string) *InlineKeyboardMarkup {
	if page < 1 {
		page = 1
	}
	start := (page - 1) * itemsPerPage
	end := start + itemsPerPage
	if start > len(items) {
		return NewInlineKeyboardMarkup()
	}
	if end > len(items) {
		end = len(items)
	}

	var rows [][]InlineKeyboardButton
	for i := start; i < end; i++ {
		rows = append(rows, []InlineKeyboardButton{items[i]})
	}

	var navRow []InlineKeyboardButton
	if page > 1 {
		navRow = append(navRow, NewInlineKeyboardButtonData("⬅️ قبلی", fmt.Sprintf("%s:%d", callbackPrefix, page-1)))
	}
	if end < len(items) {
		navRow = append(navRow, NewInlineKeyboardButtonData("بعدی ➡️", fmt.Sprintf("%s:%d", callbackPrefix, page+1)))
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func NewReplyKeyboardMarkupFromSlice(items []string, cols int) *ReplyKeyboardMarkup {
	if cols <= 0 {
		cols = 1
	}
	var rows [][]KeyboardButton
	var currentRow []KeyboardButton

	for _, item := range items {
		currentRow = append(currentRow, NewKeyboardButton(item))
		if len(currentRow) == cols {
			rows = append(rows, currentRow)
			currentRow = []KeyboardButton{}
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	return &ReplyKeyboardMarkup{
		Keyboard:       rows,
		ResizeKeyboard: true,
	}
}

func NewInlineKeyboardMarkupFromSlice(items []string, cols int, callbackPrefix string) *InlineKeyboardMarkup {
	if cols <= 0 {
		cols = 1
	}
	var rows [][]InlineKeyboardButton
	var currentRow []InlineKeyboardButton

	for _, item := range items {
		var callbackData string
		if callbackPrefix != "" {
			callbackData = fmt.Sprintf("%s:%s", callbackPrefix, item)
		} else {
			callbackData = item
		}

		currentRow = append(currentRow, NewInlineKeyboardButtonData(item, callbackData))
		if len(currentRow) == cols {
			rows = append(rows, currentRow)
			currentRow = []InlineKeyboardButton{}
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	return &InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

func (b *InlineButtonBuilder) Build() InlineKeyboardButton {
	return b.btn
}

func (b *ReplyButtonBuilder) Build() KeyboardButton {
	return b.btn
}