package renderer

import (
	"io"
	"strings"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
)

func canClearWith(cell buffer.Cell, bce bool) bool {
	if cell.Width() != 1 || cell.Content() != " " || cell.Style.HasLink() {
		return false
	}
	allowed := style.Bold | style.Faint | style.Italic | style.SlowBlink | style.RapidBlink
	if cell.Style.GetUnderlineStyle() != style.UnderlineNone || cell.Style.GetAttrs()&^allowed != 0 {
		return false
	}
	return bce || (cell.Style.GetBackground() == nil && cell.Style.GetUnderlineColor() == nil)
}

func cellsEqualBlank(cell, blank buffer.Cell) bool {
	return !cell.IsContinuation() && cell.Width() == blank.Width() && (cell.Content() == blank.Content() || (cell.Content() == "" && blank.Content() == " ") || (cell.Content() == " " && blank.Content() == "")) && cell.Style.Equal(blank.Style)
}
func isRepASCII(s string) bool { return len(s) == 1 && s[0] >= 0x1f && s[0] < 0x7f }
func cellEqualPtr(line []buffer.Cell, i int, c buffer.Cell) bool {
	return i >= 0 && i < len(line) && line[i].Equal(c)
}

func getCell(line []buffer.Cell, i int) (buffer.Cell, bool) {
	if i < 0 || i >= len(line) {
		return buffer.Cell{}, false
	}
	return line[i], true
}

func cellsEq(a buffer.Cell, aok bool, b buffer.Cell, bok bool) bool { return aok && bok && a.Equal(b) }

// clusterStart returns the first column at or before at that owns the cell it
// holds.
//
// The diff finds its boundaries by comparing columns, so one can land inside a
// cluster. A terminal draws a glyph or it does not, so emission has to begin on
// a whole one: the cursor is placed at a run's first column, and a continuation
// writes no bytes and moves no cursor, which would leave every glyph after it a
// column to the left of where it belongs.
func clusterStart(line []buffer.Cell, at int) int {
	if len(line) == 0 {
		return 0
	}
	here := min(at, len(line)-1)
	start := here
	for start > 0 && line[start].IsContinuation() {
		start--
	}
	// Only a wide cell owns the columns past itself, so a walk that ends
	// anywhere else ends on a continuation nothing owns: the left edge, which
	// [buffer.Buffer.Resize] and the shifting operations both leave behind, or
	// a narrow cell that a left shift moved in beside one. Either way it
	// belongs to no cluster and stands as its own column rather than being
	// folded into one that is not there.
	//
	// A wide cell owns exactly the columns its width accounts for, so a
	// continuation lying past the pair is unowned too, and for the same
	// reason. [clusterEnd] bounds its walk the same way, which is what makes
	// the two answer alike about the same column.
	if start != here && (!line[start].IsWide() || here >= start+line[start].Width()) {
		return here
	}
	return start
}

// clusterEnd returns the last column of the cluster that owns at.
//
// The mirror of [clusterStart]. A run ending on a cell whose continuations lie
// past it would cover part of a glyph, and which end a change exposes depends
// on the direction it grew in, so both are closed.
func clusterEnd(line []buffer.Cell, at int) int {
	// The mirror of the ownership rule in [clusterStart]. A cell that is not
	// wide draws one column and owns nothing past itself, so its cluster ends
	// on it even when a continuation nothing owns follows. A wide cell owns
	// exactly the columns its width accounts for and no more, so the walk
	// stops there rather than swallowing a continuation past the pair.
	if at < 0 || at >= len(line) || !line[at].IsWide() {
		return at
	}
	owned := at + line[at].Width() - 1
	end := at
	for end < owned && end+1 < len(line) && line[end+1].IsContinuation() {
		end++
	}
	return end
}

func (r *Renderer) transformLine(w io.Writer, newBuf *RenderBuffer, y, firstX, lastX int) error {
	if r.curBuf == nil {
		return nil
	}
	curLine := r.curBuf.Line(y)
	first, changed, err := r.transformLineInner(w, newBuf, curLine, y, firstX, lastX)
	if err != nil {
		return err
	}
	if changed {
		if nl := newBuf.Line(y); nl != nil && curLine != nil {
			if first < len(curLine) && first < len(nl) {
				copy(curLine[first:min(len(curLine), len(nl))], nl[first:min(len(curLine), len(nl))])
			} else if len(curLine) > 0 && len(nl) > 0 {
				copy(curLine[:min(len(curLine), len(nl))], nl[:min(len(curLine), len(nl))])
			}
		}
	}
	return nil
}

func (r *Renderer) transformLineInner(w io.Writer, newBuf *RenderBuffer, curLine []buffer.Cell, y, _, _ int) (int, bool, error) {
	width := newBuf.Width()
	newLine := newBuf.Line(y)
	if width == 0 || len(newLine) == 0 {
		return 0, false, nil
	}
	firstDiff := 0
	for firstDiff < width && cellEqualPtr(curLine, firstDiff, newLine[firstDiff]) {
		firstDiff++
	}
	if firstDiff == width {
		return 0, false, nil
	}
	if bail, ok := r.uncertainBail(newLine, curLine, firstDiff); ok {
		first := clusterStart(newLine, min(firstDiff, bail))
		if first < bail {
			if err := r.moveTo(w, newBuf, y, first); err != nil {
				return 0, false, err
			}
			if _, err := r.putRange(w, newBuf, curLine, newLine, y, first, bail-1); err != nil {
				return 0, false, err
			}
		}
		if err := r.repaintTail(w, newBuf, newLine, y, bail); err != nil {
			return 0, false, err
		}
		return first, true, nil
	}
	first := 0
	copyFrom := 0
	leading := newLine[0]
	if canClearWith(leading, r.opts.Contains(OptBCE)) {
		oFirst := 0
		for oFirst < len(curLine) && curLine[oFirst].Equal(leading) {
			oFirst++
		}
		nFirst := 0
		for nFirst < len(newLine) && newLine[nFirst].Equal(leading) {
			nFirst++
		}
		if nFirst == oFirst {
			first = firstDiff
			copyFrom = first
		} else if oFirst > nFirst {
			first = nFirst
			copyFrom = first
		} else {
			first = oFirst
			copyFrom = first
			lead := nFirst - oFirst
			if 4 < lead {
				if nFirst >= width {
					if err := r.moveTo(w, newBuf, y, 0); err != nil {
						return 0, false, err
					}
					if err := r.updatePen(w, &leading); err != nil {
						return 0, false, err
					}
					io.WriteString(w, ansi.EraseToEOL())
				} else {
					if err := r.moveTo(w, newBuf, y, nFirst-1); err != nil {
						return 0, false, err
					}
					if err := r.updatePen(w, &leading); err != nil {
						return 0, false, err
					}
					io.WriteString(w, ansi.EraseLineLeft)
				}
				fill := *r.cur.currentBCEBlank(r.opts.Contains(OptBCE))
				end := min(nFirst, len(curLine))
				for i := first; i < end; i++ {
					curLine[i] = fill
				}
				first = nFirst
			}
		}
	} else {
		first = firstDiff
		copyFrom = first
	}
	if first >= width {
		return 0, false, nil
	}
	// The scan compares columns, so it can stop on the second half of a
	// cluster: a continuation whose lead matched. Emission begins by placing
	// the cursor at this column, and a continuation writes no bytes and moves
	// no cursor, so the next glyph would go out a column to the left of where
	// it belongs. Backing up to the column that owns the cell re-emits the
	// whole cluster instead.
	//
	// copyFrom follows, because the caller mirrors newLine[copyFrom:] into the
	// back buffer. Left where it was, the re-emitted lead would never be
	// recorded and the same difference would be found again on every later
	// frame.
	if owner := clusterStart(newLine, first); owner != first {
		first = owner
		copyFrom = min(copyFrom, owner)
	}
	trailing := newLine[width-1]
	if !canClearWith(trailing, r.opts.Contains(OptBCE)) {
		nLast := width - 1
		for nLast > first {
			old, ok := getCell(curLine, nLast)
			if ok && old.Equal(newLine[nLast]) {
				nLast--
			} else {
				break
			}
		}
		// Close the range on the cluster that owns nLast. The walk stops on the
		// last column where the rows differ, so a wide lead there is extended
		// to its continuation and putRange ends on the whole glyph rather than
		// on the lead's own column.
		//
		// This moves nLast often yet never changes the emitted bytes, which is
		// worth saying so no one reads it as a guard: the lead it lands on is a
		// changed cell the emitter draws whole either way, while the
		// continuation it reaches carries nothing, so the added column emits
		// nothing. clusterEnd only extends forward from a lead, so it cannot
		// pull a stop back off a continuation; it does not need to, because
		// every column past nLast already matched, which leaves any lead owning
		// one of them inside the range. It is kept so every range putRange
		// receives is closed on a whole cluster at both ends, as the blank path
		// below is.
		nLast = clusterEnd(newLine, nLast)
		if nLast >= first {
			if err := r.moveTo(w, newBuf, y, first); err != nil {
				return 0, false, err
			}
			if _, err := r.putRange(w, newBuf, curLine, newLine, y, first, nLast); err != nil {
				return 0, false, err
			}
		}
		if copyFrom <= nLast && nLast < len(curLine) {
			copy(curLine[copyFrom:nLast+1], newLine[copyFrom:nLast+1])
			return 0, false, nil
		}
		return copyFrom, true, nil
	}
	blank := trailing
	oLast := width - 1
	for oLast > first && cellEqualPtr(curLine, oLast, blank) {
		oLast--
	}
	nLast := width - 1
	for nLast > first && newLine[nLast].Equal(blank) {
		nLast--
	}
	// Close both walk-backs over the cluster that owns their stopping point.
	//
	// Neither can move on this path, and that is worth stating rather than
	// leaving the reader to assume they are protecting something: blank is
	// narrow here, since canClearWith required it, and a continuation is never
	// equal to a narrow blank, so each walk-back already stopped on a column
	// whose successor is blank and therefore narrow. They are kept because the
	// branches below all treat nLast+1 and oLast+1 as resting columns, and a
	// future change to either walk would need the close to be here.
	nLast = clusterEnd(newLine, nLast)
	oLast = clusterEnd(curLine, oLast)
	el0 := r.el0Cost()
	if nLast == first && el0 < oLast-nLast {
		if err := r.moveTo(w, newBuf, y, first); err != nil {
			return 0, false, err
		}
		if !newLine[first].Equal(blank) {
			// Close the range over the cluster first owns. This cannot move
			// either: reaching here needs nLast == first, and nLast was just
			// closed, so first is already a cluster end. It is kept so the
			// range passed to emitRange is closed at both ends wherever it is
			// built, rather than only where the close happens to be needed.
			if _, err := r.emitRange(w, newBuf, newLine, first, clusterEnd(newLine, first)); err != nil {
				return 0, false, err
			}
		}
		if err := r.clearToEnd(w, curLine, blank, width, false); err != nil {
			return 0, false, err
		}
	} else {
		oldOL, okOL := getCell(curLine, oLast)
		if nLast != oLast && (!okOL || !newLine[nLast].Equal(oldOL)) {
			if err := r.moveTo(w, newBuf, y, first); err != nil {
				return 0, false, err
			}
			if oLast > nLast && oLast-nLast > el0 {
				eoi, err := r.putRange(w, newBuf, curLine, newLine, y, first, nLast)
				if err != nil {
					return 0, false, err
				}
				if eoi {
					if err := r.moveTo(w, newBuf, y, nLast+1); err != nil {
						return 0, false, err
					}
				}
				if err := r.clearToEnd(w, curLine, blank, width, false); err != nil {
					return 0, false, err
				}
			} else {
				n := max(nLast, oLast)
				if _, err := r.putRange(w, newBuf, curLine, newLine, y, first, n); err != nil {
					return 0, false, err
				}
			}
		} else {
			nLastNB, oLastNB := nLast, oLast
			nlc, olc := nLast, oLast
			for {
				nc, nok := getCell(newLine, nlc)
				oc, ook := getCell(curLine, olc)
				if !cellsEq(nc, nok, oc, ook) {
					break
				}
				np, nop := getCell(newLine, nlc-1)
				op, oop := getCell(curLine, olc-1)
				if !cellsEq(np, nop, op, oop) {
					break
				}
				nlc--
				olc--
				if nlc == -1 || olc == -1 {
					break
				}
			}
			n := min(olc, nlc)
			insertAt := n + 1
			if olc < nlc && clusterStart(newLine, insertAt) < insertAt {
				if err := r.moveTo(w, newBuf, y, first); err != nil {
					return 0, false, err
				}
				if _, err := r.putRange(w, newBuf, curLine, newLine, y, first, max(nLastNB, oLastNB)); err != nil {
					return 0, false, err
				}
				return copyFrom, true, nil
			}
			if n >= first {
				// The walk stops where the rows agree again, which can be a
				// cell that owns a column past it. emitRange draws whole
				// glyphs, so the run reaches further than n and the cursor
				// rests past the cluster; the insert branch below then
				// back-shifts over the continuation and draws the same glyph a
				// second time. Ending on the cluster's own last column keeps
				// the two in step.
				last := clusterEnd(newLine, n)
				if err := r.moveTo(w, newBuf, y, first); err != nil {
					return 0, false, err
				}
				if _, err := r.putRange(w, newBuf, curLine, newLine, y, first, last); err != nil {
					return 0, false, err
				}
			}
			if olc < nlc {
				m := max(nLastNB, oLastNB)
				nn := n
				o := olc
				if nn != 0 {
					for nn > 0 {
						c, ok := getCell(newLine, nn+1)
						if ok && c.IsContinuation() {
							nn--
							o--
						} else {
							break
						}
					}
				}
				// Whichever way the walk went, the insert begins at nn+1, and
				// that column has to own the cell it holds. The backward walk
				// stops when it reaches column zero, and it only runs at all
				// away from it, so either can leave the insert pointing inside
				// a glyph. Stepping past the cluster moves o with it, keeping
				// the two rows in lockstep.
				for {
					c, ok := getCell(newLine, nn+1)
					if ok && c.IsContinuation() {
						nn++
						o++
					} else {
						break
					}
				}
				if err := r.moveTo(w, newBuf, y, nn+1); err != nil {
					return 0, false, err
				}
				cnt := nlc - o
				ichCost := ICHCost(cnt)
				span := m - nn
				if r.opts.Contains(OptICH) && (nlc < nLastNB || ichCost > span) {
					if _, err := r.putRange(w, newBuf, curLine, newLine, y, nn+1, m); err != nil {
						return 0, false, err
					}
				} else {
					if err := r.insertCellsOp(w, newLine[nn+1:], cnt); err != nil {
						return 0, false, err
					}
				}
			} else if olc > nlc {
				// putRange emitted through n in the new row, and if newLine[n]
				// is a lead, its continuation was painted too and the cursor
				// rests past it, not at n+1. Moving to n+1 on a lead would step
				// back into the continuation of a glyph just written, and a DCH
				// there would cut the glyph in half. Slide to the column past
				// the cluster, where the wide-glyph emission already left the
				// cursor. The mirror of the insert branch's back-shift, taken
				// forward because deletion pulls content leftward.
				nEff := clusterEnd(newLine, n)
				if err := r.moveTo(w, newBuf, y, nEff+1); err != nil {
					return 0, false, err
				}
				cnt := olc - nlc
				tail := nLastNB - (nEff + 1)
				if !r.opts.Contains(OptDCH) || DCHCost(cnt) > ELCost(0)+tail {
					// nEff+1 may exceed nLastNB when the cluster covers the last
					// non-blank column; signed arithmetic keeps putRange a
					// no-op for the inverted span.
					if nEff+1 <= nLastNB {
						eoi, err := r.putRange(w, newBuf, curLine, newLine, y, nEff+1, nLastNB)
						if err != nil {
							return 0, false, err
						}
						if eoi {
							if err := r.moveTo(w, newBuf, y, nLastNB+1); err != nil {
								return 0, false, err
							}
						}
					}
					if err := r.clearToEnd(w, curLine, blank, width, false); err != nil {
						return 0, false, err
					}
				} else {
					if err := r.updatePen(w, &blank); err != nil {
						return 0, false, err
					}
					r.emitBuf = ansi.AppendDCH(r.emitBuf[:0], cnt)
					r.writeEmit(w)
				}
			}
		}
	}
	return copyFrom, true, nil
}

func (r *Renderer) clearBottom(w io.Writer, newBuf *RenderBuffer) (int, error) {
	height := newBuf.Height()
	if r.cur.style.HasLink() {
		return height, nil
	}
	bce := r.opts.Contains(OptBCE)
	if !canClearWith(*r.cur.currentBlank(), bce) {
		return height, nil
	}
	lastNon := height
	curClear := -1
	blank := *r.cur.currentBlank()
	for y := height - 1; y >= 0; y-- {
		nl := newBuf.Line(y)
		newBlank := true
		for _, c := range nl {
			if !cellsEqualBlank(c, blank) {
				newBlank = false
				break
			}
		}
		if !newBlank {
			lastNon = y + 1
			break
		}
		if r.curBuf != nil && curClear < 0 {
			cl := r.curBuf.Line(y)
			curBlank := true
			for _, c := range cl {
				if !cellsEqualBlank(c, blank) {
					curBlank = false
					break
				}
			}
			if !curBlank {
				curClear = y + 1
			}
		}
	}
	if lastNon == height {
		return height, nil
	}
	if curClear >= 0 {
		if err := r.clearBelow(w, newBuf, lastNon); err != nil {
			return height, err
		}
		if len(r.oldHashes) > 0 && len(r.oldHashes) == len(r.newHashes) {
			for y := lastNon; y < len(r.oldHashes); y++ {
				r.oldHashes[y] = r.newHashes[y]
			}
		}
	}
	return lastNon, nil
}

func (r *Renderer) clearScreen(w io.Writer) error {
	r.emitBuf = ansi.AppendCUP(r.emitBuf[:0], 0, 0)
	if err := r.writeEmit(w); err != nil {
		return err
	}
	if _, err := io.WriteString(w, ansi.EraseScreen()); err != nil {
		return err
	}
	r.cur.setPos(layout.Pos(0, 0))
	r.cur.atPhantom = false
	fill := *r.cur.currentBCEBlank(r.opts.Contains(OptBCE))
	if r.curBuf != nil {
		r.curBuf.Fill(&fill)
	}
	return nil
}

func (r *Renderer) clearBelow(w io.Writer, newBuf *RenderBuffer, row int) error {
	if err := r.moveToKeepingPen(w, newBuf, row, 0); err != nil {
		return err
	}
	return r.clearToBottom(w)
}

func (r *Renderer) clearToBottom(w io.Writer) error {
	io.WriteString(w, ansi.EraseBelow())
	row, col := r.cur.pos().Y, r.cur.pos().X
	fill := *r.cur.currentBCEBlank(r.opts.Contains(OptBCE))
	if r.curBuf != nil {
		if line := r.curBuf.Line(row); col < len(line) {
			for i := col; i < len(line); i++ {
				line[i] = fill
			}
		}
		for y := row + 1; y < r.curBuf.Height(); y++ {
			line := r.curBuf.Line(y)
			for i := range line {
				line[i] = fill
			}
		}
	}
	return nil
}

func (r *Renderer) el0Cost() int {
	if r.opts.Contains(OptBCE) {
		return 0
	}
	return ELCost(0)
}

func (r *Renderer) clearToEnd(w io.Writer, oldLine []buffer.Cell, blank buffer.Cell, width int, force bool) error {
	curX := r.cur.pos().X
	need := force || oldLine == nil
	if !need {
		for j := curX; j < width; j++ {
			if j >= len(oldLine) || !oldLine[j].Equal(blank) {
				need = true
				break
			}
		}
	}
	if !need {
		return nil
	}
	if err := r.updatePen(w, &blank); err != nil {
		return err
	}
	count := width - curX
	if r.el0Cost() <= count {
		_, err := io.WriteString(w, ansi.EraseToEOL())
		return err
	} else if count > 0 {
		if count > 1 {
			io.WriteString(w, r.blankGlyph(count-1))
			x := r.cur.pos().X + count - 1
			r.cur.setX(x)
		}
		return r.putGlyph(w, " ", 1, r.lastWidth, r.lastHeight)
	}
	return nil
}

func (r *Renderer) emitRange(w io.Writer, newBuf *RenderBuffer, line []buffer.Cell, first, last int) (bool, error) {
	return r.emitRangeSequential(w, newBuf, line, first, last, false)
}

func (r *Renderer) emitRangeSequential(w io.Writer, newBuf *RenderBuffer, line []buffer.Cell, first, last int, sequential bool) (bool, error) {
	hasECH := r.opts.Contains(OptECH)
	hasREP := r.opts.Contains(OptREP)
	if !hasECH && !hasREP {
		for x := first; x <= last; {
			c := line[x]
			if c.IsContinuation() {
				x++
				continue
			}
			if err := r.emitCell(w, &c, r.lastWidth, r.lastHeight); err != nil {
				return false, err
			}
			x += max(c.Width(), 1)
		}
		return false, nil
	}
	for x := first; x <= last; {
		if line[x].IsContinuation() {
			x++
			continue
		}
		stride := max(line[x].Width(), 1)
		next := x + stride
		if next > last {
			c := line[x]
			return false, r.emitCell(w, &c, r.lastWidth, r.lastHeight)
		}
		if !line[x].Equal(line[next]) {
			c := line[x]
			if err := r.emitCell(w, &c, r.lastWidth, r.lastHeight); err != nil {
				return false, err
			}
			x = next
			continue
		}
		cell0 := line[x]
		count := 2
		j := next + stride
		for j <= last && line[j].Equal(cell0) {
			count++
			j += stride
		}
		echB := ECHCost(count)
		cupB := CUPCost(r.cur.pos().Y, r.cur.pos().X+count)
		if sequential {
			cupB = CUFCost(count)
		}
		repB := REPCost(count)
		if hasECH && count > echB+cupB && canClearWith(cell0, r.opts.Contains(OptBCE)) {
			if err := r.updatePen(w, &cell0); err != nil {
				return false, err
			}
			r.emitBuf = ansi.AppendECH(r.emitBuf[:0], count)
			r.writeEmit(w)
			if j > last {
				return true, nil
			}
			if sequential {
				r.emitBuf = ansi.AppendCUF(r.emitBuf[:0], count)
				if err := r.writeEmit(w); err != nil {
					return false, err
				}
				r.cur.setX(r.cur.pos().X + count)
			} else {
				if err := r.moveTo(w, newBuf, r.cur.pos().Y, r.cur.pos().X+count); err != nil {
					return false, err
				}
			}
			x = j
		} else if hasREP && count > repB && cell0.Width() == 1 && isRepASCII(cell0.Content()) {
			wrap := r.cur.pos().X+count >= r.lastWidth
			repCount := count
			if wrap {
				repCount--
			}
			if err := r.updatePen(w, &cell0); err != nil {
				return false, err
			}
			if err := r.putGlyph(w, cell0.Content(), 1, r.lastWidth, r.lastHeight); err != nil {
				return false, err
			}
			repCount--
			if repCount > 0 {
				r.emitBuf = ansi.AppendREP(r.emitBuf[:0], repCount)
				r.writeEmit(w)
				nx := min(r.cur.pos().X+repCount, r.lastWidth)
				r.cur.setX(nx)
				if r.cur.pos().X >= r.lastWidth {
					r.cur.atPhantom = true
				}
			}
			if wrap {
				if err := r.putGlyph(w, cell0.Content(), 1, r.lastWidth, r.lastHeight); err != nil {
					return false, err
				}
			}
			x = j
		} else {
			if err := r.updatePen(w, &cell0); err != nil {
				return false, err
			}
			glyph := cell0.Content()
			gw := cell0.Width()
			if glyph == "" {
				glyph = r.blankGlyph(gw)
			}
			for i := 0; i < count; i++ {
				if err := r.putGlyph(w, glyph, gw, r.lastWidth, r.lastHeight); err != nil {
					return false, err
				}
			}
			x = j
		}
	}
	return false, nil
}

func (r *Renderer) emitCell(w io.Writer, cell *buffer.Cell, sw, sh int) error {
	if cell.IsContinuation() {
		return nil
	}
	if err := r.updatePen(w, cell); err != nil {
		return err
	}
	content := cell.Content()
	cw := cell.Width()
	if content == "" {
		content = r.blankGlyph(cw)
	}
	return r.putGlyph(w, content, cw, sw, sh)
}

func (r *Renderer) blankGlyph(width int) string {
	if len(r.spaces) < width {
		r.spaces = strings.Repeat(" ", width)
	}
	return r.spaces[:width]
}

func (r *Renderer) putRange(w io.Writer, newBuf *RenderBuffer, oldLine, newLine []buffer.Cell, y, start, end int) (bool, error) {
	inline := min(CUPCost(y, start), CUFCost(start))
	if r.opts.Contains(OptHPA) {
		inline = min(inline, HPACost(start))
	}
	if r.opts.Contains(OptCHA) {
		inline = min(inline, CHACost(start))
	}
	span := end - start + 1
	if span > inline {
		j := start
		same := 0
		segStart := start
		for j <= end {
			old, ok := getCell(oldLine, j)
			newc := newLine[j]
			if same == 0 && ok && old.IsContinuation() && newc.IsContinuation() {
				j++
				continue
			}
			equal := ok && old.Equal(newc)
			if equal {
				same++
			} else {
				if same > inline {
					// The jump lands where emission resumes, so it has to name
					// a column that owns its cell. Landing on a continuation
					// would park the cursor inside a glyph that emits nothing,
					// and the next one would go out a column early. The segment
					// just closed ends at the last column of its own cluster
					// for the same reason, so neither end cuts a glyph in half.
					//
					// The two ends are not alike. resume is load bearing: a row
					// can hold a continuation whose owner does not reach it,
					// which Resize leaves behind by truncating a pair and which
					// a later write can then orphan, and a jump landing there
					// parks the cursor inside a glyph that emits nothing, so
					// the column before it is never repainted.
					//
					// Close the preceding segment on a whole cluster without
					// crossing the next segment's owner.
					resume := clusterStart(newLine, j)
					prev := j - same - 1
					if prev >= segStart {
						// resume is zero when every column back to the start of
						// the row continues one cluster, leaving nothing before
						// it to emit.
						stop := min(clusterEnd(newLine, prev), resume-1)
						if stop >= segStart {
							if _, err := r.emitRange(w, newBuf, newLine, segStart, stop); err != nil {
								return false, err
							}
						}
					}
					if err := r.moveTo(w, newBuf, y, resume); err != nil {
						return false, err
					}
					segStart = resume
				}
				same = 0
			}
			j++
		}
		tailEnd := j - same - 1
		tail := false
		var err error
		if tailEnd >= segStart {
			tail, err = r.emitRange(w, newBuf, newLine, segStart, tailEnd)
			if err != nil {
				return false, err
			}
		}
		if same != 0 {
			return true, nil
		}
		return tail, nil
	}
	return r.emitRange(w, newBuf, newLine, start, end)
}

func (r *Renderer) insertCellsOp(w io.Writer, line []buffer.Cell, count int) error {
	if count == 0 {
		return nil
	}
	// count is a column count, and a cell can occupy several, so the window
	// can end between a cell and the column it owns. The cells are written
	// whole either way, so the room has to cover the whole of the last one:
	// under insert mode each glyph opens its own room as it arrives, and a
	// glyph wider than the room left carries the tail one column further than
	// was asked for.
	if count-1 < len(line) {
		count = clusterEnd(line, count-1) + 1
	}
	useICH := r.opts.Contains(OptICH)
	if useICH {
		r.emitBuf = ansi.AppendICH(r.emitBuf[:0], count)
		r.writeEmit(w)
	} else {
		r.emitBuf = ansi.AppendSetMode(r.emitBuf[:0], ansi.InsertMode)
		r.writeEmit(w)
	}
	for i := 0; i < count; i++ {
		if i < len(line) {
			c := line[i]
			if c.IsContinuation() {
				continue
			}
			if err := r.updatePen(w, &c); err != nil {
				return err
			}
			if err := r.emitCell(w, &c, r.lastWidth, r.lastHeight); err != nil {
				return err
			}
		} else {
			if err := r.resetPen(w); err != nil {
				return err
			}
			if err := r.putGlyph(w, " ", 1, r.lastWidth, r.lastHeight); err != nil {
				return err
			}
		}
	}
	if !useICH {
		r.emitBuf = ansi.AppendResetMode(r.emitBuf[:0], ansi.InsertMode)
		return r.writeEmit(w)
	}
	return nil
}
