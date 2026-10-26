package renderer

import (
	"io"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
)

func (r *Renderer) uncertainFrom(line []buffer.Cell) (int, bool) {
	if r.widthMethod != ansi.WcWidth {
		return 0, false
	}
	for x := range line {
		if r.widthIsUncertain(&line[x]) {
			return x, true
		}
	}
	return 0, false
}

func (r *Renderer) uncertainBail(newLine, oldLine []buffer.Cell, firstDiff int) (int, bool) {
	at, ok := r.uncertainFrom(newLine)
	oldEnd := len(newLine)
	if ok {
		oldEnd = at
	}
	if end := min(len(oldLine), oldEnd); firstDiff < end {
		if oldAt, found := r.uncertainFrom(oldLine[firstDiff:end]); found {
			at, ok = firstDiff+oldAt, true
		}
	}
	if !ok {
		return 0, false
	}
	if at > firstDiff {
		different := false
		for x := at; x < len(newLine); x++ {
			if !cellEqualPtr(oldLine, x, newLine[x]) {
				different = true
				break
			}
		}
		if !different {
			return 0, false
		}
	}
	return clusterStart(newLine, at), true
}

// MoveToResting places the cursor after the displayed cells before its target.
// Past an uncertain cluster, it writes those cells instead of naming a column.
func (r *Renderer) MoveToResting(w io.Writer, buf *RenderBuffer, y, x int) error {
	walked, err := r.walkToUncertain(w, buf, y, x)
	if err != nil || walked {
		return err
	}
	return r.moveTo(w, buf, y, x)
}

func (r *Renderer) walkToUncertain(w io.Writer, buf *RenderBuffer, y, x int) (bool, error) {
	line := buf.Line(y)
	if x <= 0 {
		return false, nil
	}
	from, ok := r.uncertainFrom(line[:min(len(line), x)])
	if !ok {
		return false, nil
	}
	if err := r.moveTo(w, buf, y, from); err != nil {
		return false, err
	}
	stop := clusterStart(line, x)
	if stop > from {
		r.emitBuf = ansi.AppendResetMode(r.emitBuf[:0], ansi.AutoWrapMode)
		if err := r.writeEmit(w); err != nil {
			return false, err
		}
		eoi, err := r.emitRangeSequential(w, buf, line, from, stop-1, true)
		if err != nil {
			return false, err
		}
		if eoi {
			r.emitBuf = ansi.AppendCUF(r.emitBuf[:0], stop-r.cur.pos().X)
			if err := r.writeEmit(w); err != nil {
				return false, err
			}
		}
		r.emitBuf = ansi.AppendSetMode(r.emitBuf[:0], ansi.AutoWrapMode)
		if err := r.writeEmit(w); err != nil {
			return false, err
		}
		if err := r.resetPen(w); err != nil {
			return false, err
		}
	}
	r.cur.hasX = false
	return true, nil
}

func (r *Renderer) repaintTail(w io.Writer, buf *RenderBuffer, line []buffer.Cell, y, from int) error {
	if err := r.moveTo(w, buf, y, from); err != nil {
		return err
	}
	last := len(line) - 1
	blank := line[last]
	if canClearWith(blank, r.opts.Contains(OptBCE)) {
		if err := r.updatePen(w, &blank); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ansi.EraseToEOL()); err != nil {
			return err
		}
		for last > from && line[last].Equal(blank) {
			last--
		}
		if line[last].Equal(blank) {
			return r.reanchorToRowStart(w)
		}
	}
	// Clip terminal width disagreements to this row instead of wrapping.
	r.emitBuf = ansi.AppendResetMode(r.emitBuf[:0], ansi.AutoWrapMode)
	if err := r.writeEmit(w); err != nil {
		return err
	}
	if _, err := r.emitRangeSequential(w, buf, line, from, last, true); err != nil {
		return err
	}
	r.emitBuf = ansi.AppendSetMode(r.emitBuf[:0], ansi.AutoWrapMode)
	if err := r.writeEmit(w); err != nil {
		return err
	}
	return r.reanchorToRowStart(w)
}

func (r *Renderer) reanchorToRowStart(w io.Writer) error {
	if _, err := io.WriteString(w, "\r"); err != nil {
		return err
	}
	r.cur.setX(0)
	r.cur.atPhantom = false
	return nil
}
