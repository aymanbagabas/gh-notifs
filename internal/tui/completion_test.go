package tui

import (
	"io"
	"testing"

	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
)

func TestCompletionScrollIndicator(t *testing.T) {
	for _, height := range []int{12, 32} {
		a := testApp()
		a.edit, a.input, a.caret = true, []rune("is:"), 3
		sc := screen.New(io.Discard, layout.Size{Width: 60, Height: height})
		a.paint(sc)
		var first, last hit
		for _, target := range a.hits {
			if target.kind == "completion" {
				if first.kind == "" {
					first = target
				}
				last = target
			}
		}
		if first.kind == "" {
			t.Fatal("completion popup is missing")
		}
		top := layout.Pos(first.rect.Max.X-1, first.rect.Min.Y)
		bottom := layout.Pos(last.rect.Max.X-1, last.rect.Min.Y)
		if sc.CellAt(top).Content() != "┃" || sc.CellAt(bottom).Content() != "│" {
			t.Fatal("scroll indicator does not start at the top")
		}
		values, _, _ := a.suggestions()
		a.completionIndex = len(values) - 1
		a.paint(sc)
		if sc.CellAt(top).Content() != "│" || sc.CellAt(bottom).Content() != "┃" {
			t.Fatal("scroll indicator did not move to the bottom")
		}
		a.input, a.caret = []rune("is:un"), 5
		a.paint(sc)
		for _, target := range a.hits {
			if target.kind == "completion" && sc.CellAt(layout.Pos(target.rect.Max.X-1, target.rect.Min.Y)).Content() != " " {
				t.Fatal("popup has a scrollbar when all suggestions fit")
			}
		}
	}
}
