package tui

import (
	"fmt"
	"strings"

	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/style"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

type previewRender struct {
	source  string
	baseURL string
	width   int
	light   bool
	noColor bool
	lines   []string
	err     error
}

func (a *app) previewLines(width int) ([]string, error) {
	if a.current() == nil {
		return []string{"No notification selected."}, nil
	}
	if a.preview == nil {
		return []string{"Fetching preview..."}, nil
	}
	body := *a.preview
	body.Metadata = nil
	return a.renderPreviewMarkdown(body.Markdown(), width, &a.markdown)
}

func (a *app) metadataLines(width int) ([]string, error) {
	if a.current() == nil {
		return []string{"No notification selected."}, nil
	}
	if a.preview == nil {
		return []string{"Fetching details..."}, nil
	}
	groups := make([][]notifs.PreviewSection, 5)
	for _, section := range a.preview.Metadata {
		if section.Total == 0 && len(section.Items) == 0 {
			continue
		}
		group := 0
		switch {
		case strings.HasPrefix(section.Name, "Review"):
			group = 1
		case strings.HasPrefix(section.Name, "Checks"):
			group = 2
		case section.Name == "Branches" || section.Name == "Development" || section.Name == "Milestone":
			group = 3
		case strings.HasPrefix(section.Name, "Deployments"):
			group = 4
		}
		groups[group] = append(groups[group], section)
	}
	var source strings.Builder
	for i, name := range []string{"People", "Reviews", "Checks", "Development", "Deployments"} {
		if len(groups[i]) > 0 {
			section := notifs.Preview{Title: name, Metadata: groups[i]}
			source.WriteString(section.Markdown())
		}
	}
	if source.Len() == 0 {
		source.WriteString("No additional details.")
	}
	return a.renderPreviewMarkdown(source.String(), width, &a.metadataMarkdown)
}

func (a *app) renderPreviewMarkdown(source string, width int, cache *previewRender) ([]string, error) {
	baseURL := ""
	if current := a.current(); current != nil {
		baseURL = current.URL
	}
	if a.preview != nil && a.preview.URL != "" {
		baseURL = a.preview.URL
	}
	cached := *cache
	if cached.source == source && cached.baseURL == baseURL && cached.width == width &&
		cached.light == a.light && cached.noColor == a.options.NoColor && (cached.lines != nil || cached.err != nil) {
		return cached.lines, cached.err
	}
	rendered, err := theme.Render(source, baseURL, width, a.light, a.options.NoColor)
	*cache = previewRender{source: source, baseURL: baseURL, width: width, light: a.light,
		noColor: a.options.NoColor, err: err}
	if err != nil {
		a.queueError(fmt.Errorf("render preview: %w", err))
		return nil, err
	}
	cache.lines = strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	return cache.lines, nil
}

func (a *app) toggleMetadata() {
	if a.previewNarrow {
		a.metadataHidden = false
		a.metadataFocused = !a.metadataFocused
	} else {
		a.metadataHidden = !a.metadataHidden
		a.metadataFocused = !a.metadataHidden
	}
}

func (a *app) scrollPreview(delta int) {
	if a.metadataFocused && !a.metadataHidden {
		a.metadataOffset = max(0, a.metadataOffset+delta)
	} else {
		offset := a.contentOffset()
		*offset = max(0, *offset+delta)
	}
}

func (a *app) paintPreview(sc *screen.Screen, bounds layout.Rectangle) {
	a.previewBounds = bounds
	a.previewNarrow = bounds.Dx() < 62
	if a.metadataWidth == 0 {
		a.metadataWidth = 32
	}
	p := theme.Colors(a.light)
	normal, panel, heading, dim, divider := style.New(), style.New(), style.New().Bold(), style.New().Faint(), style.New().Faint()
	if !a.options.NoColor {
		normal = normal.Foreground(p.Foreground).Background(p.Background)
		panel = normal.Background(p.Surface)
		heading = panel.Foreground(p.Purple).Bold()
		dim = panel.Foreground(p.Muted)
		divider = normal.Foreground(p.Border)
	}
	put := func(rect layout.Rectangle, value string, st style.Style) {
		text.SetStringRect(sc, rect, strings.Repeat(" ", rect.Dx()), st)
		text.SetStringRectTruncate(sc, rect, notifs.Plain(value), st, "…", st)
	}
	y := bounds.Min.Y
	put(layout.Rect(bounds.Min.X, y, bounds.Dx(), 1), "", panel)
	body := layout.Rect(bounds.Min.X+1, y+1, bounds.Dx()-2, bounds.Dy()-1)
	details := layout.Rectangle{}
	if !a.previewNarrow && !a.metadataHidden {
		width := max(24, min(a.metadataWidth, bounds.Dx()-38))
		x := bounds.Max.X - width - 1
		details = layout.Rect(x+1, y+1, width-1, bounds.Dy()-1)
		body.Max.X = x - 1
		for row := y; row < bounds.Max.Y; row++ {
			put(layout.Rect(x, row, 1, 1), "│", divider)
		}
		a.target(layout.Rect(x, y, 1, bounds.Dy()), "metadata-divider", 0)
	} else if a.previewNarrow && a.metadataFocused {
		details, body = body, layout.Rectangle{}
	}
	label := "Preview"
	if a.previewNarrow {
		label = "Body"
	}
	labelStyle := heading
	if a.metadataFocused {
		labelStyle = dim
	}
	put(layout.Rect(bounds.Min.X+1, y, 7, 1), label, labelStyle)
	a.target(layout.Rect(bounds.Min.X+1, y, 7, 1), "preview-body", 0)
	toggle := "metadata"
	if !a.previewNarrow && !a.metadataHidden {
		toggle = "hide metadata"
	}
	toggleWidth := sc.StringWidth(toggle)
	toggleX := bounds.Max.X - toggleWidth - 1
	toggleStyle := dim
	if a.metadataFocused {
		toggleStyle = heading
	}
	put(layout.Rect(toggleX, y, toggleWidth, 1), toggle, toggleStyle)
	underlineShortcut(sc, toggleX, y, toggle, 'm', toggleStyle)
	a.target(layout.Rect(toggleX, y, toggleWidth, 1), "metadata-toggle", 0)
	hintWidth := toggleX - bounds.Min.X - 10
	if !body.Empty() {
		hintWidth = min(hintWidth, body.Max.X-bounds.Min.X-10)
	}
	if hintWidth >= 19 {
		put(layout.Rect(bounds.Min.X+9, y, hintWidth, 1), "Ctrl+D/U scroll", dim)
	}
	if !body.Empty() {
		x := body.Min.X
		for tab, label := range []string{" 1 Body ", " 2 Discussion ", " 3 Threads "} {
			width := sc.StringWidth(label)
			if x+width > body.Max.X {
				break
			}
			st := dim
			if tab == a.previewTab {
				st = heading
				if !a.options.NoColor {
					st = st.Background(p.Selection)
				} else {
					st = st.Reverse()
				}
			}
			rect := layout.Rect(x, body.Min.Y, width, 1)
			put(rect, label, st)
			if supportsDiscussion(a.current(), tab) {
				a.target(rect, "preview-tab", tab)
			}
			x += width + 1
		}
		body.Min.Y++
		if a.previewTab > 0 && body.Dy() > 1 {
			state := &a.discussions[a.previewTab-1]
			row := body.Max.Y - 1
			total := 0
			if len(state.pages) > 0 {
				total = state.pages[len(state.pages)-1].Total
			}
			put(layout.Rect(body.Min.X, row, body.Dx(), 1), "", dim)
			put(layout.Rect(body.Min.X, row, body.Dx()-15, 1), fmt.Sprintf("Page %d · %d entries", state.page+1, total), dim)
			for i, control := range []struct {
				label, kind string
				enabled     bool
			}{
				{", Prev", "discussion-prev", state.page > 0},
				{". Next", "discussion-next", !state.loading && state.page < len(state.pages) && state.pages[state.page].More},
			} {
				rect := layout.Rect(body.Max.X-14+i*7, row, 6, 1)
				st := dim
				if control.enabled {
					st = heading
					a.target(rect, control.kind, 0)
				}
				put(rect, control.label, st)
			}
			body.Max.Y--
		}
	}
	paintPane := func(rect layout.Rectangle, metadata bool) {
		if rect.Empty() {
			return
		}
		kind, offset := "preview", a.contentOffset()
		var lines []string
		var err error
		if metadata {
			kind, offset = "metadata", &a.metadataOffset
			lines, err = a.metadataLines(rect.Dx())
		} else {
			if a.previewTab > 0 {
				lines, err = a.discussionLines(rect.Dx())
			} else {
				lines, err = a.previewLines(rect.Dx())
			}
		}
		if err != nil {
			lines = []string{"This pane could not be rendered."}
		}
		a.target(rect, kind, 0)
		*offset = min(*offset, max(0, len(lines)-rect.Dy()))
		painter := text.NewPainter(sc)
		for i := *offset; i < len(lines) && i-*offset < rect.Dy(); i++ {
			painter.SetStringRect(layout.Rect(rect.Min.X, rect.Min.Y+i-*offset, rect.Dx(), 1), lines[i], normal)
		}
	}
	paintPane(body, false)
	paintPane(details, true)
}
