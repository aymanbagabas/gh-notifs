package renderer

import (
	"io"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
)

const noMap = -1

type scrollHashEntry struct {
	hash               uint64
	oldCount, newCount int
	oldIdx, newIdx     int
}

func probe(table []scrollHashEntry, h uint64) int {
	i := 0
	for i < len(table) && table[i].hash != 0 {
		if table[i].hash == h {
			return i
		}
		i++
	}
	return i
}

func (r *Renderer) updateHashmap(newBuf *RenderBuffer, height int) {
	r.oldnum = r.oldnum[:0]
	for len(r.oldnum) < height {
		r.oldnum = append(r.oldnum, noMap)
	}
	if len(r.oldHashes) != height || len(r.newHashes) != height {
		return
	}
	size := (height + 1) * 2
	if cap(r.hashtab) < size {
		r.hashtab = make([]scrollHashEntry, size)
	}
	table := r.hashtab[:size]
	clear(table)
	for i, h := range r.oldHashes {
		if h == 0 {
			continue
		}
		idx := probe(table, h)
		if idx >= len(table) {
			continue
		}
		e := &table[idx]
		e.hash = h
		e.oldCount++
		e.oldIdx = i
	}
	for i, h := range r.newHashes {
		if h == 0 {
			continue
		}
		idx := probe(table, h)
		if idx >= len(table) {
			continue
		}
		e := &table[idx]
		e.hash = h
		e.newCount++
		e.newIdx = i
	}
	for _, e := range table {
		if e.hash == 0 {
			break
		}
		if e.oldCount == 1 && e.newCount == 1 && e.oldIdx != e.newIdx && e.newIdx < height {
			r.oldnum[e.newIdx] = e.oldIdx
		}
	}
	r.growHunks(newBuf, height)
	invalidateBadHunks(r.oldnum, height)
	r.growHunks(newBuf, height)
}

func (r *Renderer) growHunks(newBuf *RenderBuffer, height int) {
	h := height
	backLimit, backRef := 0, 0
	i := 0
	for i < h && r.oldnum[i] == noMap {
		i++
	}
	for i < h {
		start := i
		shift := r.oldnum[start] - start
		i = start + 1
		for i < h && r.oldnum[i] != noMap && r.oldnum[i]-i == shift {
			i++
		}
		end := i
		for i < h && r.oldnum[i] == noMap {
			i++
		}
		nextHunk := i
		fLimit := i
		fRef := i
		if i < h && r.oldnum[i] < i {
			fRef = r.oldnum[i]
		}
		j := start - 1
		bl := backLimit
		if shift < 0 {
			bl = backRef + (-shift)
		}
		for j >= bl {
			target := j + shift
			if target < 0 || target >= h {
				break
			}
			ok := r.newHashes[j] == r.oldHashes[target] || r.costEffective(newBuf, target, j, shift < 0)
			if !ok {
				break
			}
			r.oldnum[j] = target
			j--
		}
		j = end
		fl := fLimit
		if shift > 0 {
			fl = fRef - shift
		}
		for j < fl {
			target := j + shift
			if target < 0 || target >= h {
				break
			}
			ok := r.newHashes[j] == r.oldHashes[target] || r.costEffective(newBuf, target, j, shift > 0)
			if !ok {
				break
			}
			r.oldnum[j] = target
			j++
		}
		backLimit = j
		backRef = backLimit
		if shift > 0 {
			backRef += shift
		}
		i = nextHunk
	}
}

func invalidateBadHunks(oldnum []int, height int) {
	i := 0
	for i < height {
		if oldnum[i] == noMap {
			i++
			continue
		}
		start := i
		shift := oldnum[start] - start
		i++
		for i < height && oldnum[i] != noMap && oldnum[i]-i == shift {
			i++
		}
		size := i - start
		cushion := min(size/8, 2)
		if size < 3 || size+cushion < abs(shift) {
			for j := start; j < i; j++ {
				oldnum[j] = noMap
			}
		}
	}
}

func (r *Renderer) costEffective(newBuf *RenderBuffer, from, to int, blank bool) bool {
	if from == to || r.curBuf == nil {
		return false
	}
	width := newBuf.Width()
	clearBlank := *r.cur.currentBlank()
	old := r.curBuf
	newFrom := from
	if from < len(r.oldnum) && r.oldnum[from] != noMap {
		newFrom = r.oldnum[from]
	}
	costBefore := 0
	if blank {
		costBefore = updateCostBlank(clearBlank, newBuf.Line(to), width)
	} else {
		costBefore = updateCost(old.Line(to), newBuf.Line(to), width)
	}
	costBefore += updateCost(old.Line(newFrom), newBuf.Line(from), width)
	costAfter := 0
	if newFrom == from {
		costAfter = updateCostBlank(clearBlank, newBuf.Line(from), width)
	} else {
		costAfter = updateCost(old.Line(newFrom), newBuf.Line(from), width)
	}
	costAfter += updateCost(old.Line(from), newBuf.Line(to), width)
	return costBefore >= costAfter
}

func updateCost(oldLine, newLine []buffer.Cell, width int) int {
	common := min(min(len(oldLine), len(newLine)), width)
	cost := 0
	for i := 0; i < common; i++ {
		if !oldLine[i].Equal(newLine[i]) {
			cost++
		}
	}
	maxp := min(max(len(oldLine), len(newLine)), width)
	return cost + maxp - common
}

func updateCostBlank(blank buffer.Cell, line []buffer.Cell, width int) int {
	common := min(len(line), width)
	cost := 0
	for i := 0; i < common; i++ {
		if !line[i].Equal(blank) {
			cost++
		}
	}
	return cost + width - common
}

func (r *Renderer) applyScrolls(w io.Writer, newBuf *RenderBuffer) error {
	height := r.lastHeight
	if height == 0 || len(r.oldnum) < height {
		return nil
	}
	oldnum := append([]int(nil), r.oldnum...)
	h := height
	maxY := height - 1
	i := 0
	for {
		for i < h && (oldnum[i] < 0 || oldnum[i] <= i) {
			i++
		}
		if i >= h {
			break
		}
		shift := oldnum[i] - i
		start := i
		i++
		for i < h && oldnum[i] >= 0 && oldnum[i]-i == shift {
			i++
		}
		bot := min(i-1+shift, maxY)
		if err := r.scrollN(w, newBuf, shift, start, bot, maxY); err != nil {
			return err
		}
	}
	for i = h - 1; ; {
		for i >= 0 && (oldnum[i] < 0 || oldnum[i] >= i) {
			i--
		}
		if i < 0 {
			break
		}
		shift := oldnum[i] - i
		end := i
		i--
		for i >= 0 && oldnum[i] >= 0 && oldnum[i]-i == shift {
			i--
		}
		start := max(i+1-(-shift), 0)
		if err := r.scrollN(w, newBuf, shift, start, end, maxY); err != nil {
			return err
		}
		if i < 0 {
			break
		}
	}
	return nil
}

func (r *Renderer) scrollN(w io.Writer, newBuf *RenderBuffer, n, top, bot, maxY int) error {
	if n == 0 || top > bot || bot > maxY {
		return nil
	}
	v := false
	var err error
	if n > 0 {
		v, err = r.scrollUp(w, newBuf, n, top, bot, 0, maxY)
		if err != nil {
			return err
		}
		if !v && r.opts.Contains(OptCSR) {
			r.emitBuf = ansi.AppendScrollRegion(r.emitBuf[:0], top, bot)
			r.writeEmit(w)
			r.InvalidateCursor()
			v, err = r.scrollUp(w, newBuf, n, top, bot, top, bot)
			if err != nil {
				return err
			}
			io.WriteString(w, ansi.ResetScrollRegion())
			r.InvalidateCursor()
		}
		if !v && r.opts.Contains(OptILDL) {
			v, err = r.scrollIDL(w, newBuf, n, top, bot+1-n)
			if err != nil {
				return err
			}
		}
	} else {
		amt := -n
		v, err = r.scrollDown(w, newBuf, amt, top, bot, 0, maxY)
		if err != nil {
			return err
		}
		if !v && r.opts.Contains(OptCSR) {
			r.emitBuf = ansi.AppendScrollRegion(r.emitBuf[:0], top, bot)
			r.writeEmit(w)
			r.InvalidateCursor()
			v, err = r.scrollDown(w, newBuf, amt, top, bot, top, bot)
			if err != nil {
				return err
			}
			io.WriteString(w, ansi.ResetScrollRegion())
			r.InvalidateCursor()
		}
		if !v && r.opts.Contains(OptILDL) {
			v, err = r.scrollIDL(w, newBuf, amt, bot+1-amt, top)
			if err != nil {
				return err
			}
		}
	}
	if !v {
		return nil
	}
	bottom := bot + 1
	fill := *r.cur.currentBCEBlank(r.opts.Contains(OptBCE))
	if r.curBuf != nil {
		if n > 0 {
			r.curBuf.DeleteLines(top, n, bottom, &fill)
		} else {
			r.curBuf.InsertLines(top, -n, bottom, &fill)
		}
	}
	for y := top; y <= bot; y++ {
		newBuf.TouchFullLine(y)
	}
	if n > 0 {
		shiftOldHashesUp(r.oldHashes, top, bottom, n)
		r.recomputeBlankHashes(bottom-n, bottom)
	} else {
		amt := -n
		shiftOldHashesDown(r.oldHashes, top, bottom, amt)
		r.recomputeBlankHashes(top, top+amt)
	}
	return nil
}

func (r *Renderer) scrollUp(w io.Writer, newBuf *RenderBuffer, n, top, bot, minY, maxY int) (bool, error) {
	if n == 1 && top == minY && bot == maxY {
		if err := r.moveToKeepingPen(w, newBuf, bot, 0); err != nil {
			return false, err
		}
		_, err := io.WriteString(w, "\n")
		return true, err
	} else if n == 1 && bot == maxY && r.opts.Contains(OptILDL) {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		r.emitBuf = ansi.AppendDeleteLines(r.emitBuf[:0], 1)
		err := r.writeEmit(w)
		return true, err
	} else if top == minY && bot == maxY {
		if err := r.moveToKeepingPen(w, newBuf, bot, 0); err != nil {
			return false, err
		}
		if r.opts.Contains(OptSUSD) {
			r.emitBuf = ansi.AppendScrollUp(r.emitBuf[:0], n)
			err := r.writeEmit(w)
			return true, err
		}
		for i := 0; i < n; i++ {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return false, err
			}
		}
		return true, nil
	} else if bot == maxY && r.opts.Contains(OptILDL) {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		r.emitBuf = ansi.AppendDeleteLines(r.emitBuf[:0], n)
		err := r.writeEmit(w)
		return true, err
	}
	return false, nil
}

func (r *Renderer) scrollDown(w io.Writer, newBuf *RenderBuffer, n, top, bot, minY, maxY int) (bool, error) {
	if n == 1 && top == minY && bot == maxY {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		_, err := io.WriteString(w, ansi.ReverseIndex)
		return true, err
	} else if n == 1 && bot == maxY && r.opts.Contains(OptILDL) {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		r.emitBuf = ansi.AppendInsertLines(r.emitBuf[:0], 1)
		err := r.writeEmit(w)
		return true, err
	} else if top == minY && bot == maxY {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		if r.opts.Contains(OptSUSD) {
			r.emitBuf = ansi.AppendScrollDown(r.emitBuf[:0], n)
			err := r.writeEmit(w)
			return true, err
		}
		for i := 0; i < n; i++ {
			if _, err := io.WriteString(w, ansi.ReverseIndex); err != nil {
				return false, err
			}
		}
		return true, nil
	} else if bot == maxY && r.opts.Contains(OptILDL) {
		if err := r.moveToKeepingPen(w, newBuf, top, 0); err != nil {
			return false, err
		}
		r.emitBuf = ansi.AppendInsertLines(r.emitBuf[:0], n)
		err := r.writeEmit(w)
		return true, err
	}
	return false, nil
}

func (r *Renderer) scrollIDL(w io.Writer, newBuf *RenderBuffer, n, del, ins int) (bool, error) {
	if err := r.moveToKeepingPen(w, newBuf, del, 0); err != nil {
		return false, err
	}
	r.emitBuf = ansi.AppendDeleteLines(r.emitBuf[:0], n)
	if err := r.writeEmit(w); err != nil {
		return false, err
	}
	if err := r.moveToKeepingPen(w, newBuf, ins, 0); err != nil {
		return false, err
	}
	r.emitBuf = ansi.AppendInsertLines(r.emitBuf[:0], n)
	err := r.writeEmit(w)
	return true, err
}

func shiftOldHashesUp(h []uint64, top, bottom, n int) {
	if n == 0 || bottom <= top || bottom > len(h) {
		return
	}
	l := bottom - top
	s := min(n, l)
	copy(h[top:bottom-s], h[top+s:bottom])
	for i := bottom - s; i < bottom; i++ {
		h[i] = 0
	}
}

func shiftOldHashesDown(h []uint64, top, bottom, n int) {
	if n == 0 || bottom <= top || bottom > len(h) {
		return
	}
	l := bottom - top
	s := min(n, l)
	copy(h[top+s:bottom], h[top:bottom-s])
	for i := top; i < top+s; i++ {
		h[i] = 0
	}
}

func (r *Renderer) recomputeBlankHashes(start, end int) {
	if r.curBuf == nil {
		return
	}
	limit := min(end, len(r.oldHashes))
	if start >= limit {
		return
	}
	line := r.curBuf.Line(start)
	if line == nil {
		return
	}
	h := r.hashLine(line)
	for i := start; i < limit; i++ {
		r.oldHashes[i] = h
	}
}
