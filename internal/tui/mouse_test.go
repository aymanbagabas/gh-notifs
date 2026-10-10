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
