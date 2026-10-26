package buffer

import (
	"altscreen.io/vision/layout"
)

// Surface is a rectangular, readable and writable terminal cell grid.
//
// A surface stores cells and nothing else. Painting text onto one needs a
// width policy, which [altscreen.io/vision/text.Surface] adds.
type Surface interface {
	Bounds() layout.Rectangle
	CellAt(x, y int) *Cell
	SetCell(x, y int, c *Cell)
}

func cellOrBlank(c *Cell) Cell {
	if c == nil {
		return BlankCell()
	}
	return *c
}

// Width returns s's bounds width.
func Width(s Surface) int { return s.Bounds().Dx() }

// Height returns s's bounds height.
func Height(s Surface) int { return s.Bounds().Dy() }

// Contains reports whether (x, y) is inside s's bounds.
func Contains(s Surface, x, y int) bool { return layout.Pos(x, y).In(s.Bounds()) }

// Fill fills s with c.
func Fill(s Surface, c *Cell) { FillRect(s, s.Bounds(), c) }

// FillRect fills the intersection of r and s's bounds with c.
// A partial slot at the right edge becomes a blank with c's style.
func FillRect(s Surface, r layout.Rectangle, c *Cell) {
	clipped := s.Bounds().Intersect(r)
	if clipped.Empty() {
		return
	}
	fill := cellOrBlank(c)
	blank := BlankCell().WithStyle(fill.Style)
	step := max(fill.Width(), 1)
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		for x := clipped.Min.X; x < clipped.Max.X; {
			if step <= clipped.Max.X-x {
				s.SetCell(x, y, &fill)
				x += step
			} else {
				s.SetCell(x, y, &blank)
				x++
			}
		}
	}
}

// Clear fills s with BlankCell.
func Clear(s Surface) {
	blankCell := BlankCell()
	Fill(s, &blankCell)
}

// ClearRect fills r with BlankCell.
func ClearRect(s Surface, r layout.Rectangle) {
	blankCell := BlankCell()
	FillRect(s, r, &blankCell)
}

// Draw copies src into dst with src's top-left mapped to (x, y).
// Wide cells clipped by either surface become blanks in the visible columns.
func Draw(src, dst Surface, x, y int) {
	blankCell := BlankCell()

	b := src.Bounds()
	db := dst.Bounds()
	for sy := b.Min.Y; sy < b.Max.Y; sy++ {
		for sx := b.Min.X; sx < b.Max.X; {
			dx, dy := x+sx-b.Min.X, y+sy-b.Min.Y
			cell := src.CellAt(sx, sy)
			if cell == nil {
				sx++
				continue
			}
			if cell.IsContinuation() {
				dst.SetCell(dx, dy, &blankCell)
				sx++
				continue
			}
			w := max(cell.Width(), 1)
			if w > 1 && (w > b.Max.X-sx || dx < db.Min.X || dx >= db.Max.X || w > db.Max.X-dx) {
				dst.SetCell(dx, dy, &blankCell)
				sx++
				continue
			}
			dst.SetCell(dx, dy, cell)
			sx += w
		}
	}
}

// InsertLines inserts n rows at y within [y, bottom).
func InsertLines(s Surface, y, n, bottom int, fill *Cell) {
	b := s.Bounds()
	bottom = min(bottom, b.Max.Y)
	if n <= 0 || y < b.Min.Y || y >= bottom {
		return
	}
	n = min(n, bottom-y)
	for dstY := bottom - 1; dstY >= y+n; dstY-- {
		copyRow(s, dstY-n, dstY, b.Min.X, b.Max.X)
	}
	for row := y; row < y+n; row++ {
		fillSpan(s, row, b.Min.X, b.Max.X, fill)
	}
}

// DeleteLines deletes n rows at y within [y, bottom).
func DeleteLines(s Surface, y, n, bottom int, fill *Cell) {
	b := s.Bounds()
	bottom = min(bottom, b.Max.Y)
	if n <= 0 || y < b.Min.Y || y >= bottom {
		return
	}
	n = min(n, bottom-y)
	for dstY := y; dstY < bottom-n; dstY++ {
		copyRow(s, dstY+n, dstY, b.Min.X, b.Max.X)
	}
	for row := bottom - n; row < bottom; row++ {
		fillSpan(s, row, b.Min.X, b.Max.X, fill)
	}
}

// InsertCells inserts n cells at (x, y) within [x, right).
func InsertCells(s Surface, x, y, n, right int, fill *Cell) {
	b := s.Bounds()
	right = min(right, b.Max.X)
	if n <= 0 || !layout.Pos(x, y).In(b) || x >= right {
		return
	}
	n = min(n, right-x)
	primaries := collectPrimaries(s, y, x, right)
	blankSpan(s, y, x, right)
	for i := len(primaries) - 1; i >= 0; i-- {
		pc := primaries[i]
		dx := pc.x + n
		w := max(pc.cell.Width(), 1)
		if dx < right && w <= right-dx {
			s.SetCell(dx, y, &pc.cell)
		}
	}
	fillSpan(s, y, x, x+n, fill)
}

// DeleteCells deletes n cells at (x, y) within [x, right).
func DeleteCells(s Surface, x, y, n, right int, fill *Cell) {
	b := s.Bounds()
	right = min(right, b.Max.X)
	if n <= 0 || !layout.Pos(x, y).In(b) || x >= right {
		return
	}
	n = min(n, right-x)
	primaries := collectPrimaries(s, y, x, right)
	blankSpan(s, y, x, right)
	cut := x + n
	for _, pc := range primaries {
		if pc.x < cut {
			continue
		}
		dx := pc.x - n
		if max(pc.cell.Width(), 1) <= right-dx {
			s.SetCell(dx, y, &pc.cell)
		}
	}
	fillSpan(s, y, right-n, right, fill)
}

type primaryCell struct {
	x    int
	cell Cell
}

func collectPrimaries(s Surface, y, left, right int) []primaryCell {
	var out []primaryCell
	for x := left; x < right; {
		cell := s.CellAt(x, y)
		if cell != nil && !cell.IsContinuation() {
			out = append(out, primaryCell{x: x, cell: *cell})
			x += max(cell.Width(), 1)
			continue
		}
		x++
	}
	return out
}

func copyRow(s Surface, srcY, dstY, left, right int) {
	blankCell := BlankCell()

	for x := left; x < right; {
		cell := s.CellAt(x, srcY)
		if cell == nil || cell.IsContinuation() {
			s.SetCell(x, dstY, &blankCell)
			x++
			continue
		}
		w := max(cell.Width(), 1)
		if w > 1 && w > right-x {
			s.SetCell(x, dstY, &blankCell)
			x++
			continue
		}
		s.SetCell(x, dstY, cell)
		x += w
	}
}

func blankSpan(s Surface, y, left, right int) {
	blankCell := BlankCell()

	for x := left; x < right; x++ {
		s.SetCell(x, y, &blankCell)
	}
}

func fillSpan(s Surface, y, left, right int, fill *Cell) {
	blankCell := BlankCell()

	if left >= right {
		return
	}
	f := cellOrBlank(fill)
	step := max(f.Width(), 1)
	for x := left; x < right; {
		if step <= right-x {
			s.SetCell(x, y, &f)
			x += step
		} else {
			s.SetCell(x, y, &blankCell)
			x++
		}
	}
}
