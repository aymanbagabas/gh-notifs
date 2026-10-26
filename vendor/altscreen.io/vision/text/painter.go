package text

import (
	"bytes"
	"unicode/utf8"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
)

// Painter paints strings with inline SGR and OSC 8 escapes onto a surface.
type Painter struct {
	target  Surface
	method  ansi.WidthMethod
	eawWide bool
}

// NewPainter returns a painter over target.
func NewPainter(target Surface) *Painter {
	return &Painter{target: target, method: target.WidthMethod(), eawWide: target.AmbiguousWide()}
}

// Bounds returns the target bounds.
func (p *Painter) Bounds() layout.Rectangle { return p.target.Bounds() }

// CellAt returns the target cell at (x, y).
func (p *Painter) CellAt(x, y int) *buffer.Cell { return p.target.CellAt(x, y) }

// SetCell writes c to the target at (x, y).
func (p *Painter) SetCell(x, y int, c *buffer.Cell) { p.target.SetCell(x, y, c) }

// WidthMethod returns the painter width method.
func (p *Painter) WidthMethod() ansi.WidthMethod { return p.method }

// AmbiguousWide returns the East Asian Ambiguous policy the painter snapshotted
// from its target.
func (p *Painter) AmbiguousWide() bool { return p.eawWide }

// StringWidth returns the display width of s, ignoring recognized escapes.
func (p *Painter) StringWidth(s string) int { return ansi.StringWidth([]byte(s), p.method, p.eawWide) }

// SetString paints str at (x, y) with truncation at the right edge.
func (p *Painter) SetString(x, y int, str string, st style.Style) layout.Position {
	return p.SetStringWrap(x, y, str, Truncate, st)
}

// SetStringWrap paints str at (x, y) with explicit right-edge handling.
func (p *Painter) SetStringWrap(x, y int, str string, wrap WrapMode, st style.Style) layout.Position {
	return p.paint(layout.Pos(x, y), p.target.Bounds(), str, wrap, st)
}

// SetStringRect paints str inside r, starting at r.Min.
func (p *Painter) SetStringRect(r layout.Rectangle, str string, st style.Style) layout.Position {
	return p.SetStringRectWrap(r, str, Truncate, st)
}

// SetStringRectWrap paints str inside r with explicit right-edge handling.
func (p *Painter) SetStringRectWrap(r layout.Rectangle, str string, wrap WrapMode, st style.Style) layout.Position {
	return p.paint(r.Min, r.Intersect(p.target.Bounds()), str, wrap, st)
}

// SetStringTruncate paints str and stamps tail when a row overflows.
// Inline styles inherit strStyle and tailStyle within their respective strings.
func (p *Painter) SetStringTruncate(x, y int, str string, strStyle style.Style, tail string, tailStyle style.Style) layout.Position {
	return p.paintTruncate(layout.Pos(x, y), p.target.Bounds(), str, strStyle, tail, tailStyle)
}

// SetStringRectTruncate paints str inside r and stamps tail when a row overflows.
// Inline styles inherit strStyle and tailStyle within their respective strings.
func (p *Painter) SetStringRectTruncate(r layout.Rectangle, str string, strStyle style.Style, tail string, tailStyle style.Style) layout.Position {
	return p.paintTruncate(r.Min, r.Intersect(p.target.Bounds()), str, strStyle, tail, tailStyle)
}

type tail struct {
	text  string
	style style.Style
	width int
}

func (p *Painter) paintTruncate(start layout.Position, clip layout.Rectangle, str string, strStyle style.Style, tailText string, tailStyle style.Style) layout.Position {
	if clip.Empty() {
		return start
	}
	w := p.StringWidth(tailText)
	var t *tail
	if w > 0 && w <= clip.Dx() {
		t = &tail{text: tailText, style: tailStyle, width: w}
	}
	return p.paintInner(start, clip, str, Truncate, t, strStyle)
}

func (p *Painter) paintTail(t *tail, clip layout.Rectangle, y int) {
	x := clip.Max.X - t.width
	p.paintInner(layout.Pos(x, y), layout.Rect(x, y, t.width, 1).Intersect(clip), t.text, Truncate, nil, t.style)
}

func (p *Painter) paint(start layout.Position, clip layout.Rectangle, str string, wrap WrapMode, st style.Style) layout.Position {
	return p.paintInner(start, clip, str, wrap, nil, st)
}

func (p *Painter) paintInner(start layout.Position, clip layout.Rectangle, str string, wrap WrapMode, t *tail, base style.Style) layout.Position {
	if clip.Empty() || start.Y >= clip.Max.Y {
		return start
	}
	x, y := start.X, start.Y
	pen := style.Empty
	var pending *pendingCell
	truncated := false
	flush := func() {
		if pending == nil || !layout.Pos(pending.x, pending.y).In(clip) {
			pending = nil
			return
		}
		cell := buffer.NewCell(pending.text, pending.width).WithStyle(pen.Inherit(base))
		p.target.SetCell(pending.x, pending.y, &cell)
		pending = nil
	}
	tok := ansi.Tokenize([]byte(str), p.method, p.eawWide)
	for {
		token, ok := tok.Next()
		if !ok {
			break
		}
		if token.Kind != ansi.TokenText || token.Width != 0 {
			flush()
		}
		switch token.Kind {
		case ansi.TokenText:
			text := string(token.Bytes)
			if token.Width == 0 {
				if pending != nil && utf8.Valid(token.Bytes) {
					pending.text += text
				}
				continue
			}
			if truncated {
				continue
			}
			if x+token.Width > clip.Max.X {
				if wrap == Truncate {
					if t != nil {
						p.paintTail(t, clip, y)
						x = clip.Max.X
					}
					truncated = true
					continue
				}
				y++
				x = clip.Min.X
				if y >= clip.Max.Y || x+token.Width > clip.Max.X {
					return layout.Pos(x, y)
				}
			}
			pending = &pendingCell{x: x, y: y, text: text, width: token.Width}
			x += token.Width
		case ansi.TokenEscape:
			if len(token.Bytes) > 0 && token.Bytes[len(token.Bytes)-1] == 'm' {
				if body, ok := csiBody(token.Bytes); ok {
					style.Read(ansi.ParamsFromRaw(body), &pen)
				}
			} else if body, ok := oscBody(token.Bytes); ok {
				if params, url, ok := ansi.ParseHyperlink(body); ok {
					pen = pen.Link(url, params)
				}
			}
		case ansi.TokenControl:
			switch token.Control {
			case '\n':
				y++
				x = clip.Min.X
				truncated = false
				if y >= clip.Max.Y {
					return layout.Pos(x, y)
				}
			case '\r':
				x = clip.Min.X
				truncated = false
			}
		}
	}
	flush()
	return layout.Pos(x, y)
}

type pendingCell struct {
	x, y  int
	text  string
	width int
}

func csiBody(seq []byte) ([]byte, bool) {
	start := 0
	if len(seq) >= 2 && seq[0] == 0x1b && seq[1] == '[' {
		start = 2
	} else if len(seq) >= 1 && seq[0] == 0x9b {
		start = 1
	} else {
		return nil, false
	}
	last := seq[len(seq)-1]
	if last < 0x40 || last > 0x7e || len(seq) <= start {
		return nil, false
	}
	return seq[start : len(seq)-1], true
}

func oscBody(seq []byte) ([]byte, bool) {
	start := 0
	if len(seq) >= 2 && seq[0] == 0x1b && seq[1] == ']' {
		start = 2
	} else if len(seq) >= 1 && seq[0] == 0x9d {
		start = 1
	} else {
		return nil, false
	}
	end := len(seq)
	if end >= start+2 && bytes.Equal(seq[end-2:], []byte{0x1b, '\\'}) {
		end -= 2
	} else if end > start && (seq[end-1] == 0x07 || seq[end-1] == 0x9c) {
		end--
	}
	if end < start {
		return nil, false
	}
	return seq[start:end], true
}
