package tui

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"altscreen.io/vision/ansi"
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

func underlineShortcut(sc *screen.Screen, x, y int, label string, shortcut rune, st style.Style) {
	for offset, r := range label {
		if r == shortcut {
			text.SetStringRect(sc, layout.Rect(x+sc.StringWidth(label[:offset]), y, sc.StringWidth(string(r)), 1), string(r), st.Underline())
			return
		}
	}
}

func (a *app) windowTitle() string {
	unread := 0
	for _, item := range a.result.Items {
		if item.Unread {
			unread++
		}
	}
	suffix := ""
	if a.result.More {
		suffix = "+"
	}
	return fmt.Sprintf("gh-notifs - %d%s unread", unread, suffix)
}

func (a *app) render() error {
	if !a.options.NoColor {
		background := theme.Colors(a.light).Background
		if a.terminalBG != background {
			if err := a.program.SetBackgroundColor(background); err != nil {
				return fmt.Errorf("set terminal background: %w", err)
			}
			a.terminalBG = background
		}
	}
	title := a.windowTitle()
	if title != a.title {
		if err := a.program.SetTitle(title); err != nil {
			return fmt.Errorf("set terminal title: %w", err)
		}
		a.title = title
	}
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
	a.hits = a.hits[:0]
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
		text.SetStringRectTruncate(sc, layout.Rect(x, y, width, 1), notifs.Plain(value), st, "…", st)
	}
	linkText := func(x, y, width int, value, target string, st style.Style) {
		if width <= 0 {
			return
		}
		if u, err := url.Parse(target); err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && notifs.Plain(target) == target {
			st = st.Link(target, "")
		}
		text.SetStringRectTruncate(sc, layout.Rect(x, y, width, 1), notifs.Plain(value), st, "…", st)
	}
	button := func(x, y, available int, label, action string, enabled bool, shortcut rune) int {
		width := sc.StringWidth(label)
		if width > available {
			return 0
		}
		st := dim
		if enabled {
			st = accent
			a.target(layout.Rect(x, y, width, 1), action, 0)
		}
		put(x, y, width, label, st)
		underlineShortcut(sc, x, y, label, shortcut, st)
		return width + 2
	}
	put(0, 0, w, "", normal)
	titleBar := normal.Bold()
	if !a.options.NoColor {
		titleBar = titleBar.Foreground(palette.Accent)
	}
	put(1, 0, w-2, "Notifications  │  "+a.client.Host, titleBar)
	button(w-12, 0, 12, "Sidebar", "toggle", true, 'b')
	queryBorder, queryStyle := divider, normal
	if a.edit {
		queryBorder = accent
	}
	put(0, 1, w, "╭"+strings.Repeat("─", w-2)+"╮", queryBorder)
	queryText := a.query.Raw
	if queryText == "" {
		queryText = "all repositories"
	}
	if a.edit {
		var column int
		queryText, column = queryViewport(sc, a.input, a.caret, w-10)
		a.showCursor = true
		sc.SetCursorPosition(layout.Pos(9+column, 2))
	}
	put(1, 2, w-2, "Search: "+queryText, queryStyle)
	queryLabel := queryStyle.Bold()
	if !a.options.NoColor {
		queryLabel = queryLabel.Foreground(palette.Cyan)
	}
	put(1, 2, 8, "Search: ", queryLabel)
	a.target(layout.Rect(1, 2, w-2, 1), "query", 0)
	put(0, 2, 1, "│", queryBorder)
	put(w-1, 2, 1, "│", queryBorder)
	put(0, 3, w, "╰"+strings.Repeat("─", w-2)+"╯", queryBorder)
	var helpLines [][]helpPart
	footerTop := h - 4
	if a.help {
		helpLines = helpRows(sc, w-4)
		footerTop = h - min(len(helpLines)+4, max(6, h/2))
	}
	left := 0
	sideBorder := divider
	if a.sidebar {
		sideBorder = accent
	}
	entries := a.sideEntries()
	a.sideIndex = max(0, min(a.sideIndex, len(entries)-1))
	if !a.sideHidden && w >= 64 {
		if a.sideWidth == 0 {
			a.sideWidth = 25
		}
		left = max(18, min(a.sideWidth, w-30))
		a.target(layout.Rect(0, 4, left, footerTop-4), "side", 0)
		a.target(layout.Rect(left-1, 3, 1, footerTop-2), "divider", 0)
		topBorder := queryBorder
		if a.sidebar {
			topBorder = sideBorder
		}
		put(left-1, 3, 1, "┬", topBorder)
		for y := 4; y < footerTop; y++ {
			put(0, y, left-1, "", normal)
			put(left-1, y, 1, "│", sideBorder)
		}
		sideHeading := normal.Bold()
		if !a.options.NoColor {
			sideHeading = sideHeading.Foreground(palette.Muted)
		}
		type sideRow struct {
			label string
			index int
		}
		var rows []sideRow
		if len(a.options.Scopes) > 0 {
			rows = append(rows, sideRow{"Views and scopes", -1})
			for i := range a.options.Scopes {
				rows = append(rows, sideRow{entries[i].label, i})
			}
			rows = append(rows, sideRow{"", -1})
		}
		rows = append(rows, sideRow{"Repositories", -1})
		for i := len(a.options.Scopes); i < len(entries); i++ {
			rows = append(rows, sideRow{entries[i].label, i})
		}
		selectedRow := 0
		for i, row := range rows {
			if row.index == a.sideIndex {
				selectedRow = i
			}
		}
		offset := max(0, selectedRow-(footerTop-4)+1)
		for i := offset; i < len(rows) && i-offset < footerTop-4; i++ {
			row := rows[i]
			y := 4 + i - offset
			if row.index < 0 {
				put(1, y, left-2, row.label, sideHeading)
				continue
			}
			st := normal
			if entries[row.index].repository && !a.options.NoColor {
				st = normal.Foreground(palette.Cyan)
			}
			if a.sidebar && row.index == a.sideIndex {
				if a.options.NoColor {
					st = st.Reverse()
				} else {
					st = st.Background(palette.Selection)
				}
			}
			put(1, y, left-2, row.label, st)
			a.target(layout.Rect(1, y, left-2, 1), "scope", row.index)
		}
	} else if a.sidebar && len(entries) > 0 {
		put(1, 5, w-2, "View: "+entries[a.sideIndex].label+" (Enter applies)", accent)
	}
	listBottom := footerTop
	showPreview := a.showPreview && footerTop >= 16
	if showPreview {
		listBottom = (footerTop + 4) / 2
	}
	a.target(layout.Rect(left, 6, w-left, max(0, listBottom-6)), "list", 0)
	start := a.page * a.options.PageSize
	end := min(start+a.options.PageSize, len(a.result.Items))
	pages := max(1, (len(a.result.Items)+a.options.PageSize-1)/a.options.PageSize)
	header := fmt.Sprintf("Page %d/%d  │  %d loaded", a.page+1, pages, len(a.result.Items))
	if a.result.More {
		header = fmt.Sprintf("Page %d  │  %d loaded · more available", a.page+1, len(a.result.Items))
	}
	const prevLabel, nextLabel = " [ Previous ", " Next ] "
	nextX := w - sc.StringWidth(nextLabel) - 1
	prevX := nextX - sc.StringWidth(prevLabel) - 1
	put(left+1, 4, prevX-left-2, header, dim)
	for _, control := range []struct {
		x       int
		label   string
		action  string
		enabled bool
	}{
		{prevX, prevLabel, "[", a.page > 0},
		{nextX, nextLabel, "]", a.hasNextPage() && !a.loading && !a.acting},
	} {
		width := sc.StringWidth(control.label)
		st := dim
		if !a.options.NoColor {
			st = normal.Foreground(palette.Muted).Background(palette.Surface)
		}
		if control.enabled {
			st = st.Bold()
			if a.options.NoColor {
				st = st.Reverse()
			} else {
				st = st.Foreground(palette.Accent).Background(palette.Selection)
			}
			a.target(layout.Rect(control.x, 4, width, 1), control.action, 0)
		}
		put(control.x, 4, width, control.label, st)
	}
	capacity := max(0, (listBottom-6)/2)
	if a.cursor-start < a.scroll {
		a.scroll = max(0, a.cursor-start)
	}
	if a.cursor-start >= a.scroll+capacity {
		a.scroll = a.cursor - start - capacity + 1
	}
	detailX := left + 9
	detailWidth := w - detailX
	const ageWidth, metadataGap = 11, 2
	authorWidth, reasonWidth := 0, 0
	for _, item := range a.result.Items {
		if item.Author != "" {
			authorWidth = max(authorWidth, sc.StringWidth("@"+notifs.Plain(item.Author)))
		}
		reasonWidth = max(reasonWidth, sc.StringWidth(notifs.ReasonName(item.Reason)))
	}
	reasonWidth = min(reasonWidth, max(0, detailWidth/2-ageWidth-metadataGap))
	ageX := w - ageWidth
	reasonX := ageX - metadataGap - reasonWidth
	referenceEnd := reasonX - metadataGap
	authorWidth = min(authorWidth, max(0, referenceEnd-detailX-14))
	authorX := referenceEnd - authorWidth
	if authorWidth > 0 {
		referenceEnd = authorX - metadataGap
	}
	for i := start + a.scroll; i < end && i-start-a.scroll < capacity; i++ {
		item := a.result.Items[i]
		y := 6 + (i-start-a.scroll)*2
		st := normal
		hovered := item.ID == a.hoverID
		if !a.options.NoColor {
			if hovered {
				st = st.Background(palette.Surface)
			}
			if a.selected[item.ID] {
				st = st.Background(palette.Selected)
			}
		}
		if i == a.cursor && !a.sidebar {
			st = selected
		}
		put(left, y, w-left, "", st)
		metaStyle := st
		if !a.options.NoColor {
			metaStyle = st.Foreground(palette.Muted)
		} else {
			metaStyle = st.Faint()
		}
		put(left, y+1, w-left, "", metaStyle)
		if item.Unread {
			dotStyle := st.Bold()
			if !a.options.NoColor {
				dotStyle = dotStyle.Foreground(palette.Accent)
			}
			put(left+1, y, 1, "●", dotStyle)
		}
		mark, markStyle := "   ", metaStyle
		if a.selected[item.ID] || hovered {
			mark = " ✓ "
			if a.selected[item.ID] || a.hoverSelect {
				markStyle = st.Bold()
				if !a.options.NoColor {
					markStyle = markStyle.Foreground(palette.Accent)
					if a.selected[item.ID] {
						markStyle = markStyle.Foreground(palette.Success)
					}
				}
			}
		}
		put(left+3, y, 3, mark, markStyle)
		iconStyle := metaStyle
		if !a.options.NoColor {
			switch item.State {
			case "open":
				iconStyle = st.Foreground(palette.Success)
			case "merged":
				iconStyle = st.Foreground(palette.Purple)
			case "closed":
				iconStyle = st.Foreground(palette.Danger)
			}
		}
		put(left+7, y, 2, typeIcon(item.Type), iconStyle)
		titleStyle := st
		if item.Unread {
			titleStyle = titleStyle.Bold()
		}
		put(left+9, y, w-left-9, item.Title, titleStyle)

		ageText := age(item.UpdatedAt)
		put(ageX, y+1, ageWidth, strings.Repeat(" ", max(0, ageWidth-sc.StringWidth(ageText)))+ageText, metaStyle)
		if reason := notifs.ReasonName(item.Reason); reason != "" && reasonWidth > 0 {
			reason = ansi.TruncateMode(reason, reasonWidth, "…", sc.WidthMethod(), sc.AmbiguousWide())
			put(reasonX, y+1, reasonWidth, strings.Repeat(" ", max(0, reasonWidth-sc.StringWidth(reason)))+reason, metaStyle)
		}
		referenceWidth := max(0, referenceEnd-detailX)
		if item.Author != "" && authorWidth > 0 {
			author := ansi.TruncateMode("@"+notifs.Plain(item.Author), authorWidth, "…", sc.WidthMethod(), sc.AmbiguousWide())
			put(authorX, y+1, authorWidth, strings.Repeat(" ", max(0, authorWidth-sc.StringWidth(author)))+author, metaStyle)
		}
		suffix := ""
		if item.State != "" && referenceWidth >= sc.StringWidth(item.State)+7 {
			suffix = " · " + item.State
			referenceWidth -= sc.StringWidth(suffix)
		}
		reference := ansi.TruncateMode(notifs.Plain(item.Reference()), referenceWidth, "…", sc.WidthMethod(), sc.AmbiguousWide())
		linkText(detailX, y+1, referenceWidth, reference, item.URL, metaStyle)
		put(detailX+sc.StringWidth(reference), y+1, sc.StringWidth(suffix), suffix, metaStyle)
		a.target(layout.Rect(left, y, w-left, 2), "item", i)
		a.target(layout.Rect(left+3, y, 3, 1), "checkbox", i)
	}
	if len(a.result.Items) == 0 {
		message := "No notifications match this query."
		if a.loading {
			message = "Fetching notifications..."
		}
		put(left+1, 6, w-left-2, message, dim)
	}
	if showPreview {
		put(left, listBottom, w-left, strings.Repeat("─", w-left), divider)
		if left > 0 {
			put(left-1, listBottom, 1, "├", sideBorder)
		}
		a.paintPreview(sc, layout.Rect(left, listBottom+1, w-left, footerTop-listBottom-1))
	}
	status, nextStatus := "Not refreshed yet", ""
	if !a.result.CheckedAt.IsZero() {
		status = "Refreshed " + age(a.result.CheckedAt) + " (" + a.result.CheckedAt.Local().Format("15:04:05") + ")"
	}
	if a.result.FromCache {
		status = "Cached │ " + status
	}
	if a.loading {
		status += " │ refreshing..."
	}
	if a.acting {
		status += " │ applying action..."
	}
	if a.autoRefresh && !a.loading && !a.acting {
		if deadline := a.refreshDeadline(); !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				nextStatus = "Next: waiting"
			} else {
				seconds := remaining / time.Second
				if remaining%time.Second != 0 {
					seconds++
				}
				nextStatus = fmt.Sprintf("Next in %ds (%s)", seconds, deadline.Local().Format("15:04:05"))
			}
			status += " │ " + nextStatus
		}
	}
	put(0, footerTop, w, strings.Repeat("─", w), divider)
	if left > 0 {
		put(left-1, footerTop, 1, "┴", sideBorder)
	}
	if !a.help {
		autoLabel, bellLabel := "R Auto:off", "B Bell:off"
		if a.autoRefresh {
			autoLabel = "R Auto:on"
		}
		if a.bell {
			bellLabel = "B Bell:on"
		}
		x := w - sc.StringWidth(autoLabel) - sc.StringWidth(bellLabel) - 5
		x += button(x, footerTop, w-x, autoLabel, "shift+r", true, 'R')
		button(x, footerTop, w-x, bellLabel, "shift+b", true, 'B')
	}
	statusStyle := dim
	if (a.loading || a.acting) && !a.options.NoColor {
		statusStyle = normal.Foreground(palette.Warning)
	}
	position := "-"
	if a.current() != nil {
		position = fmt.Sprint(a.cursor + 1)
	} else if len(a.result.Items) == 0 {
		position = "0"
	}
	counter := fmt.Sprintf("Item %s/%d", position, len(a.result.Items))
	if a.result.More {
		counter += "+"
	}
	counterWidth := sc.StringWidth(counter)
	counterX := w - counterWidth - 1
	if nextStatus != "" && sc.StringWidth(status) > counterX-3 {
		status = nextStatus
	}
	put(1, footerTop+1, counterX-3, status, statusStyle)
	put(counterX, footerTop+1, counterWidth, counter, dim)
	if a.help {
		a.target(layout.Rect(0, footerTop, w, h-footerTop), "help", 0)
		visible := h - footerTop - 4
		a.helpOffset = min(a.helpOffset, max(0, len(helpLines)-visible))
		label := fmt.Sprintf("Help %d-%d/%d · ↑/↓ or wheel", a.helpOffset+1, a.helpOffset+visible, len(helpLines))
		put(2, footerTop, min(sc.StringWidth(label), w-18), label, accent)
		button(w-14, footerTop, 14, "? Close help", "?", true, 0)
		for i := 0; i < visible && a.helpOffset+i < len(helpLines); i++ {
			y := footerTop + 2 + i
			for _, part := range helpLines[a.helpOffset+i] {
				x := 2 + part.x
				if part.key != "" {
					keyStyle := accent
					if part.keyWidth == 0 && !a.options.NoColor {
						keyStyle = keyStyle.Foreground(palette.Purple)
					}
					put(x, y, sc.StringWidth(part.key), part.key, keyStyle)
				}
				descriptionStyle := dim
				if strings.HasPrefix(part.description, "Try:") && !a.options.NoColor {
					descriptionStyle = normal.Foreground(palette.Cyan)
				}
				put(x+part.keyWidth, y, sc.StringWidth(part.description), part.description, descriptionStyle)
				if part.action != "" {
					a.target(layout.Rect(x, y, part.keyWidth+sc.StringWidth(part.description), 1), part.action, 0)
				}
			}
		}
	}
	message, st := a.status, normal
	messageWidth := w - 2
	if a.err != nil {
		message, st = "Error: "+a.err.Error(), warning
		if !a.options.NoColor {
			st = st.Foreground(palette.Danger)
		}
		label := "Esc dismiss"
		if a.edit || a.help {
			label = "Click dismiss"
		}
		if len(a.pendingErrors) > 0 {
			label += fmt.Sprintf(" (%d)", len(a.pendingErrors)+1)
		}
		labelWidth := sc.StringWidth(label)
		messageWidth -= labelWidth + 1
		put(w-labelWidth-1, h-2, labelWidth, label, accent)
		a.target(layout.Rect(1, h-2, w-2, 1), "dismiss-error", 0)
	}
	put(1, h-2, messageWidth, message, st)
	x := 1
	for _, control := range []helpBinding{
		{"/", "search", "/"},
		{"?", "help", "?"},
		{"q", "quit", "q"},
		{"x", "select", "x"},
		{"o", "open", "o"},
		{"v", "preview", "v"},
		{"r", "refresh", "r"},
		{"e", "done", "e"},
		{"I", "read", "shift+i"},
		{"M", "unsubscribe", "shift+m"},
		{"S", "subscribe", "shift+s"},
	} {
		width := sc.StringWidth(control.key + " " + control.description)
		if width > w-x-1 {
			break
		}
		put(x, h-1, sc.StringWidth(control.key), control.key, accent)
		put(x+sc.StringWidth(control.key)+1, h-1, sc.StringWidth(control.description), control.description, dim)
		a.target(layout.Rect(x, h-1, width, 1), control.action, 0)
		x += width + 3
	}
	suggestions, _, _ := a.suggestions()
	if len(suggestions) > 0 {
		_, column := queryViewport(sc, a.input, a.caret, w-10)
		labelWidth, descriptionWidth := 0, 0
		for _, value := range suggestions {
			labelWidth = max(labelWidth, sc.StringWidth(value))
			descriptionWidth = max(descriptionWidth, sc.StringWidth(completionDescription(value)))
		}
		count := min(8, h-8, len(suggestions))
		popupWidth := max(24, labelWidth+descriptionWidth+4)
		if len(suggestions) > count {
			popupWidth++
		}
		popupWidth = min(popupWidth, w-2)
		popupX := min(9+column, w-popupWidth-1)
		offset := max(0, a.completionIndex-count+1)
		textWidth := popupWidth
		thumbSize, thumbStart := 0, 0
		if len(suggestions) > count {
			textWidth--
			thumbSize = max(1, count*count/len(suggestions))
			thumbStart = offset * (count - thumbSize) / (len(suggestions) - count)
		}
		for i := 0; i < count; i++ {
			index := offset + i
			y := 3 + i
			st := panel
			if index == a.completionIndex {
				st = selected
			}
			put(popupX, y, textWidth, "", st)
			put(popupX+1, y, min(labelWidth, textWidth-2), suggestions[index], st)
			descriptionX := labelWidth + 3
			if available := textWidth - descriptionX - 1; available > 0 {
				descriptionStyle := st
				if !a.options.NoColor {
					descriptionStyle = st.Foreground(palette.Muted)
				}
				put(popupX+descriptionX, y, available, completionDescription(suggestions[index]), descriptionStyle)
			}
			if thumbSize > 0 {
				mark, markStyle := "│", st
				if !a.options.NoColor {
					markStyle = st.Foreground(palette.Border)
				}
				if i >= thumbStart && i < thumbStart+thumbSize {
					mark, markStyle = "┃", st.Bold()
					if !a.options.NoColor {
						markStyle = markStyle.Foreground(palette.Accent)
					}
				}
				put(popupX+textWidth, y, 1, mark, markStyle)
			}
			a.target(layout.Rect(popupX, y, popupWidth, 1), "completion", index)
		}
	}
	if a.pendingAction != "" {
		a.paintDialog(sc)
	}
}

func typeIcon(kind string) string {
	switch notifs.TypeName(kind) {
	case "pull-request":
		return "⇄"
	case "issue":
		return "⊙"
	case "discussion":
		return "◇"
	case "release":
		return "◆"
	case "commit":
		return "●"
	case "check-suite":
		return "✓"
	case "repository-invitation":
		return "⊕"
	case "repository-vulnerability-alert":
		return "⚠"
	case "repository-advisory":
		return "△"
	case "gist":
		return "≡"
	default:
		return "○"
	}
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
