package buffer

import (
	"altscreen.io/vision/layout"
)

// View clips reads and writes to a rectangle on another surface.
type View struct {
	inner  Surface
	bounds layout.Rectangle
}

// NewView returns a view clipped to inner's bounds.
func NewView(inner Surface, bounds layout.Rectangle) *View {
	return &View{inner: inner, bounds: inner.Bounds().Intersect(bounds)}
}

// Inner returns the wrapped surface.
func (v *View) Inner() Surface { return v.inner }

// Bounds returns v's clipped bounds.
func (v *View) Bounds() layout.Rectangle { return v.bounds }

// CellAt returns a cell only when (x, y) is inside the view.
func (v *View) CellAt(x, y int) *Cell {
	if !layout.Pos(x, y).In(v.bounds) {
		return nil
	}
	return v.inner.CellAt(x, y)
}

// SetCell writes c only when (x, y) is inside the view.
func (v *View) SetCell(x, y int, c *Cell) {
	if layout.Pos(x, y).In(v.bounds) {
		v.inner.SetCell(x, y, c)
	}
}
