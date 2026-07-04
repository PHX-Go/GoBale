package gobale

import (
	"fmt"
)

// NewKeyboardButton creates a simple reply button
func NewKeyboardButton(text string) KeyboardButton {
	return KeyboardButton{Text: text}
}

// NewContactButton creates a reply button requesting contact
func NewContactButton(text string) KeyboardButton {
	return KeyboardButton{Text: text, RequestContact: true}
}

// NewLocationButton creates a reply button requesting location
func NewLocationButton(text string) KeyboardButton {
	return KeyboardButton{Text: text, RequestLocation: true}
}

// NewReplyKeyboardMarkup creates a ReplyKeyboardMarkup
func NewReplyKeyboardMarkup(rows ...[]KeyboardButton) *ReplyKeyboardMarkup {
	return &ReplyKeyboardMarkup{
		Keyboard:       rows,
		ResizeKeyboard: true,
	}
}

// NewInlineKeyboardButtonURL creates an inline url button
func NewInlineKeyboardButtonURL(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, URL: url}
}

// NewInlineKeyboardButtonData creates an inline callback button
func NewInlineKeyboardButtonData(text, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, CallbackData: callbackData}
}

// NewInlineKeyboardMarkup creates an InlineKeyboardMarkup
func NewInlineKeyboardMarkup(rows ...[]InlineKeyboardButton) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// NewWebAppButton creates a WebApp reply button
func NewWebAppButton(text string, url string) KeyboardButton {
	return KeyboardButton{
		Text:   text,
		WebApp: &WebAppInfo{URL: url},
	}
}

// NewInlineKeyboardButtonWebApp creates an inline WebApp button
func NewInlineKeyboardButtonWebApp(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:   text,
		WebApp: &WebAppInfo{URL: url},
	}
}

// NewInlineKeyboardButtonCopy creates an inline copy button
func NewInlineKeyboardButtonCopy(text, textToCopy string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:     text,
		CopyText: &CopyTextButton{Text: textToCopy},
	}
}

// NewReplyKeyboardRemove creates a reply keyboard remover
func NewReplyKeyboardRemove() *ReplyKeyboardRemove {
	return &ReplyKeyboardRemove{
		RemoveKeyboard: true,
	}
}

// InlineButtonBuilder handles fluent inline button generation
type InlineButtonBuilder struct {
	btn InlineKeyboardButton
}

// Btn initiates a fluent inline button builder
func Btn(text string) *InlineButtonBuilder {
	return &InlineButtonBuilder{
		btn: InlineKeyboardButton{
			Text:         text,
			CallbackData: text,
		},
	}
}

// Callback registers callback data for the inline button
func (b *InlineButtonBuilder) Callback(data string) *InlineButtonBuilder {
	b.btn.CallbackData = data
	return b
}

// URL registers redirection URL for the inline button
func (b *InlineButtonBuilder) URL(link string) *InlineButtonBuilder {
	b.btn.URL = link
	b.btn.CallbackData = ""
	return b
}

// Copy registers copy text parameters for the inline button
func (b *InlineButtonBuilder) Copy(textToCopy string) *InlineButtonBuilder {
	b.btn.CopyText = &CopyTextButton{Text: textToCopy}
	b.btn.CallbackData = ""
	return b
}

// WebApp registers WebApp URL parameters for the inline button
func (b *InlineButtonBuilder) WebApp(url string) *InlineButtonBuilder {
	b.btn.WebApp = &WebAppInfo{URL: url}
	b.btn.CallbackData = ""
	return b
}

// Build returns the finalized InlineKeyboardButton
func (b *InlineButtonBuilder) Build() InlineKeyboardButton {
	return b.btn
}

// ReplyButtonBuilder handles fluent reply button generation
type ReplyButtonBuilder struct {
	btn KeyboardButton
}

// ReplyBtn initiates a fluent reply button builder
func ReplyBtn(text string) *ReplyButtonBuilder {
	return &ReplyButtonBuilder{
		btn: KeyboardButton{Text: text},
	}
}

// Contact configures the button to request user phone directory
func (b *ReplyButtonBuilder) Contact() *ReplyButtonBuilder {
	b.btn.RequestContact = true
	return b
}

// Location configures the button to request user geo-coordinates
func (b *ReplyButtonBuilder) Location() *ReplyButtonBuilder {
	b.btn.RequestLocation = true
	return b
}

// Build returns the finalized KeyboardButton
func (b *ReplyButtonBuilder) Build() KeyboardButton {
	return b.btn
}

// InlineMarkupBuilder handles fluent inline keyboard generation
type InlineMarkupBuilder struct {
	markup *InlineKeyboardMarkup
}

// InlineMarkup initiates a fluent inline keyboard builder
func InlineMarkup() *InlineMarkupBuilder {
	return &InlineMarkupBuilder{
		markup: &InlineKeyboardMarkup{
			InlineKeyboard: make([][]InlineKeyboardButton, 0),
		},
	}
}

// Row appends a single row of buttons containing mixed elements
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

// Build returns the finalized InlineKeyboardMarkup
func (b *InlineMarkupBuilder) Build() *InlineKeyboardMarkup {
	return b.markup
}

// ReplyMarkupBuilder handles fluent reply keyboard generation
type ReplyMarkupBuilder struct {
	markup *ReplyKeyboardMarkup
}

// ReplyMarkup initiates a fluent reply keyboard builder
func ReplyMarkup() *ReplyMarkupBuilder {
	return &ReplyMarkupBuilder{
		markup: &ReplyKeyboardMarkup{
			Keyboard:       make([][]KeyboardButton, 0),
			ResizeKeyboard: true,
		},
	}
}

// Row appends a single row of buttons containing mixed elements
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

// OneTime configures if reply keyboard hides after use
func (b *ReplyMarkupBuilder) OneTime(val bool) *ReplyMarkupBuilder {
	b.markup.OneTimeKeyboard = val
	return b
}

// Build returns the finalized ReplyKeyboardMarkup
func (b *ReplyMarkupBuilder) Build() *ReplyKeyboardMarkup {
	return b.markup
}

// NewReplyKeyboardMarkupFromSlice converts a 1D string array to reply keyboard matrix
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

// NewInlineKeyboardMarkupFromSlice converts a 1D string array to inline keyboard matrix with prefixes
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

// BtnMap generates sequential inline buttons using format patterns dynamically
func BtnMap(textFormat, callbackFormat string, start, end int) []InlineKeyboardButton {
	var buttons []InlineKeyboardButton
	for i := start; i <= end; i++ {
		t := fmt.Sprintf(textFormat, i)
		d := fmt.Sprintf(callbackFormat, i)
		buttons = append(buttons, Btn(t).Callback(d).Build())
	}
	return buttons
}

// BtnList maps matching arrays of labels and callbacks into inline buttons
func BtnList(texts, callbacks []string) []InlineKeyboardButton {
	var buttons []InlineKeyboardButton
	limit := len(texts)
	if len(callbacks) < limit {
		limit = len(callbacks)
	}
	for i := 0; i < limit; i++ {
		buttons = append(buttons, Btn(texts[i]).Callback(callbacks[i]).Build())
	}
	return buttons
}

// WebApp registers WebApp URL parameters for the reply keyboard button fluidly
func (b *ReplyButtonBuilder) WebApp(url string) *ReplyButtonBuilder {
	b.btn.WebApp = &WebAppInfo{URL: url}
	return b
}
