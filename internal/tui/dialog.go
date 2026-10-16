package tui

import (
	"context"
	"fmt"
	"strings"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/style"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func (a *app) confirmAction(action string) {
	if a.acting {
		a.status = "Wait for the current action to finish."
		return
	}
	ids := a.actionIDs()
	if len(ids) == 0 {
		a.status = "No notifications selected."
		return
	}
	a.pendingAction, a.pendingIDs = action, ids
	a.dialogMoved, a.dialogConfirm = false, false
	a.dialogHover = 0
	a.dragging = ""
}

func (a *app) dialogKey(ctx context.Context, key event.Key) {
	switch {
	case key.MatchesAny("tab", "shift+tab", "left", "right"):
		a.dialogConfirm = !a.dialogConfirm
	case key.MatchesAny("y", "enter", "n", "esc"):
		confirm := key.Matches("y") || (key.Matches("enter") && a.dialogConfirm)
		action, ids := a.pendingAction, a.pendingIDs
		a.pendingAction, a.pendingIDs, a.dragging = "", nil, ""
		if confirm {
			a.act(ctx, action, ids)
		}
	}
}

func (a *app) paintDialog(sc *screen.Screen) {
	w, h := sc.Size().Width, sc.Size().Height
	width, height := min(58, w-4), 8
	x, y := (w-width)/2, (h-height)/2
	if a.dialogMoved {
		x = max(0, min(a.dialogPosition.X, w-width))
		y = max(0, min(a.dialogPosition.Y, h-height))
	}
	a.dialogPosition = layout.Pos(x, y)
	a.dialogRect = layout.Rect(x, y, width, height)
	a.showCursor = false
	sc.ClearCursorPosition()
	for row := 0; row < h; row++ {
		for column := 0; column < w; column++ {
			cell := *sc.CellAt(column, row)
			cell.Style = cell.Style.Faint()
			sc.SetCell(column, row, &cell)
		}
	}
	p := theme.Colors(a.light)
	panel, border, heading, dim := style.New(), style.New(), style.New().Bold(), style.New().Faint()
	if !a.options.NoColor {
		panel = panel.Foreground(p.Foreground).Background(p.Surface)
		border = panel.Foreground(p.Border)
		heading = panel.Foreground(p.Warning).Bold()
		dim = panel.Foreground(p.Muted)
	}
	put := func(column, row, width int, value string, st style.Style) {
		rect := layout.Rect(column, row, width, 1)
		text.SetStringRect(sc, rect, strings.Repeat(" ", width), st)
		text.SetStringRectTruncate(sc, rect, value, st, "…", st)
	}
	for row := y + 1; row < y+height-1; row++ {
		put(x, row, width, "", panel)
		put(x, row, 1, "│", border)
		put(x+width-1, row, 1, "│", border)
	}
	put(x, y, width, "╭"+strings.Repeat("─", width-2)+"╮", border)
	put(x, y+height-1, width, "╰"+strings.Repeat("─", width-2)+"╯", border)
	caption := " Confirm action "
	put(x+2, y, min(width-6, sc.StringWidth(caption)), caption, heading)
	hoverStyle := panel.Bold().Reverse().Italic()
	if !a.options.NoColor {
		hoverStyle = panel.Bold().Foreground(p.Background).Background(p.Accent)
	}
	closeStyle := heading
	if a.dialogHover == 3 {
		closeStyle = hoverStyle
	}
	put(x+width-3, y, 1, "×", closeStyle)
	title := map[string]string{"done": "Mark done", "unsubscribe": "Unsubscribe from", "subscribe": "Subscribe to"}[a.pendingAction]
	put(x+2, y+2, width-4, fmt.Sprintf("%s %d notification(s)?", title, len(a.pendingIDs)), panel.Bold())
	put(x+2, y+3, width-4, "This updates your GitHub notifications.", dim)
	put(x+2, y+4, width-4, "Drag the title bar to move this dialog.", dim)
	a.hits = a.hits[:0]
	a.target(a.dialogRect, "dialog", 0)
	a.target(layout.Rect(x+1, y, width-2, 1), "dialog-title", 0)
	a.target(layout.Rect(x+width-3, y, 1, 1), "cancel", 3)
	buttonX := x + width - 13
	for i, button := range []struct {
		text, action string
		shortcut     rune
	}{
		{" Yes ", "confirm", 'Y'}, {" No ", "cancel", 'N'},
	} {
		st := panel.Bold()
		if !a.options.NoColor {
			st = st.Foreground(p.Accent)
		}
		if (i == 0) == a.dialogConfirm {
			if a.options.NoColor {
				st = st.Reverse()
			} else {
				st = st.Background(p.Selection)
			}
		}
		if a.dialogHover == i+1 {
			st = hoverStyle
		}
		buttonWidth := sc.StringWidth(button.text)
		put(buttonX, y+6, buttonWidth, button.text, st)
		underlineShortcut(sc, buttonX, y+6, button.text, button.shortcut, st)
		a.target(layout.Rect(buttonX, y+6, buttonWidth, 1), button.action, i+1)
		buttonX += buttonWidth + 2
	}
}
