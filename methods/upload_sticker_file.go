package methods

type UploadStickerFile struct {
	UserID int64 `json:"user_id"`
}

func (u UploadStickerFile) Method() string {
	return "uploadStickerFile"
}

func (u UploadStickerFile) Params() any {
	return u
}