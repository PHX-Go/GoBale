package utils

import (
	"strings"
)

type FormattedText struct {
	sb strings.Builder
}

func Text() *FormattedText {
	return &FormattedText{}
}

func (t *FormattedText) Line(parts ...string) *FormattedText {
	for _, part := range parts {
		t.sb.WriteString(part)
	}
	t.sb.WriteString("\n")
	return t
}

func (t *FormattedText) Build() string {
	return t.sb.String()
}