package methods

type SendContact struct {
	ChatID           any    `json:"chat_id"`
	PhoneNumber      any    `json:"phone_number"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name,omitempty"`
	ReplyToMessageID int64  `json:"reply_to_message_id,omitempty"`
	ReplyMarkup      any    `json:"reply_markup,omitempty"`
}

func (s SendContact) Method() string {
	return "sendContact"
}

func (s SendContact) Params() any {
	return s
}