package methods

import "github.com/PHX-Go/GoBale/models"

type AddStickerToSet struct {
	UserID  int64               `json:"user_id"`
	Name    string              `json:"name"`
	Sticker models.InputSticker `json:"sticker"`
}

func (a AddStickerToSet) Method() string {
	return "addStickerToSet"
}

func (a AddStickerToSet) Params() any {
	return a
}