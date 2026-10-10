package tui

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/style"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
	"github.com/cli/go-gh/v2/pkg/browser"
)

func OpenBrowser(b *browser.Browser, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("invalid browser URL")
	}
	return b.Browse(target)
}

func (a *app) render() error {
	sc := a.program.Screen()
	a.paint(sc)
	if a.showCursor != sc.CursorVisible() {
		var err error
		if a.showCursor {
			err = a.program.ShowCursor()
		} else {
			err = a.program.HideCursor()
		}
		if err != nil {
			return err
		}
	}
	return sc.Render()
}

func (a *app) paint(sc *screen.Screen) {
	buffer.Clear(sc)
	a.showCursor = false
	sc.ClearCursorPosition()
	w, h := sc.Size().Width, sc.Size().Height
	if w < 40 || h < 10 {
		sc.SetString(layout.Pos(0, 0), "Resize to at least 40 columns by 10 rows.", style.New())
		return
	}
	normal, accent, dim, selected, warning := style.New(), style.New().Bold(), style.New().Faint(), style.New().Reverse(), style.New().Bold()
	palette := theme.Colors(a.light)
	panel, divider := normal, dim
	if !a.options.NoColor {
		normal = normal.Foreground(palette.Foreground).Background(palette.Background)
		accent = normal.Foreground(palette.Accent).Bold()
		dim = normal.Foreground(palette.Muted)
		selected = normal.Background(palette.Selection)
		warning = normal.Foreground(palette.Warning).Bold()
		panel = normal.Background(palette.Surface)
		divider = normal.Foreground(palette.Border)
		cell := buffer.BlankCell().WithStyle(normal)
		buffer.Fill(sc, &cell)
	}
	put := func(x, y, width int, value string, st style.Style) {
		if width <= 0 || y < 0 || y >= h {
			return
		}
		text.SetStringRect(sc, layout.Rect(x, y, width, 1), strings.Repeat(" ", width), st)
		text.SetStringRect(sc, layout.Rect(x, y, width, 1), notifs.Plain(value), st)
	}
	put(0, 0, w, "", panel)
	put(1, 0, w-2, "Notifications  │  "+a.client.Host, accent)
	put(0, 1, w, "╭"+strings.Repeat("─", w-2)+"╮", divider)
	queryText := a.query.Raw
	if queryText == "" {
		queryText = "all repositories"
	}
	if a.edit {
		var column int
		queryText, column = queryViewport(sc, a.input, a.caret, w-10)
		a.showCursor = !a.help
		sc.SetCursorPosition(layout.Pos(9+column, 2))
	}
	put(1, 2, w-2, "Search: "+queryText, selected)
	put(0, 2, 1, "│", divider)
	put(w-1, 2, 1, "│", divider)
	put(0, 3, w, "╰"+strings.Repeat("─", w-2)+"╯", divider)
	if a.help {
		lines := []string{
			"Keyboard shortcuts",
			"j/k, arrows       Move through the list or sidebar",
			"/ or s            Edit query; Enter applies; Escape cancels",
			"Tab               Switch between sidebar and inbox",
			"x / Shift+A       Select item / select all loaded items",
			"Enter / o         Open in browser (sidebar: apply filter)",
			"v                 Toggle preview; Ctrl+D/U scroll preview",
			"Shift+I           Mark selected notifications read",
			"e                 Mark done (confirmation required)",
			"Shift+M / Shift+S  Unsubscribe / subscribe (confirmation)",
			"[ / ], PgUp/PgDn  Previous / next result page",
			"g/G, Home/End     First / last item on this page or sidebar",
			"r / i             Refresh now / return to initial inbox",
			"? / q / Ctrl+C    Help / quit",
			"",
			"Queries: repo:OWNER/REPO org:ORG reason:review-requested",
			"is:read is:unread is:pr is:issue author:@me state:open",
			"since:2026-10-01 before:2026-11-01 \"title text\"",
			"Dates filter notification updates. Terms combine with AND.",
			"Saved, Done history, and mark-unread have no supported API.",
		}
		for i, line := range lines {
			put(2, 4+i, w-4, line, normal)
		}
		return
	}
	left := 0
	if w >= 90 {
		left = 25
		put(left-1, 3, 1, "┬", divider)
		for y := 4; y < h-4; y++ {
			put(left-1, y, 1, "│", divider)
		}
		put(1, 4, left-2, "Views and scopes", dim)
		entries := a.sideEntries()
		a.sideIndex = min(a.sideIndex, len(entries)-1)
		offset := max(0, a.sideIndex-(h-10))
		for i := offset; i < len(entries) && i-offset < h-9; i++ {
			st := normal
			if a.sidebar && i == a.sideIndex {
				st = selected
			}
			put(1, 5+i-offset, left-2, entries[i].label, st)
		}
	} else if a.sidebar {
		entries := a.sideEntries()
		a.sideIndex = min(a.sideIndex, len(entries)-1)
		put(1, 4, w-2, "View: "+entries[a.sideIndex].label+" (Enter applies)", accent)
	}
	listBottom := h - 4
	if a.showPreview && h >= 20 {
		listBottom = h / 2
	}
	start := a.page * a.options.PageSize
	end := min(start+a.options.PageSize, len(a.result.Items))
	pages := max(1, (len(a.result.Items)+a.options.PageSize-1)/a.options.PageSize)
	header := fmt.Sprintf("Page %d/%d  │  %d loaded", a.page+1, pages, len(a.result.Items))
	if a.result.More {
		header = fmt.Sprintf("Page %d  │  %d loaded · more available", a.page+1, len(a.result.Items))
	}
	if !(a.sidebar && left == 0) {
		put(left+1, 4, w-left-2, header, dim)
	}
	prevStyle, nextStyle := dim, dim
	if a.page > 0 {
		prevStyle = accent
	}
	if a.hasNextPage() && !a.loading {
		nextStyle = accent
	}
	put(left+1, 5, 15, "[ Previous", prevStyle)
	put(left+16, 5, w-left-17, "Next ]", nextStyle)
	capacity := max(0, (listBottom-6)/2)
	if a.cursor-start < a.scroll {
		a.scroll = max(0, a.cursor-start)
	}
	if a.cursor-start >= a.scroll+capacity {
		a.scroll = a.cursor - start - capacity + 1
	}
	for i := start + a.scroll; i < end && i-start-a.scroll < capacity; i++ {
		item := a.result.Items[i]
		y := 6 + (i-start-a.scroll)*2
		mark, read := "[ ]", " "
		if a.selected[item.ID] {
			mark = "[x]"
		}
		if item.Unread {
			read = "*"
		}
		st := normal
		if item.Unread {
			st = st.Bold()
		}
		if i == a.cursor && !a.sidebar {
			st = selected
		}
		put(left+1, y, w-left-2, mark+read+" "+item.Title, st)
		meta := fmt.Sprintf("     %s  %s %s  %s  %s", item.Reference(), notifs.TypeName(item.Type), item.State, notifs.ReasonName(item.Reason), age(item.UpdatedAt))
		metaStyle := dim
		if !a.options.NoColor {
			switch item.State {
			case "open":
				metaStyle = normal.Foreground(palette.Success)
			case "merged":
				metaStyle = normal.Foreground(palette.Purple)
			case "closed":
				metaStyle = normal.Foreground(palette.Danger)
			}
		}
		put(left+1, y+1, w-left-2, meta, metaStyle)
	}
	if len(a.result.Items) == 0 {
		message := "No notifications match this query."
		if a.loading {
			message = "Fetching notifications..."
		}
		put(left+1, 6, w-left-2, message, dim)
	}
	if a.showPreview && h >= 20 {
		put(left, listBottom, w-left, strings.Repeat("─", w-left), divider)
		if left > 0 {
			put(left-1, listBottom, 1, "├", divider)
		}
		put(left+1, listBottom+1, w-left-2, "Preview  │  Ctrl+D/U scroll  │  v closes", accent)
		lines, err := a.previewLines(w - left - 2)
		if err != nil {
			a.err = fmt.Errorf("render Markdown preview: %w", err)
			lines = []string{"The preview could not be rendered."}
		}
		a.previewOffset = min(a.previewOffset, max(0, len(lines)-1))
		painter := text.NewPainter(sc)
		for i := a.previewOffset; i < len(lines) && listBottom+2+i-a.previewOffset < h-4; i++ {
			rect := layout.Rect(left+1, listBottom+2+i-a.previewOffset, w-left-2, 1)
			painter.SetStringRect(rect, lines[i], normal)
		}
	}
	status := "Not refreshed yet"
	if !a.result.CheckedAt.IsZero() {
		status = "Refreshed " + age(a.result.CheckedAt) + " (" + a.result.CheckedAt.Local().Format("15:04:05") + ")"
	}
	if a.result.FromCache {
		status = "Cached │ " + status
	}
	if a.loading {
		status += " │ refreshing..."
	} else if a.options.Interval == 0 {
		status += " │ auto refresh off"
	} else if wait := time.Until(a.result.NextPoll); wait > 0 {
		status += " │ refresh in " + wait.Round(time.Second).String()
	}
	if a.acting {
		status += " │ applying action..."
	}
	put(0, h-4, w, strings.Repeat("─", w), divider)
	if left > 0 {
		put(left-1, h-4, 1, "┴", divider)
	}
	put(1, h-3, w-2, status, dim)
	message, st := a.status, normal
	if a.err != nil {
		message, st = "Error: "+a.err.Error(), warning
	}
	if a.pendingAction != "" {
		message, st = fmt.Sprintf("%s %d notification(s)? y confirms; any other key cancels", a.pendingAction, len(a.actionIDs())), warning
	}
	put(1, h-2, w-2, message, st)
	put(1, h-1, w-2, "/ query  r refresh  g/G top/end  [/] pages  ? help  q quit", dim)
}

func queryViewport(sc *screen.Screen, input []rune, caret, width int) (string, int) {
	prefix := string(input[:caret])
	column := sc.StringWidth(prefix)
	start := 0
	if column >= width {
		for cluster, cells := range sc.GraphemeCells(prefix) {
			start += utf8.RuneCountInString(cluster)
			column -= cells
			if column < width {
				break
			}
		}
	}
	return string(input[start:]), column
}

func age(t time.Time) string {
	d := max(time.Duration(0), time.Since(t))
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
