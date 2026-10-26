package renderer

import (
	"image/color"
	"io"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
)

type penPolicy int

const (
	resetBeforeScroll penPolicy = iota
	keepPen
)

func (c *cursor) currentBCEBlank(bce bool) *buffer.Cell {
	var wantBg color.Color
	if bce {
		wantBg = c.style.GetBackground()
	}
	key := bceKey{valid: true, bce: bce, bg: newBgKey(wantBg)}
	if c.bceKey != key {
		c.bceBlank = buffer.BlankCell().WithStyle(style.Empty.Background(wantBg))
		c.bceKey = key
	}
	return &c.bceBlank
}

func (r *Renderer) Render(w io.Writer, newBuf *RenderBuffer) error {
	width, height := newBuf.Width(), newBuf.Height()
	if !r.forceClear && !newBuf.HasChanges() {
		return nil
	}
	resized, err := r.prepareFrame(w, newBuf, width, height)
	if err != nil {
		return err
	}
	if err := r.diffFrame(w, newBuf, width, height); err != nil {
		return err
	}
	return r.finalizeFrame(w, newBuf, resized, height)
}

func (r *Renderer) RenderBack(w io.Writer) error {
	back := r.backBuf
	r.backBuf = NewRenderBuffer(0, 0)
	err := r.Render(w, back)
	back.ClearTouched()
	r.backBuf = back
	return err
}

func (r *Renderer) prepareFrame(w io.Writer, newBuf *RenderBuffer, width, height int) (bool, error) {
	if r.curBuf == nil {
		r.curBuf = NewRenderBuffer(width, height)
	}
	prevW, prevH := r.lastWidth, r.lastHeight
	resized := width != prevW || height != prevH
	inlinePartial := resized && !r.fullscreen && r.curBuf != nil && width == prevW && height < prevH
	if resized {
		r.curBuf.Resize(width, height)
		r.oldHashes = nil
		r.newHashes = nil
	}
	if r.tabs.Width() != width {
		r.tabs.Resize(width)
	}
	r.lastWidth, r.lastHeight = width, height
	if inlinePartial && height > 0 && !r.forceClear {
		if err := r.resetPen(w); err != nil {
			return false, err
		}
		if err := r.clearBelow(w, newBuf, height-1); err != nil {
			return false, err
		}
	}
	if r.forceClear {
		if err := r.clearUpdate(w, newBuf); err != nil {
			return false, err
		}
		r.forceClear = false
	} else if r.scrollOptimize && r.fullscreen && r.syncOutput {
		// The scrolls emitted here are always full width, so a detected scroll
		// moves every column of the rows it covers, including a fixed sidebar,
		// file tree, gutter or scrollbar that should have stayed put. This
		// frame paints that region back, so the settled screen is correct and a
		// test comparing the finished grid sees nothing wrong. What the user
		// sees is the region jumping and snapping back, on every frame, for as
		// long as they keep scrolling.
		//
		// That correction is only hidden when the whole frame is presented in
		// one step, so detection runs only while synchronized output is on and
		// rows are redrawn directly otherwise.
		touched := newBuf.TouchedLineCount()
		threshold := max(2, height/8)
		if touched > threshold {
			r.computeHashes(newBuf)
			r.updateHashmap(newBuf, height)
			if err := r.applyScrolls(w, newBuf); err != nil {
				return false, err
			}
		}
	}
	return resized, nil
}

func (r *Renderer) clearUpdate(w io.Writer, newBuf *RenderBuffer) error {
	if !r.fullscreen {
		if err := r.clearBelow(w, newBuf, 0); err != nil {
			return err
		}
	} else {
		if err := r.clearScreen(w); err != nil {
			return err
		}
	}
	newBuf.TouchAll()
	return nil
}

func (r *Renderer) computeHashes(newBuf *RenderBuffer) {
	height := newBuf.Height()
	if r.curBuf == nil {
		return
	}
	cached := len(r.oldHashes) == height && len(r.newHashes) == height
	if !cached {
		r.oldHashes = make([]uint64, height)
		r.newHashes = make([]uint64, height)
	}
	for y := 0; y < height; y++ {
		if cached {
			if _, ok := newBuf.Touched(y); !ok {
				continue
			}
		}
		if line := r.curBuf.Line(y); line != nil {
			r.oldHashes[y] = r.hashLine(line)
		}
		if line := newBuf.Line(y); line != nil {
			r.newHashes[y] = r.hashLine(line)
		}
	}
}

func (r *Renderer) diffFrame(w io.Writer, newBuf *RenderBuffer, width, height int) error {
	nonEmpty, err := r.clearBottom(w, newBuf)
	if err != nil {
		return err
	}
	last := width - 1
	for y := 0; y < min(nonEmpty, height); y++ {
		if _, ok := newBuf.Touched(y); ok {
			if err := r.transformLine(w, newBuf, y, 0, last); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Renderer) finalizeFrame(w io.Writer, newBuf *RenderBuffer, resized bool, height int) error {
	if resized && !r.fullscreen && height > 0 {
		if err := r.moveTo(w, newBuf, height-1, 0); err != nil {
			return err
		}
	}
	return r.updatePen(w, nil)
}

// MoveCursorTo moves the cursor to row y, column x for a caller outside the
// diff loop, so that a move staged between frames costs the same sequences the
// renderer would have chosen for one inside a frame.
//
// size is the caller's current surface, which the wrap and the clamp are
// measured against. The renderer's own size is whatever it last rendered, and
// between a resize and the frame that answers it the caller's is newer; using
// the stale one retargets the move to a position the caller did not ask for.
//
// Ordinary moves use control sequences, never unrendered cells. A target past
// an uncertain cluster instead walks the last displayed row. A pending clear
// invalidates that snapshot and disables the walk until the next render.
func (r *Renderer) MoveCursorTo(w io.Writer, size layout.Size, y, x int) error {
	return r.moveToSize(w, nil, size.Width, size.Height, y, x, resetBeforeScroll)
}

// moveTo moves the cursor to row y, column x, using buf's row y for overwrite
// consideration. It runs the pre-move bookkeeping (autowrap, phantom-cell
// snap, height clamp, no-op shortcut) and then defers to the optimal-move
// planner.
//
// buf is the frame being diffed, and a move made during a diff always targets
// a column the transform has proven equal between the current and the new
// line, so the cells the planner may reprint are what the terminal already
// shows. A caller without that invariant wants [Renderer.MoveCursorTo].
//
// It may reset the pen: an inline downward move is emitted as a literal
// newline that can scroll the host, and under back-color erase the exposed row
// would otherwise keep the active background.
func (r *Renderer) moveTo(w io.Writer, buf *RenderBuffer, y, x int) error {
	return r.moveToWithPen(w, buf, y, x, resetBeforeScroll)
}

func (r *Renderer) moveToKeepingPen(w io.Writer, buf *RenderBuffer, y, x int) error {
	return r.moveToWithPen(w, buf, y, x, keepPen)
}

// moveToWithPen measures the move against the last rendered size, which is what
// a caller inside a frame wants: the frame being diffed is that size.
func (r *Renderer) moveToWithPen(w io.Writer, buf *RenderBuffer, y, x int, pen penPolicy) error {
	return r.moveToSize(w, buf, r.lastWidth, r.lastHeight, y, x, pen)
}

func (r *Renderer) moveToSize(w io.Writer, buf *RenderBuffer, width, height, y, x int, pen penPolicy) error {
	if width > 0 && x >= width {
		y += x / width
		x %= width
	}
	if r.cur.atPhantom {
		if _, err := io.WriteString(w, "\r"); err != nil {
			return err
		}
		r.cur.setX(0)
		r.cur.atPhantom = false
	}
	if height > 0 && y > height-1 {
		y = height - 1
	}
	if buf == nil && !r.forceClear && r.curBuf != nil {
		walked, err := r.walkToUncertain(w, r.curBuf, y, x)
		if err != nil || walked {
			return err
		}
	}
	if r.cur.known() && r.cur.x == x && r.cur.y == y {
		return nil
	}
	return r.moveCursorWithPen(w, buf, y, x, pen)
}

func (r *Renderer) moveCursorWithPen(w io.Writer, buf *RenderBuffer, y, x int, pen penPolicy) error {
	if !r.fullscreen && r.relativeCursor && !r.cur.hasX {
		if err := r.reanchorToRowStart(w); err != nil {
			return err
		}
		if !r.cur.hasY {
			r.cur.setY(0)
		}
	}
	target := layout.Pos(x, y)
	var line []buffer.Cell
	if buf != nil {
		line = buf.Line(y)
	}
	if err := r.writeOptimalMoveWithPen(w, r.cur.pos(), target, line, pen); err != nil {
		return err
	}
	r.cur.setPos(target)
	return nil
}

func (r *Renderer) putGlyph(w io.Writer, content string, cellWidth, surfaceWidth, surfaceHeight int) error {
	if cellWidth == 0 {
		return nil
	}
	if r.cur.atPhantom {
		nextY := r.cur.pos().Y + 1
		if nextY < surfaceHeight {
			target := layout.Pos(0, nextY)
			if err := r.writeOptimalMove(w, r.cur.pos(), target, nil); err != nil {
				return err
			}
			r.cur.setPos(target)
		}
		r.cur.atPhantom = false
	}
	lowerRight := r.fullscreen && cellWidth == 1 && r.cur.pos().X+1 == surfaceWidth && r.cur.pos().Y+1 == surfaceHeight
	if lowerRight {
		r.emitBuf = ansi.AppendResetMode(r.emitBuf[:0], ansi.AutoWrapMode)
		if err := r.writeEmit(w); err != nil {
			return err
		}
		if _, err := io.WriteString(w, content); err != nil {
			return err
		}
		r.emitBuf = ansi.AppendSetMode(r.emitBuf[:0], ansi.AutoWrapMode)
		if err := r.writeEmit(w); err != nil {
			return err
		}
		x := max(surfaceWidth-1, 0)
		r.cur.setX(x)
		r.cur.atPhantom = false
		return nil
	}
	if _, err := io.WriteString(w, content); err != nil {
		return err
	}
	nx := r.cur.pos().X + cellWidth
	if nx >= surfaceWidth {
		nx = surfaceWidth
		r.cur.atPhantom = true
	} else {
		r.cur.atPhantom = false
	}
	r.cur.setX(nx)
	return nil
}
