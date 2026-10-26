package buffer

import (
	"altscreen.io/vision/layout"
)

// Window is an owned buffer plus a target position.
type Window struct {
	buffer   *Buffer
	position layout.Position
}

// NewWindow returns a blank window at the origin.
func NewWindow(width, height int) *Window {
	return &Window{buffer: New(width, height)}
}

// Position returns where w's top-left lands on a target.
func (w *Window) Position() layout.Position { return w.position }

// SetPosition moves w.
func (w *Window) SetPosition(x, y int) { w.position = layout.Pos(x, y) }

// Buffer returns w's backing buffer.
func (w *Window) Buffer() *Buffer { return w.buffer }

// Resize resizes w's backing buffer.
func (w *Window) Resize(width, height int) { w.buffer.Resize(width, height) }

// Bounds returns w's local bounds.
func (w *Window) Bounds() layout.Rectangle { return w.buffer.Bounds() }

// CellAt returns the cell at (x, y).
func (w *Window) CellAt(x, y int) *Cell { return w.buffer.CellAt(x, y) }

// SetCell writes c at (x, y).
func (w *Window) SetCell(x, y int, c *Cell) { w.buffer.SetCell(x, y, c) }

// Present draws w onto dst at w's position.
func (w *Window) Present(dst Surface) { Draw(w.buffer, dst, w.position.X, w.position.Y) }

// Fill fills w with c.
func (w *Window) Fill(c *Cell) { Fill(w, c) }

// InsertLines inserts n rows at y within [y, bottom).
func (w *Window) InsertLines(y, n, bottom int, fill *Cell) { InsertLines(w, y, n, bottom, fill) }

// DeleteLines deletes n rows at y within [y, bottom).
func (w *Window) DeleteLines(y, n, bottom int, fill *Cell) { DeleteLines(w, y, n, bottom, fill) }

// InsertCells inserts n cells at (x, y) within [x, right).
func (w *Window) InsertCells(x, y, n, right int, fill *Cell) {
	InsertCells(w, x, y, n, right, fill)
}

// DeleteCells deletes n cells at (x, y) within [x, right).
func (w *Window) DeleteCells(x, y, n, right int, fill *Cell) {
	DeleteCells(w, x, y, n, right, fill)
}
