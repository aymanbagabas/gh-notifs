package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func TestRowIconsMetadataAndHyperlinks(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, state := range []string{"open", "closed", "merged", "draft"} {
			a := testApp()
			a.light = light
			item := &a.result.Items[0]
			item.State, item.Reason = state, "review_requested"
			item.URL = "https://github.com/acme/app/pull/7"
			item.UpdatedAt = time.Now().Add(-2 * time.Minute)
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
			a.paint(sc)
			palette := theme.Colors(light)
			want := palette.Muted
			switch state {
			case "open":
				want = palette.Success
			case "closed":
				want = palette.Danger
			case "merged":
				want = palette.Purple
			}
			icon := sc.CellAt(layout.Pos(32, 6))
			if icon.Content() != "⇄" || icon.Style.GetForeground() != want {
				t.Fatalf("%s: wrong icon or state color", state)
			}
			for x := 25; x < 120; x++ {
				cell := sc.CellAt(layout.Pos(x, 7))
				if cell.Style.GetForeground() != palette.Muted || cell.Style.GetBackground() != palette.Selection {
					t.Fatalf("%s: metadata color changed at %d", state, x)
				}
			}
			for _, pos := range []layout.Position{layout.Pos(34, 7)} {
				link, ok := sc.CellAt(pos).Style.GetLink()
				if !ok || link.URL != item.URL {
					t.Fatalf("missing hyperlink at %v", pos)
				}
			}
			for _, pos := range []layout.Position{layout.Pos(34, 6), layout.Pos(33, 6), layout.Pos(119, 6), layout.Pos(70, 7), layout.Pos(119, 7)} {
				if sc.CellAt(pos).Style.HasLink() {
					t.Fatalf("hyperlink extends into padding or other metadata at %v", pos)
				}
			}
			var right strings.Builder
			for x := 112; x < 120; x++ {
				right.WriteString(sc.CellAt(layout.Pos(x, 7)).Content())
			}
			if !strings.HasSuffix(right.String(), "2m ago") {
				t.Fatalf("time is not right aligned: %q", right.String())
			}
			if !strings.Contains(text.String(sc, color.TrueColor), "\x1b]8;") {
				t.Fatal("rendered output has no OSC 8 sequence")
			}
		}
	}
}

func TestSelectionMarkAndUnsafeLink(t *testing.T) {
	a := testApp()
	a.selected["1"] = true
	a.result.Items[0].URL = "https://github.com/\x1b]52;c;bad"
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	tile := sc.CellAt(layout.Pos(29, 6))
	if tile.Content() != "✓" || tile.Style.GetForeground() != theme.Colors(false).Success ||
		tile.Style.GetBackground() != theme.Colors(false).Selection {
		t.Fatal("selection mark has the wrong text, color, or background")
	}
	if sc.CellAt(layout.Pos(34, 7)).Style.HasLink() {
		t.Fatal("unsafe hyperlink was emitted")
	}
	if sc.CellAt(layout.Pos(26, 8)).Content() != " " {
		t.Fatal("read item has an unread marker")
	}
}

func TestHoverSelection(t *testing.T) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	if sc.CellAt(layout.Pos(29, 8)).Content() != " " {
		t.Fatal("unselected row shows a selection control without hover")
	}
	a.mouse(context.Background(), event.MouseMoveEvent{X: 60, Y: 9, Button: event.MouseNone}, sc)
	a.paint(sc)
	if sc.CellAt(layout.Pos(29, 8)).Content() != "✓" ||
		sc.CellAt(layout.Pos(119, 9)).Style.GetBackground() != theme.Colors(false).Surface {
		t.Fatal("hover did not reveal the selection control and row highlight")
	}
	a.mouse(context.Background(), event.MouseMoveEvent{X: 29, Y: 8, Button: event.MouseNone}, sc)
	a.paint(sc)
	if sc.CellAt(layout.Pos(29, 8)).Style.GetForeground() != theme.Colors(false).Accent {
		t.Fatal("selection control does not change color on hover")
	}
	a.mouse(context.Background(), event.MouseClickEvent{X: 29, Y: 8, Button: event.MouseLeft}, sc)
	a.mouse(context.Background(), event.MouseMoveEvent{X: 0, Y: 0, Button: event.MouseNone}, sc)
	a.cursor = 0
	a.paint(sc)
	if sc.CellAt(layout.Pos(29, 8)).Content() != "✓" ||
		sc.CellAt(layout.Pos(29, 8)).Style.GetForeground() != theme.Colors(false).Success ||
		sc.CellAt(layout.Pos(119, 9)).Style.GetBackground() != theme.Colors(false).Selected {
		t.Fatal("selected row lost its mark or tint after the pointer left")
	}
}

func TestMetadataColumnsStayAligned(t *testing.T) {
	for _, width := range []int{60, 120} {
		a := testApp()
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
		column := -1
		for _, elapsed := range []time.Duration{30 * time.Minute, time.Hour, 120 * time.Hour, 2400 * time.Hour} {
			a.result.Items[0].UpdatedAt = time.Now().Add(-elapsed)
			a.result.Items[0].Reason = "mention"
			a.paint(sc)
			for x := width - 14; x < width-11; x++ {
				if sc.CellAt(layout.Pos(x, 7)).Content() != " " {
					t.Fatal("reason and age must have a plain space separator")
				}
			}
			var line strings.Builder
			for x := 0; x < width; x++ {
				line.WriteString(sc.CellAt(layout.Pos(x, 7)).Content())
			}
			current := strings.Index(line.String(), "mention")
			if column < 0 {
				column = current
			}
			if current < 0 || current != column {
				t.Fatalf("width %d: reason shifted from %d to %d for %s", width, column, current, elapsed)
			}
		}
	}
}
