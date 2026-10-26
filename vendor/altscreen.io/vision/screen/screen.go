// Package screen provides a cell-diff renderer over an io.Writer.
package screen

import (
	"bytes"
	"io"
	"strings"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/color"
	"altscreen.io/vision/internal/renderer"
	"altscreen.io/vision/internal/trace"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
	"altscreen.io/vision/term"
	"altscreen.io/vision/text"
)

// Optimizations is the renderer capability bitset.
type Optimizations = renderer.Optimizations

const (
	// OptECH enables erase-character optimization.
	OptECH = renderer.OptECH
	// OptREP enables repeat-character optimization.
	OptREP = renderer.OptREP
	// OptICH enables insert-character optimization.
	OptICH = renderer.OptICH
	// OptDCH enables delete-character optimization.
	OptDCH = renderer.OptDCH
	// OptCSR enables scroll-region optimization.
	OptCSR = renderer.OptCSR
	// OptSUSD enables scroll-up and scroll-down optimization.
	OptSUSD = renderer.OptSUSD
	// OptILDL enables insert-line and delete-line optimization.
	OptILDL = renderer.OptILDL
	// OptBCE enables back-color-erase optimization.
	OptBCE = renderer.OptBCE
	// OptCHA enables cursor-horizontal-absolute optimization.
	OptCHA = renderer.OptCHA
	// OptHPA enables horizontal-position-absolute optimization.
	OptHPA = renderer.OptHPA
	// OptVPA enables vertical-position-absolute optimization.
	OptVPA = renderer.OptVPA
	// OptTABS enables tab-based horizontal movement.
	OptTABS = renderer.OptTABS
	// OptCBT enables cursor-backward-tab movement.
	OptCBT = renderer.OptCBT
	// OptCHT enables cursor-forward-tab movement.
	OptCHT = renderer.OptCHT
	// OptBS enables backspace-based leftward movement.
	OptBS = renderer.OptBS
	// OptONLCR records terminal newline translation.
	OptONLCR = renderer.OptONLCR
)

// DefaultOptimizations returns the default renderer optimization set.
func DefaultOptimizations() Optimizations { return renderer.DefaultOptimizations() }

// ModernOptimizations returns the optimization set for modern terminals.
func ModernOptimizations() Optimizations { return renderer.ModernOptimizations() }

// NoOptimizations returns an empty optimization set.
func NoOptimizations() Optimizations { return renderer.NoOptimizations() }

// OptimizationsFromEnv detects optimizations from an environment.
func OptimizationsFromEnv(env term.Environ) Optimizations {
	return renderer.OptimizationsFromEnv(env)
}

// Screen is a cell-diff renderer over a writer.
type Screen struct {
	writer           io.Writer
	front            *renderer.RenderBuffer
	renderer         *renderer.Renderer
	out              bytes.Buffer
	width, height    int
	eawWide          bool
	fullscreen       bool
	cursorVisible    bool
	syncOutput       bool
	graphemeClusters bool
	desiredCursor    *layout.Position
	cursorRestedAt   layout.Position
	cursorRested     bool
}

// New builds a screen rendering into w, with a managed area of size.
func New(w io.Writer, size layout.Size) *Screen {
	r := renderer.NewRenderer()
	r.SetFullscreen(false)
	r.SetRelativeCursor(true)
	s := &Screen{writer: w, front: renderer.NewRenderBuffer(0, 0), renderer: r, cursorVisible: true}
	// A frame's escape bytes are staged here before one write reaches the
	// terminal, so start with room for a typical frame.
	s.out.Grow(4096)
	if size.Width != 0 || size.Height != 0 {
		s.Resize(size)
	}
	return s
}

// Writer returns the writer frames are rendered into.
func (s *Screen) Writer() io.Writer { return s.writer }

// Write appends bytes to the staging buffer.
func (s *Screen) Write(p []byte) (int, error) { return s.out.Write(p) }

// Flush drains staged bytes to the writer and flushes the writer when supported.
func (s *Screen) Flush() error {
	if s.out.Len() > 0 {
		trace.Output(s.out.Bytes())
		// WriteTo drains what it managed to write even when the write
		// fails, so a retry does not resend bytes already on the wire.
		if _, err := s.out.WriteTo(s.writer); err != nil {
			return err
		}
	}
	if f, ok := s.writer.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// SetCell writes cell at (x, y) in the desired frame.
func (s *Screen) SetCell(x, y int, cell *buffer.Cell) { s.front.SetCell(x, y, cell) }

// Cell returns the cell at (x, y) and marks its columns touched, so edits made
// through the returned pointer are rendered.
//
// [Screen.CellAt] is the read path and marks nothing; writing through the
// pointer it returns leaves the change invisible to the renderer.
func (s *Screen) Cell(x, y int) *buffer.Cell {
	c := s.front.CellAt(x, y)
	if c == nil {
		return nil
	}
	s.front.TouchLine(y, x, x+max(c.Width(), 1)-1)
	return c
}

// Size returns the managed area size in cells.
func (s *Screen) Size() layout.Size { return layout.Size{Width: s.width, Height: s.height} }

// Render diffs the desired frame, writes escape bytes, and flushes them.
func (s *Screen) Render() error {
	changed := s.renderer.SyncFront(s.front)
	if changed || s.cursorMovePending() {
		if err := s.writeFrame(); err != nil {
			return err
		}
	}
	return s.Flush()
}

// Invalidate forces a full redraw on the next Render.
func (s *Screen) Invalidate() { s.renderer.RequestClear() }

// Resize changes the managed area size.
//
// The next [Screen.Render] is a full repaint. That holds even when the size is
// unchanged: a resize report can follow a font or window change that keeps the
// cell grid but moves where those cells land, and the tracked contents are then
// wrong in a way no diff can see. An explicit resize says the caller wants the
// area re-established, so it is.
//
// [altscreen.io/vision/program.Program.Autoresize] runs on every resize
// report, including ones that change nothing, so it skips the call when the
// size already matches rather than repainting on each.
func (s *Screen) Resize(size layout.Size) {
	s.width, s.height = size.Width, size.Height
	s.front.Resize(size.Width, size.Height)
	s.renderer.RequestClear()
}

// InsertAbove inserts content above the managed area and flushes it.
func (s *Screen) InsertAbove(content string) error {
	if content == "" {
		return nil
	}
	s.out.WriteString("\r")
	y := s.renderer.CursorPosition().Y
	if down := s.height - y - 1; down > 0 {
		s.out.Write(ansi.AppendCUD(s.out.AvailableBuffer(), down))
	}
	lines := strings.Split(content, "\n")
	offset := len(lines)
	if s.width > 0 {
		for _, line := range lines {
			offset += s.StringWidth(line) / s.width
		}
	}
	for range offset {
		s.out.WriteString("\n")
	}
	if up := offset + s.height - 1; up > 0 {
		s.out.Write(ansi.AppendCUU(s.out.AvailableBuffer(), up))
	}
	s.out.Write(ansi.AppendInsertLines(s.out.AvailableBuffer(), offset))
	for _, line := range lines {
		s.out.WriteString(line)
		s.out.WriteString(ansi.EraseLineRight)
		s.out.WriteString("\r\n")
	}
	s.renderer.SetCursorPosition(layout.Pos(0, 0))
	return s.Flush()
}

// ParkCursor stages a move to the bottom row of the last rendered surface.
// It does nothing before the first render and ignores pending size changes.
// Call Flush to send the move before a shell handoff.
func (s *Screen) ParkCursor() error {
	width, height := s.renderer.LastSize()
	if height > 0 {
		return s.renderer.MoveCursorTo(&s.out, layout.Size{Width: width, Height: height}, height-1, 0)
	}
	return nil
}

// MoveCursorTo immediately moves the terminal cursor to (x, y) and flushes.
// Past an uncertain-width cluster, it re-emits the last rendered cells.
// It does not render staged edits. After invalidation, it uses control sequences.
func (s *Screen) MoveCursorTo(x, y int) error {
	if err := s.stageMoveCursorTo(layout.Pos(x, y)); err != nil {
		return err
	}
	return s.Flush()
}

// MoveCursorBy immediately moves the terminal cursor by dx and dy and flushes.
func (s *Screen) MoveCursorBy(dx, dy int) error {
	cur, ok := s.TrackedCursor()
	if !ok {
		cur = layout.Pos(0, 0)
	}
	return s.MoveCursorTo(max(cur.X+dx, 0), max(cur.Y+dy, 0))
}

// SetCursorPosition stages a resting cursor position for every Render.
func (s *Screen) SetCursorPosition(x, y int) { p := layout.Pos(x, y); s.desiredCursor = &p }

// ClearCursorPosition clears the staged resting cursor position.
func (s *Screen) ClearCursorPosition() { s.desiredCursor = nil }

// TrackedCursor returns the renderer's known cursor position.
// A move past an uncertain-width cluster leaves the tracked column unknown.
func (s *Screen) TrackedCursor() (layout.Position, bool) {
	if !s.renderer.CursorKnown() {
		return layout.Position{}, false
	}
	return s.renderer.CursorPosition(), true
}

// InvalidateTrackedCursor marks the renderer cursor position unknown.
func (s *Screen) InvalidateTrackedCursor() { s.renderer.InvalidateCursor() }

// SaveCursor saves the tracked cursor for RestoreCursor without emitting bytes.
func (s *Screen) SaveCursor() { s.renderer.SaveCursor() }

// RestoreCursor restores the saved tracked cursor without emitting bytes.
func (s *Screen) RestoreCursor() { s.renderer.RestoreCursor() }

// SetTrackedCursor records (x, y) without emitting a cursor move.
func (s *Screen) SetTrackedCursor(x, y int) { s.renderer.SetCursorPosition(layout.Pos(x, y)) }

// SetFullscreen records whether the managed area is the whole viewport.
func (s *Screen) SetFullscreen(fullscreen bool) {
	if s.fullscreen == fullscreen {
		return
	}
	s.fullscreen = fullscreen
	s.renderer.SetFullscreen(fullscreen)
	s.renderer.SetRelativeCursor(!fullscreen)
	if fullscreen {
		s.renderer.SaveCursor()
	} else {
		s.renderer.RestoreCursor()
	}
	s.renderer.RequestClear()
}

// Fullscreen reports whether the managed area is the whole viewport.
func (s *Screen) Fullscreen() bool { return s.fullscreen }

// SetCursorVisible records whether the terminal cursor is visible.
func (s *Screen) SetCursorVisible(visible bool) { s.cursorVisible = visible }

// CursorVisible reports whether the terminal cursor is recorded as visible.
func (s *Screen) CursorVisible() bool { return s.cursorVisible }

// SetGraphemeClusters chooses grapheme-cluster width measurement.
// With code-point measurement, the renderer repaints each changed row from
// its first uncertain-width cluster. Grapheme measurement requires a terminal
// that agrees with that policy.
func (s *Screen) SetGraphemeClusters(enabled bool) {
	if s.graphemeClusters == enabled {
		return
	}
	s.graphemeClusters = enabled
	s.renderer.SetWidthMethod(s.WidthMethod())
	s.Invalidate()
}

// GraphemeClusters reports whether grapheme-cluster measurement is enabled.
func (s *Screen) GraphemeClusters() bool { return s.graphemeClusters }

// SetSynchronizedOutput records whether frames are wrapped in synchronized output.
//
// It also permits the renderer's scroll-detection pass, which needs the whole
// frame presented in one step. See [Screen.SetScrollOptimize].
func (s *Screen) SetSynchronizedOutput(enabled bool) {
	s.syncOutput = enabled
	s.renderer.SetSyncOutput(enabled)
}

// SynchronizedOutput reports whether synchronized output is enabled.
func (s *Screen) SynchronizedOutput() bool { return s.syncOutput }

// SetColorProfile sets the color profile used for styled cells.
func (s *Screen) SetColorProfile(profile color.Profile) { s.renderer.SetColorProfile(profile) }

// ColorProfile returns the color profile used for styled cells.
func (s *Screen) ColorProfile() color.Profile { return s.renderer.ColorProfile() }

// SetOptimizations sets the renderer optimization flags.
func (s *Screen) SetOptimizations(opts Optimizations) { s.renderer.SetOptimizations(opts) }

// Optimizations returns the renderer optimization flags.
func (s *Screen) Optimizations() Optimizations { return s.renderer.Optimizations() }

// SetScrollOptimize turns the renderer's scroll-detection pass on or off. It is
// on by default and best left on: when a run of rows has simply moved, telling
// the terminal to move them costs a handful of bytes instead of a repaint.
//
// Enabling it permits detection rather than turning it on. Detection also
// requires fullscreen and [Screen.SynchronizedOutput], and rows are redrawn
// directly whenever any of the three is missing.
//
// Synchronized output is required because the scrolls emitted here are always
// full width. Rows move with SU, SD, IL, DL, or a line feed, and the renderer
// leaves the left and right margins alone, so a detected scroll carries a fixed
// sidebar, file tree, or gutter along with everything else and the same frame
// paints it back. The settled screen is correct either way, which is why
// comparing the finished screen finds nothing wrong. What a user sees is that
// region jumping and being put back, on every frame, for as long as the
// scrolling continues. Wrapping the frame presents it in one step, so the
// intermediate state never reaches the display.
//
// Synchronized output starts off, and a [altscreen.io/vision/program.Program]
// turns it on only once the terminal reports support, so an application that
// never queries capabilities gets direct redraws whatever its terminal
// supports. Setting this takes effect on the next [Screen.Render].
func (s *Screen) SetScrollOptimize(enabled bool) { s.renderer.SetScrollOptimize(enabled) }

// Bounds returns the screen bounds.
func (s *Screen) Bounds() layout.Rectangle { return s.front.Bounds() }

// CellAt returns the cell at (x, y).
func (s *Screen) CellAt(x, y int) *buffer.Cell { return s.front.CellAt(x, y) }

// WidthMethod returns the width method used for text painting.
func (s *Screen) WidthMethod() ansi.WidthMethod {
	if s.graphemeClusters {
		return ansi.GraphemeWidth
	}
	return ansi.WcWidth
}

// AmbiguousWide reports whether East Asian Ambiguous code points measure two
// cells under the screen policy.
func (s *Screen) AmbiguousWide() bool { return s.eawWide }

// SetString paints str at (x, y) with truncation at the right edge.
func (s *Screen) SetString(x, y int, str string, st style.Style) layout.Position {
	return text.SetString(s, x, y, str, st)
}

// SetStringWrap paints str at (x, y) with explicit right-edge handling.
func (s *Screen) SetStringWrap(x, y int, str string, wrap text.WrapMode, st style.Style) layout.Position {
	return text.SetStringWrap(s, x, y, str, wrap, st)
}

// StringWidth returns the terminal width of str under the screen policy.
func (s *Screen) StringWidth(str string) int {
	total := 0
	for _, w := range text.GraphemeCells(str, s.WidthMethod(), s.eawWide) {
		total += w
	}
	return total
}

// GraphemeCells returns grapheme clusters and widths under the screen policy.
func (s *Screen) GraphemeCells(str string) func(func(string, int) bool) {
	return text.GraphemeCells(str, s.WidthMethod(), s.eawWide)
}

func (s *Screen) writeFrame() error {
	bracketCursor := s.cursorVisible && !s.syncOutput
	if s.syncOutput {
		s.out.Write(ansi.AppendSetMode(s.out.AvailableBuffer(), ansi.SynchronizedOutputMode))
	}
	if bracketCursor {
		s.out.Write(ansi.AppendResetMode(s.out.AvailableBuffer(), ansi.CursorVisibleMode))
	}
	if err := s.renderer.RenderBack(&s.out); err != nil {
		return err
	}
	if s.desiredCursor != nil {
		pos := s.clampToSurface(*s.desiredCursor)
		if err := s.renderer.MoveToResting(&s.out, s.front, pos.Y, pos.X); err != nil {
			return err
		}
		s.cursorRestedAt = pos
		s.cursorRested = true
	}
	if bracketCursor {
		s.out.Write(ansi.AppendSetMode(s.out.AvailableBuffer(), ansi.CursorVisibleMode))
	}
	if s.syncOutput {
		s.out.Write(ansi.AppendResetMode(s.out.AvailableBuffer(), ansi.SynchronizedOutputMode))
	}
	return nil
}

func (s *Screen) stageMoveCursorTo(pos layout.Position) error {
	s.cursorRested = false
	return s.renderer.MoveCursorTo(&s.out, s.Size(), pos.Y, pos.X)
}

func (s *Screen) clampToSurface(pos layout.Position) layout.Position {
	return layout.Pos(min(pos.X, max(s.width-1, 0)), min(pos.Y, max(s.height-1, 0)))
}

func (s *Screen) cursorMovePending() bool {
	if s.desiredCursor == nil {
		return false
	}
	pos := s.clampToSurface(*s.desiredCursor)
	if s.cursorRested && s.cursorRestedAt == pos && s.renderer.CursorPlacedOnRow(pos.Y) {
		return false
	}
	return !s.renderer.CursorKnown() || s.renderer.CursorPosition() != pos
}

var (
	_ text.Surface = (*Screen)(nil)
	_ io.Writer    = (*Screen)(nil)
)
