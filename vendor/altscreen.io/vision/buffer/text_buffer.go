package buffer

import (
	"altscreen.io/vision/ansi"
	"altscreen.io/vision/layout"
)

// TextBuffer is a Buffer paired with text-width options.
type TextBuffer struct {
	buffer        *Buffer
	method        ansi.WidthMethod
	ambiguousWide bool
}

// NewTextBuffer returns a blank text buffer.
func NewTextBuffer(width, height int) *TextBuffer {
	return &TextBuffer{buffer: New(width, height), method: ansi.WcWidth}
}

// WithWidthMethod returns tb after setting the width method.
func (tb *TextBuffer) WithWidthMethod(method ansi.WidthMethod) *TextBuffer {
	tb.method = method
	return tb
}

// WithAmbiguousWide returns tb after setting East Asian Ambiguous width.
func (tb *TextBuffer) WithAmbiguousWide(wide bool) *TextBuffer {
	tb.ambiguousWide = wide
	return tb
}

// SetWidthMethod sets the width method.
func (tb *TextBuffer) SetWidthMethod(method ansi.WidthMethod) { tb.method = method }

// SetAmbiguousWide sets East Asian Ambiguous width handling.
func (tb *TextBuffer) SetAmbiguousWide(wide bool) { tb.ambiguousWide = wide }

// Width returns tb's width.
func (tb *TextBuffer) Width() int { return tb.buffer.Width() }

// Height returns tb's height.
func (tb *TextBuffer) Height() int { return tb.buffer.Height() }

// Buffer returns tb's backing buffer.
func (tb *TextBuffer) Buffer() *Buffer { return tb.buffer }

// Bounds returns tb's bounds.
func (tb *TextBuffer) Bounds() layout.Rectangle { return tb.buffer.Bounds() }

// WidthMethod returns tb's width method.
func (tb *TextBuffer) WidthMethod() ansi.WidthMethod { return tb.method }

// AmbiguousWide returns tb's East Asian Ambiguous policy.
func (tb *TextBuffer) AmbiguousWide() bool { return tb.ambiguousWide }

// CellAt returns the cell at (x, y).
func (tb *TextBuffer) CellAt(x, y int) *Cell { return tb.buffer.CellAt(x, y) }

// SetCell writes c at (x, y).
func (tb *TextBuffer) SetCell(x, y int, c *Cell) { tb.buffer.SetCell(x, y, c) }

// Resize resizes tb's backing buffer.
func (tb *TextBuffer) Resize(width, height int) { tb.buffer.Resize(width, height) }
