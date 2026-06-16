package models

type AlbumBuilder struct {
	media []any
}

func Album() *AlbumBuilder {
	return &AlbumBuilder{
		media: make([]any, 0),
	}
}

func (b *AlbumBuilder) Photo(path string, caption ...string) *AlbumBuilder {
	p := InputMediaPhoto{
		Type:  "photo",
		Media: path,
	}
	if len(caption) > 0 {
		p.Caption = caption[0]
	}
	b.media = append(b.media, p)
	return b
}

func (b *AlbumBuilder) Video(path string, caption ...string) *AlbumBuilder {
	v := InputMediaVideo{
		Type:  "video",
		Media: path,
	}
	if len(caption) > 0 {
		v.Caption = caption[0]
	}
	b.media = append(b.media, v)
	return b
}

func (b *AlbumBuilder) Build() []any {
	return b.media
}