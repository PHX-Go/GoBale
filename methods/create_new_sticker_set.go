package methods

import "github.com/PHX-Go/GoBale/models"

type CreateNewStickerSet struct {
	UserID  int64                 `json:"user_id"`
	Name    string                `json:"name"`
	Title   string                `json:"title"`
	Sticker []models.InputSticker `json:"sticker"`
}

func (c CreateNewStickerSet) Method() string {
	return "createNewStickerSet"
}

func (c CreateNewStickerSet) Params() any {
	return c
}