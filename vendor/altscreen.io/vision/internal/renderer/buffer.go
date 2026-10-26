package renderer

import (
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
)

// TouchedSpan is the inclusive modified-column span for one row.
type TouchedSpan struct{ First, Last int }

// RenderBuffer wraps a cell buffer with per-row dirty spans.
type RenderBuffer struct {
	Buffer  *buffer.Buffer
	touched []TouchedSpan
	set     []bool
}

// NewRenderBuffer returns a blank render buffer with no touched rows.
func NewRenderBuffer(width, height int) *RenderBuffer {
	return &RenderBuffer{Buffer: buffer.New(width, height), touched: make([]TouchedSpan, max(height, 0)), set: make([]bool, max(height, 0))}
}

func (b *RenderBuffer) Bounds() layout.Rectangle     { return b.Buffer.Bounds() }
func (b *RenderBuffer) Width() int                   { return b.Buffer.Width() }
func (b *RenderBuffer) Height() int                  { return b.Buffer.Height() }
func (b *RenderBuffer) CellAt(x, y int) *buffer.Cell { return b.Buffer.CellAt(x, y) }
func (b *RenderBuffer) Line(y int) []buffer.Cell     { return b.Buffer.Line(y) }

// Touch marks one cell position as changed.
func (b *RenderBuffer) Touch(x, y int) { b.TouchLine(y, x, x) }

// TouchLine marks an inclusive range on one row as changed.
func (b *RenderBuffer) TouchLine(y, first, last int) {
	if y < 0 || y >= len(b.touched) || first > last {
		return
	}
	if b.set[y] {
		if first < b.touched[y].First {
			b.touched[y].First = first
		}
		if last > b.touched[y].Last {
			b.touched[y].Last = last
		}
		return
	}
	b.touched[y] = TouchedSpan{First: first, Last: last}
	b.set[y] = true
}

func (b *RenderBuffer) TouchFullLine(y int) {
	if b.Width() > 0 {
		b.TouchLine(y, 0, b.Width()-1)
	}
}

func (b *RenderBuffer) Touched(y int) (TouchedSpan, bool) {
	if y < 0 || y >= len(b.touched) || !b.set[y] {
		return TouchedSpan{}, false
	}
	return b.touched[y], true
}

func (b *RenderBuffer) HasChanges() bool {
	for _, ok := range b.set {
		if ok {
			return true
		}
	}
	return false
}

func (b *RenderBuffer) TouchedLineCount() int {
	n := 0
	for _, ok := range b.set {
		if ok {
			n++
		}
	}
	return n
}

func (b *RenderBuffer) ClearTouched() {
	for i := range b.set {
		b.set[i] = false
	}
}

func (b *RenderBuffer) TouchAll() {
	if b.Width() == 0 {
		return
	}
	for y := 0; y < b.Height(); y++ {
		b.TouchLine(y, 0, b.Width()-1)
	}
}

func (b *RenderBuffer) Resize(width, height int) {
	b.Buffer.Resize(width, height)
	h := max(height, 0)
	b.touched = make([]TouchedSpan, h)
	b.set = make([]bool, h)
	b.TouchAll()
}

// SetCell writes c and touches the occupied columns when the cell changed.
//
// When a wide cell is overwritten by a narrower one, the touched span covers
// the whole cluster being broken: both the column written and the primary that
// owns it, which the write blanks.
//
// A continuation is placed by the cell that owns it, so [buffer.Buffer.SetCell]
// ignores one arriving on its own and this records no damage for it.
func (b *RenderBuffer) SetCell(x, y int, c *buffer.Cell) {
	p := layout.Pos(x, y)
	if !p.In(b.Bounds()) {
		return
	}
	cell := buffer.BlankCell()
	if c != nil {
		cell = *c
	}
	// The buffer leaves the column as its owner wrote it, so there is nothing
	// for the diff to look at.
	if cell.IsContinuation() {
		return
	}
	existing := b.Buffer.CellAt(x, y)
	// An equal cell is only a no-op when the write would change nothing, and a
	// wide cell writes a continuation beside itself. Resize truncates a pair
	// when the row shrinks and leaves the primary behind, so the two columns
	// can disagree with a primary that already matches, and returning here
	// would leave the second one holding whatever the truncation left.
	if existing != nil && existing.Equal(cell) && b.pairIntact(p, cell) {
		return
	}
	prevW := 1
	if existing != nil {
		prevW = max(existing.Width(), 1)
	}
	w := max(max(cell.Width(), 1), prevW)
	// Writing over a continuation blanks the primary that owns it, so the
	// damage starts there. The buffer walks back over chained continuations to
	// find that primary and blanks only when it is wide, so this follows the
	// same rule: a continuation beside a narrow cell, or one that reaches the
	// left edge, leaves its neighbour untouched and the damage may not claim
	// it.
	first := b.ownedFrom(p)
	// The far end closes the same way. A write covering a wide primary blanks
	// the continuation that primary owns, which can lie past the columns the
	// write itself covers.
	last := b.ownedThrough(layout.Pos(x+min(w, b.Width()-x)-1, y))
	b.Buffer.SetCell(x, y, &cell)
	// The buffer truncates a cell that does not fit, writing a blank where the
	// second half would have gone, so the span must not claim that column
	// either.
	b.TouchLine(y, first, min(last, b.Width()-1))
}

func (b *RenderBuffer) Fill(c *buffer.Cell)               { buffer.Fill(b, c) }
func (b *RenderBuffer) Clear()                            { buffer.Clear(b) }
func (b *RenderBuffer) ClearRect(r layout.Rectangle)      { buffer.ClearRect(b, r) }
func (b *RenderBuffer) Draw(dst buffer.Surface, x, y int) { buffer.Draw(b, dst, x, y) }

func (b *RenderBuffer) FillRect(r layout.Rectangle, c *buffer.Cell) {
	clipped := b.Bounds().Intersect(r)
	if clipped.Empty() {
		return
	}
	// A fill starting on a continuation makes the buffer walk back to the
	// primary that owns it and blank from there, which can be a column outside
	// the rect. Ask each row where its damage really begins before the fill
	// rewrites the evidence. TouchLine only merges a span into the row's
	// record, so it does not care that the fill has not run yet.
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		b.TouchLine(y,
			b.ownedFrom(layout.Pos(clipped.Min.X, y)),
			b.ownedThrough(layout.Pos(clipped.Max.X-1, y)))
	}
	b.Buffer.FillRect(r, c)
}

// ownedFrom returns the column a write at p starts changing, which is p itself
// unless p holds a continuation whose owning primary lies further left. It
// reads the row as it stands, so callers must ask before writing.
func (b *RenderBuffer) ownedFrom(p layout.Position) int {
	existing := b.Buffer.CellAt(p.X, p.Y)
	if existing == nil || !existing.IsContinuation() || p.X == 0 {
		return p.X
	}
	line := b.Buffer.Line(p.Y)
	pc := p.X - 1
	for pc > 0 && line[pc].IsContinuation() {
		pc--
	}
	if line[pc].IsWide() {
		return pc
	}
	return p.X
}

// pairIntact reports whether the owner fits and every continuation has its style.
// A one-column cell has no continuations and is always intact.
func (b *RenderBuffer) pairIntact(p layout.Position, cell buffer.Cell) bool {
	if !cell.IsWide() {
		return true
	}
	if cell.Width() > b.Width()-p.X {
		return false
	}
	end := p.X + cell.Width()
	for x := p.X + 1; x < end; x++ {
		next := b.Buffer.CellAt(x, p.Y)
		if !next.IsContinuation() || !next.Style.Equal(cell.Style) {
			return false
		}
	}
	return true
}

// ownedThrough returns the last column changed by a write at p.
// A write over a primary or its continuation clears the whole cluster.
//
// The mirror of [RenderBuffer.ownedFrom], and it reads the row as it stands for
// the same reason: the write is what destroys the evidence.
func (b *RenderBuffer) ownedThrough(p layout.Position) int {
	if p.X < 0 {
		return p.X
	}
	existing := b.Buffer.CellAt(p.X, p.Y)
	if existing == nil {
		return p.X
	}
	owner := b.ownedFrom(p)
	cell := b.Buffer.CellAt(owner, p.Y)
	if !cell.IsWide() {
		return p.X
	}
	return max(p.X, owner+min(cell.Width(), b.Width()-owner)-1)
}

func (b *RenderBuffer) InsertLines(y, n, bottom int, fill *buffer.Cell) {
	b.Buffer.InsertLines(y, n, bottom, fill)
	bot := min(bottom, b.Height())
	if y >= bot || b.Width() == 0 {
		return
	}
	for row := y; row < bot; row++ {
		b.TouchFullLine(row)
	}
}

func (b *RenderBuffer) DeleteLines(y, n, bottom int, fill *buffer.Cell) {
	b.Buffer.DeleteLines(y, n, bottom, fill)
	bot := min(bottom, b.Height())
	if y >= bot || b.Width() == 0 {
		return
	}
	for row := y; row < bot; row++ {
		b.TouchFullLine(row)
	}
}

// InsertCells shifts a row right. Both ends of the damage close over the
// clusters the shift breaks, for the same reason a write does: the row is read
// before the shift, which is what destroys the evidence.
func (b *RenderBuffer) InsertCells(x, y, n, right int, fill *buffer.Cell) {
	first, last := b.shiftSpan(layout.Pos(x, y), right)
	b.Buffer.InsertCells(x, y, n, right, fill)
	b.TouchLine(y, first, last)
}

// DeleteCells shifts a row left, and closes its damage the way InsertCells does.
func (b *RenderBuffer) DeleteCells(x, y, n, right int, fill *buffer.Cell) {
	first, last := b.shiftSpan(layout.Pos(x, y), right)
	b.Buffer.DeleteCells(x, y, n, right, fill)
	b.TouchLine(y, first, last)
}

// shiftSpan returns the columns a row shift bounded by right actually changes.
func (b *RenderBuffer) shiftSpan(p layout.Position, right int) (int, int) {
	return b.ownedFrom(p), b.ownedThrough(layout.Pos(min(right, b.Width())-1, p.Y))
}
