package tui

import (
	"strings"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/screen"
)

type helpBinding struct {
	key, description, action string
}

type helpPart struct {
	x, keyWidth int
	helpBinding
}

func helpRows(sc *screen.Screen, width int) [][]helpPart {
	wrap := func(value string, width int) []string {
		return strings.Split(ansi.WrapMode(value, width, "", sc.WidthMethod(), sc.AmbiguousWide()), "\n")
	}
	var rows [][]helpPart
	for _, prompt := range []string{
		"Press / to search. Repeated filter values are alternatives; every group must match.",
		"Type and read status are separate groups. Try: is:issue-or-pull-request is:release is:unread",
		"is:issue, is:pr, and is:pull-request all select both issues and pull requests, like the website.",
		"Try: reason:mention reason:comment",
		"Every title term and date bound must match. AND and OR are title words, not operators.",
		"Try: repo:OWNER/REPO state:open \"title text\"",
		"",
	} {
		for _, line := range wrap(prompt, width) {
			rows = append(rows, []helpPart{{helpBinding: helpBinding{description: line}}})
		}
	}
	groups := []struct {
		title    string
		bindings []helpBinding
	}{
		{"Navigate", []helpBinding{
			{"j/k, arrows", "Move", ""},
			{"g / G", "First / last", ""},
			{"Home / End", "First / last", ""},
			{"[ / ]", "Previous / next page", ""},
			{"PgUp / PgDn", "Previous / next page", ""},
			{"Tab", "Switch pane", "tab"},
			{"b", "Toggle scopes", "b"},
			{"v", "Toggle preview", "v"},
			{"m", "Toggle details", "m"},
			{"1 / 2 / 3", "Body / discussion / threads", ""},
			{", / .", "Previous / next discussion page", ""},
			{"Alt+Left/Right", "Focus body / details", ""},
			{"Ctrl+D/U", "Scroll preview", ""},
			{"r", "Refresh now", "r"},
			{"i", "Initial inbox", "i"},
			{"? / Esc", "Close help", "?"},
			{"q / Ctrl+C", "Quit", "q"},
		}},
		{"Actions", []helpBinding{
			{"Enter / o", "Open / apply scope", "o"},
			{"x", "Select item", "x"},
			{"Shift+A", "Select all loaded", "shift+a"},
			{"Shift+X", "Clear all selections", "shift+x"},
			{"Shift+I", "Mark read", "shift+i"},
			{"e", "Mark done", "e"},
			{"Shift+M", "Unsubscribe", "shift+m"},
			{"Shift+S", "Subscribe", "shift+s"},
			{"Click", "Focus item or use control", ""},
			{"Shift+click", "Select range", ""},
			{"Wheel", "Scroll pane", ""},
			{"Drag divider", "Resize sidebar", ""},
			{"Drag title", "Move confirmation dialog", ""},
		}},
		{"Search", []helpBinding{
			{"/ or s", "Edit query", "/"},
			{"Ctrl+Space", "Show completions", ""},
			{"Enter / Esc", "Apply / cancel", ""},
			{"Up / Down", "Select suggestion", ""},
			{"Tab / click", "Insert suggestion", ""},
			{"Ctrl+A/E", "Start / end", ""},
			{"Ctrl+B/F", "Left / right", ""},
			{"Alt+B/F", "Move by word", ""},
			{"Ctrl+Left/Right", "Move by word", ""},
			{"Ctrl+U/K", "Cut to start / end", ""},
			{"Ctrl+W", "Cut previous term", ""},
			{"Alt+Backspace", "Cut previous word", ""},
			{"Alt+D", "Cut next word", ""},
			{"Ctrl+Y", "Insert last cut", ""},
			{"Ctrl+D", "Delete character", ""},
		}},
	}
	columns := min(3, max(1, width/38))
	columnWidth := (width - (columns-1)*3) / columns
	for start := 0; start < len(groups); start += columns {
		var table [][]helpPart
		for column := 0; column < columns && start+column < len(groups); column++ {
			group := groups[start+column]
			keyWidth := 0
			for _, binding := range group.bindings {
				keyWidth = max(keyWidth, sc.StringWidth(binding.key)+2)
			}
			x := column * (columnWidth + 3)
			lines := []helpPart{{x: x, helpBinding: helpBinding{key: group.title}}}
			for _, binding := range group.bindings {
				for i, line := range wrap(binding.description, columnWidth-keyWidth) {
					part := helpPart{x: x, keyWidth: keyWidth, helpBinding: binding}
					part.description = line
					if i > 0 {
						part.key, part.action = "", ""
					}
					lines = append(lines, part)
				}
			}
			for i, part := range lines {
				for len(table) <= i {
					table = append(table, nil)
				}
				table[i] = append(table[i], part)
			}
		}
		rows = append(rows, table...)
		rows = append(rows, nil)
	}
	for _, note := range []string{
		"Dates filter notification updates: since:2026-10-01 before:2026-11-01.",
		"Done and subscription changes require confirmation.",
		"Saved, Done history, and mark-unread have no supported API.",
	} {
		for _, line := range wrap(note, width) {
			rows = append(rows, []helpPart{{helpBinding: helpBinding{description: line}}})
		}
	}
	return rows
}
