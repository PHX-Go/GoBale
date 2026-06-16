package methods

type GetFile struct {
	FileID string `json:"file_id"`
}

func (g GetFile) Method() string {
	return "getFile"
}

func (g GetFile) Params() any {
	return g
}