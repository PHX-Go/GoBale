package methods

type GetMe struct{}

func (g GetMe) Method() string {
	return "getMe"
}

func (g GetMe) Params() any {
	return nil
}