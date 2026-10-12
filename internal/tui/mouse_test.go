package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/style"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/config"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func TestMouseSelectionDragAndScopes(t *testing.T) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	ctx := context.Background()
	a.paint(sc)
	a.mouse(ctx, event.MouseClickEvent{X: 90, Y: 9, Button: event.MouseLeft}, sc)
	if a.cursor != 1 {
		t.Fatal("clicking the second line did not select the item")
	}
	if len(a.selected) != 0 {
		t.Fatal("plain click changed multi-selection")
	}
	a.paint(sc)
	for _, y := range []int{8, 9} {
		for _, x := range []int{34, 119} {
			if sc.CellAt(layout.Pos(x, y)).Style.GetBackground() != theme.Colors(false).Selection {
				t.Fatal("selection does not fill both lines")
			}
		}
		if sc.CellAt(layout.Pos(25, y)).Content() != " " {
			t.Fatal("item still has a left border")
		}
	}
	if sc.CellAt(layout.Pos(26, 6)).Style.GetForeground() != theme.Colors(false).Accent ||
		sc.CellAt(layout.Pos(26, 6)).Content() != "●" {
		t.Fatal("unread dot is not blue")
	}
	a.mouse(ctx, event.MouseWheelEvent{X: 90, Y: 9, Button: event.MouseWheelUp}, sc)
	if a.cursor != 0 || a.sidebar {
		t.Fatal("wheel did not move within the pointed pane")
	}
	a.mouse(ctx, event.MouseClickEvent{X: 24, Y: 10, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseMoveEvent{X: 40, Y: 10, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseReleaseEvent{X: 40, Y: 10, Button: event.MouseLeft}, sc)
	a.paint(sc)
	if a.sideWidth != 41 || a.dragging != "" || sc.CellAt(layout.Pos(40, 4)).Content() != "│" {
		t.Fatal("sidebar resize did not update layout")
	}
	a.mouse(ctx, event.MouseClickEvent{X: 44, Y: 8, Button: event.MouseLeft}, sc)
	if !a.selected["2"] {
		t.Fatal("resized checkbox hit area is wrong")
	}
	if entries := a.sideEntries(); len(entries) != 1 || entries[0].query != "repo:acme/app" {
		t.Fatal("sidebar did not show a flat repository list")
	}
	a.options.Scopes = []config.Scope{{Name: "All", Query: ""}}
	a.query.Raw = "repo:acme/app"
	if a.sideEntries()[0].query != "" {
		t.Fatal("saved scope inherited the current query")
	}
}

func TestShiftClickSelectsRange(t *testing.T) {
	for _, tt := range []struct {
		name     string
		from, to int
		kind     string
	}{
		{"forward", 1, 3, "item"},
		{"backward", 3, 1, "item"},
		{"same item", 2, 2, "item"},
		{"checkbox", 1, 3, "checkbox"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := testApp()
			a.result.Items = append(a.result.Items,
				notifs.Thread{ID: "3", Title: "Third"},
				notifs.Thread{ID: "4", Title: "Fourth"},
				notifs.Thread{ID: "5", Title: "Fifth"})
			a.options.PageSize = 5
			a.cursor = tt.from
			a.selected["1"] = true
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
			a.paint(sc)
			found := false
			for _, target := range a.hits {
				if target.kind == tt.kind && target.index == tt.to {
					a.mouse(context.Background(), event.MouseClickEvent{
						X: target.rect.Max.X - 1, Y: target.rect.Max.Y - 1,
						Button: event.MouseLeft, Mod: event.KeyModShift,
					}, sc)
					found = true
					break
				}
			}
			if !found || a.cursor != tt.to {
				t.Fatal("Shift+click did not focus the target item")
			}
			for i, item := range a.result.Items {
				want := i == 0 || i >= min(tt.from, tt.to) && i <= max(tt.from, tt.to)
				if a.selected[item.ID] != want {
					t.Fatalf("item %s selected=%v, want %v", item.ID, a.selected[item.ID], want)
				}
			}
		})
	}
}

func TestEllipsisAndEmptySidebar(t *testing.T) {
	a := testApp()
	a.result.Items[0].Title = strings.Repeat("世界", 80)
	sc := screen.New(io.Discard, layout.Size{Width: 60, Height: 22})
	a.paint(sc)
	if sc.CellAt(layout.Pos(59, 6)).Content() != "…" {
		t.Fatal("long title has no Unicode ellipsis")
	}
	for _, x := range []int{9, 59} {
		st := sc.CellAt(layout.Pos(x, 6)).Style
		p := theme.Colors(false)
		if !st.GetBold() || st.GetForeground() != p.Foreground || st.GetBackground() != p.Selection {
			t.Fatalf("truncated title lost its text or tail style at column %d", x)
		}
	}
	a.result.Items = nil
	a.repositories = nil
	a.sidebar = true
	a.key(context.Background(), event.Key{Code: 'g', Mod: event.KeyModShift})
	a.key(context.Background(), event.Key{Code: event.KeyEnter})
	a.paint(sc)
	if !strings.Contains(text.String(sc, color.Disabled), "No notifications") {
		t.Fatal("empty sidebar broke the inbox")
	}
}

func TestCompletionPreservesSurroundingQuery(t *testing.T) {
	a := testApp()
	a.edit = true
	a.input = []rune("org:acme is:un state:open")
	a.caret = len([]rune("org:acme is:un"))
	values, start, end := a.suggestions()
	if len(values) != 1 || values[0] != "is:unread" {
		t.Fatalf("unexpected suggestions: %v", values)
	}
	a.complete(values[0], start, end)
	if string(a.input) != "org:acme is:unread state:open" {
		t.Fatalf("completion changed adjacent terms: %q", a.input)
	}
	a.input, a.caret = []rune(`repo:"ac`), 8
	values, start, end = a.suggestions()
	if len(values) != 1 || values[0] != "repo:acme/app" {
		t.Fatalf("quoted completion: %v", values)
	}
	a.complete(values[0], start, end)
	if string(a.input) != "repo:acme/app" {
		t.Fatal("completion left an unclosed quote")
	}
	a.input, a.caret = []rune("is:"), 3
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	for _, h := range a.hits {
		if h.kind == "completion" && h.index == 1 {
			a.mouse(context.Background(), event.MouseClickEvent{X: h.rect.Min.X, Y: h.rect.Min.Y, Button: event.MouseLeft}, sc)
			break
		}
	}
	if string(a.input) != "is:unread" {
		t.Fatal("popup click did not insert the selected value")
	}
}

func TestMouseCannotChangePendingActionTargets(t *testing.T) {
	a := testApp()
	a.pendingAction = "done"
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	a.mouse(context.Background(), event.MouseClickEvent{X: 90, Y: 9, Button: event.MouseLeft}, sc)
	if a.cursor != 0 || a.pendingAction != "done" {
		t.Fatal("background click changed the pending action")
	}
	for _, h := range a.hits {
		if h.kind == "cancel" {
			a.mouse(context.Background(), event.MouseClickEvent{X: h.rect.Min.X, Y: h.rect.Min.Y, Button: event.MouseLeft}, sc)
			break
		}
	}
	if a.pendingAction != "" || a.acting {
		t.Fatal("cancel click did not dismiss the action")
	}
}

func TestMousePointerTargetsAndDrags(t *testing.T) {
	a := testApp()
	a.result.Items[0].URL = "https://github.com/acme/app/pull/7"
	a.showPreview, a.help = true, true
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 48})
	ctx := context.Background()
	a.paint(sc)
	if got := a.mousePointer(sc); got != "default" {
		t.Fatalf("pointer before mouse input=%q", got)
	}
	for _, tc := range []struct{ kind, shape string }{
		{"query", "text"}, {"item", "default"}, {"checkbox", "pointer"},
		{"scope", "pointer"}, {"toggle", "pointer"}, {"preview-tab", "pointer"},
		{"metadata-toggle", "pointer"}, {"divider", "ew-resize"},
		{"metadata-divider", "ew-resize"}, {"preview-divider", "ns-resize"},
		{"help-divider", "ns-resize"}, {"metadata", "default"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			h := findHit(t, a, tc.kind)
			a.mouse(ctx, event.MouseMoveEvent{X: h.rect.Min.X, Y: h.rect.Min.Y}, sc)
			if got := a.mousePointer(sc); got != tc.shape {
				t.Fatalf("pointer=%q, want %q", got, tc.shape)
			}
		})
	}
	item := findHit(t, a, "item")
	a.mouse(ctx, event.MouseMoveEvent{X: item.rect.Min.X + 9, Y: item.rect.Min.Y + 1}, sc)
	if got := a.mousePointer(sc); got != "pointer" {
		t.Fatalf("item hyperlink pointer=%q", got)
	}
	// The help close button must take precedence over its resize border.
	a.mouse(ctx, event.MouseMoveEvent{X: 108, Y: findHit(t, a, "help-divider").rect.Min.Y}, sc)
	if got := a.mousePointer(sc); got != "pointer" {
		t.Fatalf("help close pointer=%q", got)
	}
	for _, tc := range []struct{ kind, shape string }{
		{"divider", "ew-resize"}, {"metadata-divider", "ew-resize"},
		{"preview-divider", "ns-resize"}, {"help-divider", "ns-resize"},
	} {
		h := findHit(t, a, tc.kind)
		a.mouse(ctx, event.MouseClickEvent{X: h.rect.Min.X, Y: h.rect.Min.Y, Button: event.MouseLeft}, sc)
		a.mouse(ctx, event.MouseMoveEvent{X: 0, Y: 0, Button: event.MouseLeft}, sc)
		a.paint(sc)
		if got := a.mousePointer(sc); got != tc.shape {
			t.Fatalf("%s active drag pointer=%q", tc.kind, got)
		}
		a.mouse(ctx, event.MouseReleaseEvent{X: 0, Y: 0, Button: event.MouseLeft}, sc)
		if got := a.mousePointer(sc); got != "default" || a.dragging != "" {
			t.Fatalf("%s released pointer=%q, drag=%q", tc.kind, got, a.dragging)
		}
	}
	a.confirmAction("done")
	a.paint(sc)
	title := findHit(t, a, "dialog-title")
	a.mouse(ctx, event.MouseMoveEvent{X: title.rect.Min.X, Y: title.rect.Min.Y}, sc)
	if got := a.mousePointer(sc); got != "grab" {
		t.Fatalf("dialog title pointer=%q", got)
	}
	a.mouse(ctx, event.MouseClickEvent{X: title.rect.Min.X, Y: title.rect.Min.Y, Button: event.MouseLeft}, sc)
	if got := a.mousePointer(sc); got != "grabbing" {
		t.Fatalf("pressed dialog title pointer=%q", got)
	}
	a.mouse(ctx, event.MouseReleaseEvent{X: title.rect.Min.X, Y: title.rect.Min.Y, Button: event.MouseLeft}, sc)
	if got := a.mousePointer(sc); got != "grab" {
		t.Fatalf("released dialog title pointer=%q", got)
	}
	a.mouse(ctx, event.MouseMoveEvent{X: 10, Y: 2}, sc)
	if got := a.mousePointer(sc); got != "default" {
		t.Fatalf("modal background pointer=%q", got)
	}
	cancel := findHit(t, a, "cancel")
	a.mouse(ctx, event.MouseMoveEvent{X: cancel.rect.Min.X, Y: cancel.rect.Min.Y}, sc)
	if got := a.mousePointer(sc); got != "pointer" {
		t.Fatalf("dialog close pointer=%q", got)
	}
}

func TestMousePointerTracksLayoutAndLinks(t *testing.T) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	ctx := context.Background()
	a.mouse(ctx, event.MouseMoveEvent{X: 24, Y: 10}, sc)
	if got := a.mousePointer(sc); got != "ew-resize" {
		t.Fatalf("initial divider hover=%q", got)
	}
	a.sideHidden = true
	a.paint(sc)
	if got := a.mousePointer(sc); got != "default" {
		t.Fatalf("hidden divider pointer=%q", got)
	}
	sc.SetString(layout.Pos(24, 10), "link", style.New().Link("https://github.com", ""))
	if got := a.mousePointer(sc); got != "pointer" {
		t.Fatalf("hyperlink pointer=%q", got)
	}
	a.pendingAction = "done"
	a.paint(sc)
	if got := a.mousePointer(sc); got != "default" {
		t.Fatalf("covered hyperlink pointer=%q", got)
	}
	a.pendingAction, a.sideHidden = "", false
	a.paint(sc)
	a.mouse(ctx, event.MouseClickEvent{X: 24, Y: 10, Button: event.MouseLeft}, sc)
	a.sideHidden = true
	a.paint(sc)
	if got := a.mousePointer(sc); got != "default" || a.dragging != "" {
		t.Fatalf("hidden drag pointer=%q, drag=%q", got, a.dragging)
	}
	a.mouse(ctx, event.MouseMoveEvent{X: 200, Y: 200}, sc)
	if got := a.mousePointer(sc); got != "default" {
		t.Fatalf("out-of-bounds pointer=%q", got)
	}
}

func TestVerticalPaneResize(t *testing.T) {
	a := testApp()
	a.showPreview = true
	a.preview = &notifs.Preview{Title: "Preview", Body: strings.Repeat("Body.\n\n", 40)}
	a.previewOffset = 3
	preview := a.preview
	generation := a.generation
	ctx := context.Background()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 40})
	drag := func(kind string, y int) {
		t.Helper()
		a.paint(sc)
		h := findHit(t, a, kind)
		a.mouse(ctx, event.MouseClickEvent{X: h.rect.Min.X, Y: h.rect.Min.Y, Button: event.MouseLeft}, sc)
		a.mouse(ctx, event.MouseMoveEvent{X: h.rect.Min.X, Y: y, Button: event.MouseLeft}, sc)
		a.mouse(ctx, event.MouseReleaseEvent{X: h.rect.Min.X, Y: y, Button: event.MouseLeft}, sc)
		a.paint(sc)
	}
	drag("preview-divider", 12)
	if a.previewHeight != 24 || a.previewBounds.Min.Y != 13 || a.previewOffset != 3 {
		t.Fatalf("preview resize: height=%d, bounds=%v, offset=%d", a.previewHeight, a.previewBounds, a.previewOffset)
	}
	a.showPreview = false
	a.paint(sc)
	a.showPreview = true
	a.paint(sc)
	if a.previewBounds.Min.Y != 13 {
		t.Fatal("preview height was not preserved")
	}
	a.help = true
	drag("help-divider", 28)
	if a.helpHeight != 12 || findHit(t, a, "help-divider").rect.Min.Y != 28 || a.previewBounds.Max.Y != 28 {
		t.Fatal("help resize did not leave the preview docked above it")
	}
	a.help = false
	a.paint(sc)
	a.help = true
	a.paint(sc)
	if findHit(t, a, "help-divider").rect.Min.Y != 28 {
		t.Fatal("help height was not preserved")
	}
	drag("help-divider", -20)
	if findHit(t, a, "help-divider").rect.Min.Y != 16 {
		t.Fatal("help covered the preview or list")
	}
	drag("preview-divider", -20)
	if findHit(t, a, "list").rect.Dy() < 2 || a.previewBounds.Dy() < 5 {
		t.Fatal("preview resize hid list rows or preview controls")
	}
	drag("preview-divider", 100)
	if a.previewHeight != 6 || a.previewBounds.Dy() != 5 {
		t.Fatal("preview minimum height was not enforced")
	}
	drag("help-divider", 40)
	if a.helpHeight != 6 {
		t.Fatal("help minimum height was not enforced")
	}
	if a.preview != preview || a.previewGeneration != 0 || a.generation != generation || a.previewOffset != 3 {
		t.Fatalf("resizing changed preview content, scroll, or fetch state: same=%t previewGeneration=%d generation=%d offset=%d",
			a.preview == preview, a.previewGeneration, a.generation, a.previewOffset)
	}
}

func TestPaneSizesAcrossTerminalResize(t *testing.T) {
	a := testApp()
	a.showPreview, a.help = true, true
	a.previewHeight, a.helpHeight = 18, 12
	for height := 10; height <= 60; height++ {
		sc := screen.New(io.Discard, layout.Size{Width: 120, Height: height})
		a.paint(sc)
		help := findHit(t, a, "help-divider")
		if help.rect.Min.Y < 4 || height-help.rect.Min.Y < 6 {
			t.Fatalf("height=%d: invalid help bounds: %v", height, help.rect)
		}
		if height >= 22 {
			list := findHit(t, a, "list")
			if list.rect.Dy() < 2 || a.previewBounds.Dy() < 5 || a.previewBounds.Max.Y != help.rect.Min.Y {
				t.Fatalf("height=%d: list=%v preview=%v help=%v", height, list.rect, a.previewBounds, help.rect)
			}
		}
		if a.previewHeight != 18 || a.helpHeight != 12 {
			t.Fatal("terminal resize overwrote chosen sizes")
		}
	}
}
