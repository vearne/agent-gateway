package weixin

import (
	"regexp"
	"strings"
)

var (
	reImage     = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	reH5H6     = regexp.MustCompile(`(?m)^#{5,6}\s*`)
	reCJKItalic = regexp.MustCompile(`(\p{Han}|\p{Hangul})\*([^*]+)\*(\p{Han}|\p{Hangul})`)
)

type StreamingMarkdownFilter struct {
	buf strings.Builder
}

func (f *StreamingMarkdownFilter) Feed(delta string) string {
	f.buf.WriteString(delta)
	text := f.buf.String()
	filtered := filterMarkdownString(text)
	return filtered
}

func (f *StreamingMarkdownFilter) Flush() string {
	text := f.buf.String()
	f.buf.Reset()
	return filterMarkdownString(text)
}

func filterMarkdownString(text string) string {
	text = reImage.ReplaceAllString(text, "")
	text = reH5H6.ReplaceAllString(text, "")
	text = reCJKItalic.ReplaceAllString(text, "$1$2$3")
	return strings.TrimSpace(text)
}
