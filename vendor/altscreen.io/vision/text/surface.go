package text

import (
	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
)

// Surface is a cell grid that also carries a text-width policy.
//
// Painting text needs to know how wide each grapheme cluster is, which a bare
// [buffer.Surface] cannot answer. [buffer.TextBuffer], [Painter] and
// screen.Screen implement this; a plain buffer, window or view does not.
type Surface interface {
	buffer.Surface
	// WidthMethod reports how grapheme clusters are measured.
	WidthMethod() ansi.WidthMethod
	// AmbiguousWide reports whether East Asian Ambiguous code points measure
	// two cells rather than one.
	AmbiguousWide() bool
}

// WrapMode selects whether string painting stops or wraps at the right edge.
type WrapMode uint8

const (
	// Truncate stops when text reaches the right edge.
	Truncate WrapMode = iota
	// Wrap continues text on the next row.
	Wrap
)

// SetString paints str at (x, y), clipped to s bounds.
func SetString(s Surface, x, y int, str string, st style.Style) layout.Position {
	return SetStringWrap(s, x, y, str, Truncate, st)
}

// SetStringWrap paints str at (x, y) with explicit right-edge handling.
func SetStringWrap(s Surface, x, y int, str string, wrap WrapMode, st style.Style) layout.Position {
	return paintLiteral(s, layout.Pos(x, y), s.Bounds(), str, wrap, s.WidthMethod(), s.AmbiguousWide(), st)
}

// SetStringRect paints str inside r, starting at r.Min.
func SetStringRect(s Surface, r layout.Rectangle, str string, st style.Style) layout.Position {
	return SetStringRectWrap(s, r, str, Truncate, st)
}

// SetStringRectWrap paints str inside r with explicit right-edge handling.
func SetStringRectWrap(s Surface, r layout.Rectangle, str string, wrap WrapMode, st style.Style) layout.Position {
	return paintLiteral(s, r.Min, r.Intersect(s.Bounds()), str, wrap, s.WidthMethod(), s.AmbiguousWide(), st)
}

// SetStringTruncate paints str at (x, y) and stamps tail when a row overflows.
// strStyle applies to str; tailStyle applies only to tail.
func SetStringTruncate(s Surface, x, y int, str string, strStyle style.Style, tail string, tailStyle style.Style) layout.Position {
	return paintLiteralTruncate(s, layout.Pos(x, y), s.Bounds(), str, strStyle, tail, tailStyle, s.WidthMethod(), s.AmbiguousWide())
}

// SetStringRectTruncate paints str inside r and stamps tail when a row overflows.
// strStyle applies to str; tailStyle applies only to tail.
func SetStringRectTruncate(s Surface, r layout.Rectangle, str string, strStyle style.Style, tail string, tailStyle style.Style) layout.Position {
	return paintLiteralTruncate(s, r.Min, r.Intersect(s.Bounds()), str, strStyle, tail, tailStyle, s.WidthMethod(), s.AmbiguousWide())
}

// StringWidth returns the terminal width of str under s's width method.
func StringWidth(s Surface, str string) int {
	return stringWidth(str, s.WidthMethod(), s.AmbiguousWide())
}

type literalTail struct {
	text  string
	style style.Style
	width int
}

func paintLiteral(target Surface, start layout.Position, clip layout.Rectangle, str string, wrap WrapMode, mode ansi.WidthMethod, eawWide bool, st style.Style) layout.Position {
	return paintLiteralInner(target, start, clip, str, wrap, mode, eawWide, st, nil)
}

func paintLiteralTruncate(target Surface, start layout.Position, clip layout.Rectangle, str string, strStyle style.Style, tailText string, tailStyle style.Style, mode ansi.WidthMethod, eawWide bool) layout.Position {
	if clip.Empty() {
		return start
	}
	w := stringWidth(tailText, mode, eawWide)
	var tail *literalTail
	if w > 0 && w <= clip.Dx() {
		tail = &literalTail{text: tailText, style: tailStyle, width: w}
	}
	return paintLiteralInner(target, start, clip, str, Truncate, mode, eawWide, strStyle, tail)
}

func paintLiteralInner(target Surface, start layout.Position, clip layout.Rectangle, str string, wrap WrapMode, mode ansi.WidthMethod, eawWide bool, st style.Style, tail *literalTail) layout.Position {
	if clip.Empty() || start.Y >= clip.Max.Y {
		return start
	}
	x, y := start.X, start.Y
	truncated := false
	// Hoisted so it is allocated once for the whole string rather than once per
	// cell. SetCell copies through the pointer and never retains it, so one
	// variable can carry every cell in turn.
	var cell buffer.Cell
	for cluster, w := range GraphemeCells(str, mode, eawWide) {
		switch cluster {
		case "\n", "\r\n":
			y++
			x = clip.Min.X
			truncated = false
			if y >= clip.Max.Y {
				return layout.Pos(x, y)
			}
			continue
		case "\r":
			x = clip.Min.X
			truncated = false
			continue
		}
		if truncated || w == 0 {
			continue
		}
		if x+w > clip.Max.X {
			if wrap == Truncate {
				if tail != nil {
					stampLiteralTail(target, tail, clip, y, mode, eawWide)
					x = clip.Max.X
				}
				truncated = true
				continue
			}
			y++
			x = clip.Min.X
			if y >= clip.Max.Y || x+w > clip.Max.X {
				return layout.Pos(x, y)
			}
		}
		if layout.Pos(x, y).In(clip) {
			cell = buffer.NewCell(cluster, w).WithStyle(st)
			target.SetCell(x, y, &cell)
		}
		x += w
	}
	return layout.Pos(x, y)
}

func stampLiteralTail(target Surface, tail *literalTail, clip layout.Rectangle, y int, mode ansi.WidthMethod, eawWide bool) {
	x := clip.Max.X - tail.width
	paintLiteralInner(target, layout.Pos(x, y), layout.Rect(x, y, tail.width, 1).Intersect(clip), tail.text, Truncate, mode, eawWide, tail.style, nil)
}
