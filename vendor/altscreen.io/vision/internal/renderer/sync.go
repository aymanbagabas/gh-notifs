package renderer

// SyncFront copies touched front cells into the staging buffer and returns whether rendering has work.
func (r *Renderer) SyncFront(front *RenderBuffer) bool {
	width, height := front.Width(), front.Height()
	if r.backBuf.Width() != width || r.backBuf.Height() != height {
		r.backBuf.Resize(width, height)
		r.backBuf.ClearTouched()
	}
	for y := 0; y < height; y++ {
		span, ok := front.Touched(y)
		if !ok {
			continue
		}
		for x := span.First; x <= span.Last; {
			cell := front.CellAt(x, y)
			if cell == nil {
				x++
				continue
			}
			// Skip continuation columns. Copying the primary places the
			// continuation on the far side as part of the same write, so the
			// column already carries the value this loop would ask for.
			// SetCell ignores a continuation offered on its own, so passing
			// one on would be a wasted call rather than a wrong one, and
			// stepping over it keeps the walk in step with the widths it
			// advances by.
			if cell.IsContinuation() {
				x++
				continue
			}
			r.backBuf.SetCell(x, y, cell)
			x += max(cell.Width(), 1)
		}
	}
	front.ClearTouched()
	return r.forceClear || r.backBuf.HasChanges()
}
