package methods

type SendLocation struct {
	ChatID             any     `json:"chat_id"`
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	HorizontalAccuracy float64 `json:"horizontal_accuracy,omitempty"`
	ReplyToMessageID   int64   `json:"reply_to_message_id,omitempty"`
	ReplyMarkup        any     `json:"reply_markup,omitempty"`
}

func (s SendLocation) Method() string {
	return "sendLocation"
}

func (s SendLocation) Params() any {
	return s
}