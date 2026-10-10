package tui

import (
	"strings"

	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

type previewRender struct {
	source  string
	baseURL string
	width   int
	light   bool
	noColor bool
	lines   []string
}

func (a *app) previewLines(width int) ([]string, error) {
	if a.preview == nil {
		return []string{"Fetching preview..."}, nil
	}
	source := "# " + a.preview.Title + "\n\n" + a.preview.Body
	cached := a.markdown
	if cached.source == source && cached.baseURL == a.preview.URL && cached.width == width &&
		cached.light == a.light && cached.noColor == a.options.NoColor && cached.lines != nil {
		return cached.lines, nil
	}
	rendered, err := theme.Render(source, a.preview.URL, width, a.light, a.options.NoColor)
	if err != nil {
		return nil, err
	}
	a.markdown = previewRender{source: source, baseURL: a.preview.URL, width: width, light: a.light,
		noColor: a.options.NoColor, lines: strings.Split(strings.TrimRight(rendered, "\n"), "\n")}
	return a.markdown.lines, nil
}
