package buffer

import "altscreen.io/vision/style"

// Cell is one terminal-grid cell with content, style, and a column width.
// The zero value is a continuation placeholder.
type Cell struct {
	content string
	// Style is the visual style applied to the cell.
	Style style.Style
	width int
}

// BlankCell returns a blank narrow cell with empty style.
func BlankCell() Cell { return Cell{content: " ", width: 1} }

// NewCell returns a cell with content and a width in terminal columns.
// Width zero denotes a continuation, regardless of content. A negative width
// panics. The caller must measure the content under the target width policy.
func NewCell(content string, width int) Cell {
	if width < 0 {
		panic("buffer: negative cell width")
	}
	return Cell{content: content, Style: style.Empty, width: width}
}

// ContinuationCell returns a wide-cell continuation placeholder.
func ContinuationCell() Cell { return Cell{} }

// IsWide reports whether c owns more than one column.
func (c Cell) IsWide() bool { return c.width > 1 }

// IsContinuation reports whether c is a wide-cell continuation.
func (c Cell) IsContinuation() bool { return c.width == 0 }

// IsBlank reports whether c draws a single blank column, regardless of style.
func (c Cell) IsBlank() bool { return c.width == 1 && (c.content == "" || c.content == " ") }

// Content returns c's grapheme content.
func (c Cell) Content() string { return c.content }

// Width returns c's terminal-cell footprint.
func (c Cell) Width() int { return c.width }

// WithStyle returns c with st as its style.
func (c Cell) WithStyle(st style.Style) Cell { c.Style = st; return c }

// Equal reports whether c and other are the same cell.
func (c Cell) Equal(other Cell) bool {
	return c.width == other.width && c.content == other.content && c.Style.Equal(other.Style)
}
