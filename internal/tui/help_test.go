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
)

func TestHelpBarUsesSeparateKeysWithoutUnderlines(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, noColor := range []bool{false, true} {
			a := testApp()
			a.light, a.options.NoColor = light, noColor
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
			a.paint(sc)
			for _, control := range []struct{ action, label string }{
				{"q", "q quit"}, {"x", "x select"}, {"v", "v preview"},
				{"shift+i", "I read"}, {"shift+m", "M unsubscribe"}, {"shift+s", "S subscribe"},
			} {
				target := findHit(t, a, control.action)
				if target.rect.Dx() != sc.StringWidth(control.label) {
					t.Fatalf("help control has the wrong width: %q", control.label)
				}
				for offset, r := range control.label {
					x := sc.StringWidth(control.label[:offset])
					cell := sc.CellAt(layout.Pos(target.rect.Min.X+x, target.rect.Min.Y))
					if cell.Content() != string(r) || cell.Style.GetUnderline() {
						t.Fatalf("help control %q has an incorrect or underlined cell", control.label)
					}
				}
				key := sc.CellAt(target.rect.Min).Style
				description := sc.CellAt(target.rect.Min.Add(layout.Pos(2, 0))).Style
				if !key.GetBold() || description.GetBold() {
					t.Fatal("help keys and descriptions lack distinct styles")
				}
			}
		}
	}
}

func TestHelpExpandsFooterAndRestoresInbox(t *testing.T) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	ctx := context.Background()
	a.paint(sc)
	listBottom := func() int {
		for _, target := range a.hits {
			if target.kind == "list" {
				return target.rect.Max.Y
			}
		}
		t.Fatal("missing inbox hit area")
		return 0
	}
	original := listBottom()
	a.key(ctx, event.Key{Code: '?'}.Normalized())
	a.paint(sc)
	if !a.help || listBottom() >= original {
		t.Fatal("help did not reduce the inbox height")
	}
	if sc.CellAt(layout.Pos(60, listBottom())).Content() != "─" {
		t.Fatal("help caption erased the top border")
	}
	out := text.String(sc, color.Disabled)
	for _, want := range []string{"Fix slow uploads", "Refreshed", "? Close help", "j/k, arrows", "refresh", "Try: is:issue-or-pull-request", "Navigate", "Actions", "Search"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expanded footer lost %q", want)
		}
	}
	a.mouse(ctx, event.MouseWheelEvent{X: 10, Y: 20, Button: event.MouseWheelDown}, sc)
	if a.helpOffset == 0 || a.cursor != 0 {
		t.Fatal("help wheel scrolled the inbox instead")
	}
	a.paint(sc)
	for _, target := range a.hits {
		if target.kind == "?" && target.rect.Min.Y < 31 {
			a.mouse(ctx, event.MouseClickEvent{X: target.rect.Min.X, Y: target.rect.Min.Y, Button: event.MouseLeft}, sc)
			break
		}
	}
	a.paint(sc)
	if a.help || listBottom() != original {
		t.Fatal("closing help did not restore the inbox")
	}
}

func TestHelpWrapsAndScrollsInSmallTerminal(t *testing.T) {
	a := testApp()
	a.help = true
	sc := screen.New(io.Discard, layout.Size{Width: 40, Height: 12})
	a.paint(sc)
	if !strings.Contains(text.String(sc, color.Disabled), "Close help") {
		t.Fatal("small terminal has no help close control")
	}
	a.key(context.Background(), event.Key{Code: event.KeyDown})
	a.paint(sc)
	if a.helpOffset != 3 {
		t.Fatal("arrow key did not scroll help")
	}
	a.key(context.Background(), event.Key{Code: event.KeyEscape})
	if a.help {
		t.Fatal("Escape did not close help")
	}
}

func TestHelpColumnsAndExamples(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
		rows := helpRows(sc, width-4)
		exampleRow, firstHeading, headings := -1, -1, 0
		for y, row := range rows {
			for _, part := range row {
				if strings.HasPrefix(part.description, "Try:") {
					exampleRow = y
				}
				if part.key == "Navigate" || part.key == "Actions" || part.key == "Search" {
					headings++
					if firstHeading < 0 {
						firstHeading = y
					}
					if width == 120 && y != firstHeading {
						t.Fatal("wide terminal did not place keybinding groups side by side")
					}
					if width == 40 && part.x != 0 {
						t.Fatal("narrow terminal did not stack keybinding groups")
					}
				}
				if part.x+max(sc.StringWidth(part.key), part.keyWidth+sc.StringWidth(part.description)) > width-4 {
					t.Fatalf("help exceeds width %d: %#v", width, part)
				}
			}
		}
		if headings != 3 || exampleRow < 0 || exampleRow >= firstHeading {
			t.Fatal("help must show examples before all three keybinding groups")
		}
	}
}

func TestHelpShortcutClickAndWheel(t *testing.T) {
	a := testApp()
	a.help = true
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	var selectHit *hit
	for i := range a.hits {
		if a.hits[i].kind == "x" && a.hits[i].rect.Min.Y < 31 {
			selectHit = &a.hits[i]
			break
		}
	}
	if selectHit == nil {
		t.Fatal("expanded help has no selection shortcut")
	}
	x, y := selectHit.rect.Min.X, selectHit.rect.Min.Y
	a.mouse(context.Background(), event.MouseClickEvent{X: x, Y: y, Button: event.MouseLeft}, sc)
	if !a.selected[a.result.Items[0].ID] {
		t.Fatal("help shortcut did not select the current item")
	}
	a.mouse(context.Background(), event.MouseWheelEvent{X: x, Y: y, Button: event.MouseWheelDown}, sc)
	if a.helpOffset != 3 || a.cursor != 0 {
		t.Fatal("wheel over a help shortcut must scroll help")
	}
}
