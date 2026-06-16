package methods

type SendMediaGroup struct {
	ChatID           any `json:"chat_id"`
	Media            any   `json:"media"`
	ReplyToMessageID int64 `json:"reply_to_message_id,omitempty"`
}

func (s SendMediaGroup) Method() string {
	return "sendMediaGroup"
}

func (s SendMediaGroup) Params() any {
	return s
}