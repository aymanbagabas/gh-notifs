package buffer

import (
	"altscreen.io/vision/layout"
)

// Buffer is row-major storage for a rectangular grid of terminal cells.
type Buffer struct {
	cells  []Cell
	width  int
	height int
}

// New returns a blank buffer with width columns and height rows.
func New(width, height int) *Buffer {
	width = max(width, 0)
	height = max(height, 0)
	b := &Buffer{cells: make([]Cell, width*height), width: width, height: height}
	for i := range b.cells {
		b.cells[i] = BlankCell()
	}
	return b
}

// Width returns b's width in cells.
func (b *Buffer) Width() int { return b.width }

// Height returns b's height in cells.
func (b *Buffer) Height() int { return b.height }

// Bounds returns b's cell rectangle.
func (b *Buffer) Bounds() layout.Rectangle { return layout.Rect(0, 0, b.width, b.height) }

// Line returns row y, or nil when y is out of bounds.
func (b *Buffer) Line(y int) []Cell {
	if y < 0 || y >= b.height {
		return nil
	}
	start := y * b.width
	return b.cells[start : start+b.width]
}

// CellAt returns the cell at (x, y), or nil when it is out of bounds.
func (b *Buffer) CellAt(x, y int) *Cell {
	if !layout.Pos(x, y).In(b.Bounds()) {
		return nil
	}
	return &b.cells[y*b.width+x]
}

// SetCell writes c at (x, y) while preserving wide-cell invariants.
//
// A continuation passed on its own is ignored, since the wide cell to its left
// places it as part of that cell's own write. Writing one neither moves nor
// clears part of a wide cell, and the wide cell under it survives.
func (b *Buffer) SetCell(x, y int, c *Cell) {
	if !layout.Pos(x, y).In(b.Bounds()) {
		return
	}
	cell := cellOrBlank(c)
	// A continuation is placed by the wide cell to its left as part of that
	// cell's own write, so one arriving on its own has nothing to say that the
	// owning write did not already say. Ignoring it keeps the column as its
	// owner left it, which is what a caller mirroring a buffer cell by cell
	// wants: it walks continuations because they are in the grid, not because
	// it means to move them.
	//
	// Ignoring is the only safe answer. Storing the cell lets a column carry a
	// style its owner never had, and destroys a wide primary written over.
	// Blanking puts a one-column cell where the owner accounts for more, so the
	// row emits more columns than it holds.
	if cell.IsContinuation() {
		return
	}
	line := b.Line(y)
	if x > 0 && line[x].IsContinuation() {
		pc := x - 1
		for pc > 0 && line[pc].IsContinuation() {
			pc--
		}
		if line[pc].IsWide() {
			// The bound has to be taken before the fill. This loop overwrites
			// the very cell it measures, so re-reading its width mid-loop asks
			// the blank how wide the owner was and stops one column in, leaving
			// the rest of the cluster behind as orphan continuations.
			end := pc + min(line[pc].Width(), b.width-pc)
			blank := BlankCell().WithStyle(line[pc].Style)
			for i := pc; i < end; i++ {
				line[i] = blank
			}
		} else if line[pc].IsContinuation() {
			line[x] = BlankCell()
		}
	}
	if line[x].IsWide() {
		end := x + min(line[x].Width(), b.width-x)
		blank := BlankCell().WithStyle(line[x].Style)
		for i := x + 1; i < end; i++ {
			line[i] = blank
		}
	}
	if cell.IsWide() {
		// Reject a partial placement before it creates orphan continuations.
		if cell.Width() > b.width-x {
			line[x] = BlankCell().WithStyle(cell.Style)
			return
		}
		for i := x + 1; i < x+cell.Width(); i++ {
			if line[i].IsWide() {
				end := i + min(line[i].Width(), b.width-i)
				blank := BlankCell().WithStyle(line[i].Style)
				for j := i + 1; j < end; j++ {
					line[j] = blank
				}
			}
			line[i] = ContinuationCell().WithStyle(cell.Style)
		}
	}
	line[x] = cell
}

// Set is an alias for SetCell.
func (b *Buffer) Set(x, y int, c *Cell) { b.SetCell(x, y, c) }

// Resize changes b's dimensions, preserving the top-left intersection.
func (b *Buffer) Resize(width, height int) {
	width = max(width, 0)
	height = max(height, 0)
	if width == b.width && height == b.height {
		return
	}
	cells := make([]Cell, width*height)
	for i := range cells {
		cells[i] = BlankCell()
	}
	copyW, copyH := min(width, b.width), min(height, b.height)
	for y := 0; y < copyH; y++ {
		copy(cells[y*width:y*width+copyW], b.cells[y*b.width:y*b.width+copyW])
	}
	b.cells, b.width, b.height = cells, width, height
}

// Fill fills b with c.
func (b *Buffer) Fill(c *Cell) { Fill(b, c) }

// FillRect fills r with c.
func (b *Buffer) FillRect(r layout.Rectangle, c *Cell) { FillRect(b, r, c) }

// Clear fills b with BlankCell.
func (b *Buffer) Clear() { Clear(b) }

// ClearRect fills r with BlankCell.
func (b *Buffer) ClearRect(r layout.Rectangle) { ClearRect(b, r) }

// Draw copies b into dst with b's top-left mapped to (x, y).
func (b *Buffer) Draw(dst Surface, x, y int) { Draw(b, dst, x, y) }

// InsertLines inserts n rows at y within [y, bottom).
func (b *Buffer) InsertLines(y, n, bottom int, fill *Cell) { InsertLines(b, y, n, bottom, fill) }

// DeleteLines deletes n rows at y within [y, bottom).
func (b *Buffer) DeleteLines(y, n, bottom int, fill *Cell) { DeleteLines(b, y, n, bottom, fill) }

// InsertCells inserts n cells at (x, y) within [x, right).
func (b *Buffer) InsertCells(x, y, n, right int, fill *Cell) {
	InsertCells(b, x, y, n, right, fill)
}

// DeleteCells deletes n cells at (x, y) within [x, right).
func (b *Buffer) DeleteCells(x, y, n, right int, fill *Cell) {
	DeleteCells(b, x, y, n, right, fill)
}
