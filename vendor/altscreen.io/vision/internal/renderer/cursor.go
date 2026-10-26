package renderer

import (
	"io"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
)

const longDist = 7

// verticalKind names the sequence a vertical move resolves to.
type verticalKind uint8

const (
	verticalNone verticalKind = iota
	verticalVPA
	verticalCUD
	verticalCUU
	verticalRI
	verticalLF
)

// horizontalKind names the sequence a horizontal move resolves to.
type horizontalKind uint8

const (
	horizontalNone horizontalKind = iota
	horizontalHPA
	horizontalCHA
	horizontalForward
	horizontalTabs
	horizontalCHT
	horizontalCUB
	horizontalBS
	horizontalCBT
)

// forwardKind names the sequence a forward residual move resolves to.
type forwardKind uint8

const (
	forwardNone forwardKind = iota
	forwardCUF
	forwardOverwrite
)

type verticalShape struct {
	kind verticalKind
	n    int
}
type verticalPlan struct {
	shape        verticalShape
	cost, postFX int
}
type forwardShape struct {
	kind      forwardKind
	fx, tx, n int
}
type horizontalShape struct {
	kind                 horizontalKind
	tx, count, residualN int
	residual             forwardShape
}
type horizontalPlan struct {
	shape horizontalShape
	cost  int
}
type relativePlan struct {
	cost       int
	vertical   verticalPlan
	horizontal horizontalPlan
}
type prefixShape uint8

const (
	prefixNone prefixShape = iota
	prefixCR
	prefixHome
)

type moveWinner struct {
	cup    bool
	prefix prefixShape
	plan   relativePlan
	cost   int
}

func notLocal(width int, from, to layout.Position) bool {
	if width <= 2*longDist+2 {
		return false
	}
	right := width - 1 - longDist
	dx := abs(to.X - from.X)
	dy := abs(to.Y - from.Y)
	return to.X > longDist && to.X < right && dx+dy > longDist
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (p prefixShape) cost() int {
	switch p {
	case prefixCR:
		return CRCost
	case prefixHome:
		return HomeCost
	}
	return 0
}

func (p prefixShape) apply(from layout.Position) layout.Position {
	switch p {
	case prefixCR:
		return layout.Pos(0, from.Y)
	case prefixHome:
		return layout.Pos(0, 0)
	}
	return from
}

// emitPrefix is a method on the renderer rather than on prefixShape so it can
// stage the home sequence in the renderer's buffer.
func (r *Renderer) emitPrefix(w io.Writer, p prefixShape) error {
	switch p {
	case prefixCR:
		r.emitBuf = append(r.emitBuf[:0], '\r')
		return r.writeEmit(w)
	case prefixHome:
		r.emitBuf = ansi.AppendCUP(r.emitBuf[:0], 0, 0)
		return r.writeEmit(w)
	}
	return nil
}

func (r *Renderer) writeOptimalMove(w io.Writer, from, to layout.Position, line []buffer.Cell) error {
	return r.writeOptimalMoveWithPen(w, from, to, line, resetBeforeScroll)
}

func (r *Renderer) writeOptimalMoveWithPen(w io.Writer, from, to layout.Position, line []buffer.Cell, pen penPolicy) error {
	if from == to && r.cur.known() {
		return nil
	}
	if !r.relativeCursor && (!r.cur.known() || r.lastWidth == 0 || notLocal(r.lastWidth, from, to)) {
		r.emitBuf = ansi.AppendCUP(r.emitBuf[:0], to.Y, to.X)
		return r.writeEmit(w)
	}
	best := r.planMove(from, to, line)
	if pen == resetBeforeScroll && r.lfWouldBleedBackground(best) {
		if err := r.resetPen(w); err != nil {
			return err
		}
		best = r.planMove(from, to, line)
	}
	if best.cup {
		r.emitBuf = ansi.AppendCUP(r.emitBuf[:0], to.Y, to.X)
		return r.writeEmit(w)
	}
	if err := r.emitPrefix(w, best.prefix); err != nil {
		return err
	}
	return r.emitRelativePlan(w, best.plan, line)
}

func (r *Renderer) lfWouldBleedBackground(best moveWinner) bool {
	return !best.cup && best.plan.vertical.shape.kind == verticalLF && !r.fullscreen && r.opts.Contains(OptBCE) && r.cur.style.GetBackground() != nil
}

func (r *Renderer) planMove(from, to layout.Position, line []buffer.Cell) moveWinner {
	best := moveWinner{cost: 1 << 30}
	if !r.relativeCursor {
		best = moveWinner{cup: true, cost: CUPCost(to.Y, to.X)}
	}
	mask := 0
	if r.opts.Contains(OptTABS) {
		mask |= 2
	}
	if r.opts.Contains(OptBS) {
		mask |= 1
	}
	for i := 0; i <= mask; i++ {
		if i&^mask != 0 {
			continue
		}
		useTabs := i&2 != 0
		useBS := i&1 != 0
		for _, p := range []prefixShape{prefixNone, prefixCR, prefixHome} {
			if p == prefixHome && r.relativeCursor {
				continue
			}
			plan := r.relativeCursorPlan(p.apply(from), to, line, useTabs, useBS)
			total := p.cost() + plan.cost
			if total < best.cost {
				best = moveWinner{prefix: p, plan: plan, cost: total}
			}
		}
	}
	return best
}

func (r *Renderer) relativeCursorPlan(from, to layout.Position, line []buffer.Cell, useTabs, useBS bool) relativePlan {
	v := r.planVerticalCost(from.Y, to.Y, from.X)
	h := r.planHorizontalCost(v.postFX, to.X, line, useTabs, useBS)
	return relativePlan{v.cost + h.cost, v, h}
}

func (r *Renderer) emitRelativePlan(w io.Writer, p relativePlan, line []buffer.Cell) error {
	if err := r.emitVertical(w, p.vertical.shape); err != nil {
		return err
	}
	return r.emitHorizontal(w, p.horizontal.shape, line)
}

func (r *Renderer) relativeCursorMove(w io.Writer, from, to layout.Position, line []buffer.Cell, useTabs, useBS bool) error {
	return r.emitRelativePlan(w, r.relativeCursorPlan(from, to, line, useTabs, useBS), line)
}

func (r *Renderer) planVerticalCost(fy, ty, fx int) verticalPlan {
	if ty == fy {
		return verticalPlan{verticalShape{verticalNone, 0}, 0, fx}
	}
	bestCost := 1 << 30
	best := verticalShape{}
	if !r.relativeCursor && r.opts.Contains(OptVPA) {
		best = verticalShape{verticalVPA, ty}
		bestCost = VPACost(ty)
	}
	if ty > fy {
		n := ty - fy
		if c := CUDCost(n); c < bestCost {
			bestCost = c
			best = verticalShape{verticalCUD, n}
		}
		if c := LFCost(n); !r.fullscreen || c < bestCost {
			bestCost = c
			best = verticalShape{verticalLF, n}
		}
	} else {
		n := fy - ty
		if c := CUUCost(n); c < bestCost {
			bestCost = c
			best = verticalShape{verticalCUU, n}
		}
		if n == 1 && fy > 1 && RICost < bestCost {
			bestCost = RICost
			best = verticalShape{verticalRI, 1}
		}
	}
	post := fx
	if best.kind == verticalLF && r.opts.Contains(OptONLCR) {
		post = 0
	}
	return verticalPlan{best, bestCost, post}
}

func (r *Renderer) emitVertical(w io.Writer, s verticalShape) error {
	dst := r.emitBuf[:0]
	switch s.kind {
	case verticalVPA:
		dst = ansi.AppendVPA(dst, s.n)
	case verticalCUD:
		dst = ansi.AppendCUD(dst, s.n)
	case verticalCUU:
		dst = ansi.AppendCUU(dst, s.n)
	case verticalRI:
		dst = append(dst, ansi.ReverseIndex...)
	case verticalLF:
		for range s.n {
			dst = append(dst, '\n')
		}
	}
	r.emitBuf = dst
	return r.writeEmit(w)
}

func (r *Renderer) planHorizontalCost(fx, tx int, line []buffer.Cell, useTabs, useBS bool) horizontalPlan {
	if tx == fx {
		return horizontalPlan{horizontalShape{kind: horizontalNone}, 0}
	}
	bestCost := 1 << 30
	best := horizontalShape{}
	if !r.relativeCursor {
		if r.opts.Contains(OptHPA) {
			if c := HPACost(tx); c < bestCost {
				bestCost = c
				best = horizontalShape{kind: horizontalHPA, tx: tx}
			}
		} else if r.opts.Contains(OptCHA) {
			if c := CHACost(tx); c < bestCost {
				bestCost = c
				best = horizontalShape{kind: horizontalCHA, tx: tx}
			}
		}
	}
	if tx > fx {
		tr := r.forwardTabWalk(fx, tx)
		if useTabs && tr.count > 0 {
			res, rc := r.forwardResidualCost(line, tr.post, tx)
			total := TabCost(tr.count) + rc
			if total < bestCost {
				bestCost = total
				best = horizontalShape{kind: horizontalTabs, count: tr.count, residual: res}
			}
		}
		if useTabs && r.opts.Contains(OptCHT) && tr.count > 0 {
			res, rc := r.forwardResidualCost(line, tr.post, tx)
			total := CHTCost(tr.count) + rc
			if CHTCost(tr.count) < TabCost(tr.count) && total < bestCost {
				bestCost = total
				best = horizontalShape{kind: horizontalCHT, count: tr.count, residual: res}
			}
		}
		res, rc := r.forwardResidualCost(line, fx, tx)
		if rc < bestCost {
			bestCost = rc
			best = horizontalShape{kind: horizontalForward, residual: res}
		}
	} else {
		n := fx - tx
		if useTabs && r.opts.Contains(OptCBT) && r.tabs.Width() > 0 {
			col := fx
			cbt := 0
			for r.tabs.Prev(col) >= tx {
				col = r.tabs.Prev(col)
				cbt++
				if r.tabs.Prev(col) == col || col == 0 {
					break
				}
			}
			if cbt > 0 {
				rn := col - tx
				rc := 0
				if rn > 0 {
					rc = CUBCost(rn)
				}
				total := CBTCost(cbt) + rc
				if total < bestCost {
					bestCost = total
					best = horizontalShape{kind: horizontalCBT, count: cbt, residualN: rn}
				}
			}
		}
		if n > 0 {
			if c := CUBCost(n); c < bestCost {
				bestCost = c
				best = horizontalShape{kind: horizontalCUB, residualN: n}
			}
			if useBS {
				if c := BSCost(n); c < bestCost {
					bestCost = c
					best = horizontalShape{kind: horizontalBS, residualN: n}
				}
			}
		}
	}
	return horizontalPlan{best, bestCost}
}

func (r *Renderer) emitHorizontal(w io.Writer, s horizontalShape, line []buffer.Cell) error {
	switch s.kind {
	case horizontalNone:
		return nil
	case horizontalHPA:
		r.emitBuf = ansi.AppendHPA(r.emitBuf[:0], s.tx)
		return r.writeEmit(w)
	case horizontalCHA:
		r.emitBuf = ansi.AppendCHA(r.emitBuf[:0], s.tx)
		return r.writeEmit(w)
	case horizontalForward:
		return r.emitForward(w, s.residual, line)
	case horizontalTabs:
		for i := 0; i < s.count; i++ {
			if _, e := io.WriteString(w, "\t"); e != nil {
				return e
			}
		}
		return r.emitForward(w, s.residual, line)
	case horizontalCHT:
		r.emitBuf = ansi.AppendCHT(r.emitBuf[:0], s.count)
		if e := r.writeEmit(w); e != nil {
			return e
		}
		return r.emitForward(w, s.residual, line)
	case horizontalCUB:
		r.emitBuf = ansi.AppendCUB(r.emitBuf[:0], s.residualN)
		return r.writeEmit(w)
	case horizontalBS:
		for i := 0; i < s.residualN; i++ {
			if _, e := io.WriteString(w, "\b"); e != nil {
				return e
			}
		}
		return nil
	case horizontalCBT:
		r.emitBuf = ansi.AppendCBT(r.emitBuf[:0], s.count)
		if e := r.writeEmit(w); e != nil {
			return e
		}
		if s.residualN > 0 {
			r.emitBuf = ansi.AppendCUB(r.emitBuf[:0], s.residualN)
			return r.writeEmit(w)
		}
	}
	return nil
}

type tabRun struct{ count, post int }

func (r *Renderer) forwardTabWalk(fx, tx int) tabRun {
	if r.tabs.Width() == 0 {
		return tabRun{0, fx}
	}
	count := 0
	col := fx
	for {
		next := r.tabs.NextStop(col)
		if next > tx {
			break
		}
		col = next
		count++
	}
	return tabRun{count, col}
}

func (r *Renderer) forwardResidualCost(line []buffer.Cell, fx, tx int) (forwardShape, int) {
	if tx == fx {
		return forwardShape{kind: forwardNone}, 0
	}
	n := tx - fx
	best := forwardShape{kind: forwardCUF, n: n}
	cost := CUFCost(n)
	if n < cost && line != nil {
		if ow, ok := OverwriteCost(line, r.cur.style, fx, tx); ok && ow < cost {
			best = forwardShape{kind: forwardOverwrite, fx: fx, tx: tx}
			cost = ow
		}
	}
	return best, cost
}

func (r *Renderer) emitForward(w io.Writer, k forwardShape, line []buffer.Cell) error {
	switch k.kind {
	case forwardNone:
		return nil
	case forwardCUF:
		r.emitBuf = ansi.AppendCUF(r.emitBuf[:0], k.n)
		return r.writeEmit(w)
	case forwardOverwrite:
		if !collectOverwriteBytes(w, line, r.cur.style, k.fx, k.tx) {
			r.emitBuf = ansi.AppendCUF(r.emitBuf[:0], k.tx-k.fx)
			return r.writeEmit(w)
		}
		return nil
	}
	return nil
}

// collectOverwriteBytes writes the content an overwrite move over line[from:to]
// would draw, and reports whether the move is usable at all.
//
// Eligibility comes from [overwriteEligible], the same decision the planner
// prices the move with, so the planner can never choose a move this refuses to
// emit.
func collectOverwriteBytes(w io.Writer, line []buffer.Cell, st style.Style, from, to int) bool {
	if _, ok := overwriteEligible(line, st, from, to); !ok {
		return false
	}
	for i := from; i < to; i += max(line[i].Width(), 1) {
		io.WriteString(w, line[i].Content())
	}
	return true
}
