package renderer

import (
	"hash/maphash"
	"image/color"
	"unicode"
	"unicode/utf8"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	termcolor "altscreen.io/vision/color"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/style"
	"altscreen.io/vision/unicode/graphemeaw"
)

type cursor struct {
	style style.Style
	// The column and row are tracked separately because either can be unknown
	// on its own: a carriage return fixes the column while leaving the row
	// undetermined. Flags rather than pointers, so moving the cursor does not
	// allocate.
	x, y       int
	hasX, hasY bool
	atPhantom  bool
	blank      buffer.Cell
	blankDirty bool
	bceBlank   buffer.Cell
	bceKey     bceKey
}

type bceKey struct {
	valid, bce bool
	bg         bgKey
}

// bgKey identifies a background for the blank cache. bceKey is compared with
// ==, and color.Color is an open interface whose implementations need not be
// comparable, so holding one directly panics on a color backed by a slice or a
// map. Resolving to RGBA sidesteps that, and it is the same equality styles
// already compare colors by, so a substitute with identical channels is
// indistinguishable everywhere the blank is used.
type bgKey struct {
	r, g, b, a uint32
	set        bool
}

func newBgKey(c color.Color) bgKey {
	if c == nil {
		return bgKey{}
	}
	r, g, b, a := c.RGBA()
	return bgKey{r: r, g: g, b: b, a: a, set: true}
}

func newCursor() cursor {
	return cursor{blank: buffer.BlankCell(), bceBlank: buffer.BlankCell(), bceKey: bceKey{valid: true}}
}

func (c *cursor) pos() layout.Position {
	x, y := 0, 0
	if c.hasX {
		x = c.x
	}
	if c.hasY {
		y = c.y
	}
	return layout.Pos(x, y)
}

func (c *cursor) setPos(p layout.Position) { c.setX(p.X); c.setY(p.Y) }
func (c *cursor) setX(v int)               { c.x, c.hasX = v, true }
func (c *cursor) setY(v int)               { c.y, c.hasY = v, true }
func (c *cursor) forget()                  { c.hasX, c.hasY = false, false }
func (c *cursor) known() bool              { return c.hasX && c.hasY }
func (c *cursor) setStyle(st style.Style)  { c.style = st; c.markPenChanged() }
func (c *cursor) markPenChanged()          { c.blankDirty = true; c.bceKey.valid = false }
func (c *cursor) currentBlank() *buffer.Cell {
	if c.blankDirty {
		c.blank = buffer.BlankCell().WithStyle(c.style)
		c.blankDirty = false
	}
	return &c.blank
}

// Renderer owns terminal diff state used by the renderer pipeline.
type Renderer struct {
	curBuf                                                 *RenderBuffer
	backBuf                                                *RenderBuffer
	oldHashes, newHashes                                   []uint64
	oldnum                                                 []int
	cur, saved                                             cursor
	opts                                                   Optimizations
	colorProfile                                           colorCache
	fullscreen, relativeCursor, scrollOptimize, forceClear bool
	// syncOutput records that the caller brackets each frame in synchronized
	// output. The markers themselves come from Screen; the renderer only needs
	// to know whether the frame it is building will be presented in one step.
	syncOutput            bool
	lastWidth, lastHeight int
	tabs                  *TabStops
	// emitBuf stages one pen transition or escape sequence. Every emit path
	// builds through Append forms into it and writes immediately, so a frame's
	// style changes, cursor moves, erases, and scrolls cost no allocations.
	emitBuf []byte
	// spaces holds the longest blank glyph; shorter glyphs share its prefix.
	spaces string
	// hashBuf and hashSeed back line hashing, and hashtab backs the scroll
	// hash table. All three are reused so scroll detection, which runs every
	// frame and mostly fails, does not allocate to do it.
	hashBuf     []byte
	hashSeed    maphash.Seed
	hashtab     []scrollHashEntry
	widthMethod ansi.WidthMethod
}

func NewRenderer() *Renderer {
	return &Renderer{backBuf: NewRenderBuffer(0, 0), cur: newCursor(), saved: newCursor(), opts: DefaultOptimizations(), colorProfile: newColorCache(termcolor.TrueColor), relativeCursor: true, scrollOptimize: true, tabs: DefaultTabStops(0), hashSeed: maphash.MakeSeed(), widthMethod: ansi.WcWidth}
}

// SetWidthMethod records the width policy used to measure the cells.
func (r *Renderer) SetWidthMethod(method ansi.WidthMethod) { r.widthMethod = method }

func (r *Renderer) widthIsUncertain(cell *buffer.Cell) bool {
	content := cell.Content()
	if r.widthMethod != ansi.WcWidth || len(content) <= 1 {
		return false
	}
	_, first := utf8.DecodeRuneInString(content)
	for _, c := range content[first:] {
		if c == '\ufe0e' || c == '\ufe0f' || c == '\u20e3' {
			return true
		}
		if unicode.IsControl(c) {
			continue
		}
		props, _ := graphemeaw.LookupRune(c)
		if props.Width(false) != 0 {
			return true
		}
	}
	return false
}

// CursorPlacedOnRow reports a known row with a column only the terminal knows.
func (r *Renderer) CursorPlacedOnRow(y int) bool { return !r.cur.hasX && r.cur.hasY && r.cur.y == y }

func (r *Renderer) SetOptimizations(opts Optimizations) { r.opts = opts }
func (r *Renderer) Optimizations() Optimizations        { return r.opts }
func (r *Renderer) SetColorProfile(p termcolor.Profile) {
	if r.colorProfile.Profile() != p {
		r.RequestClear()
	}
	r.colorProfile.SetProfile(p)
}
func (r *Renderer) ColorProfile() termcolor.Profile     { return r.colorProfile.Profile() }
func (r *Renderer) SetFullscreen(v bool)                { r.fullscreen = v }
func (r *Renderer) SetRelativeCursor(v bool)            { r.relativeCursor = v }
func (r *Renderer) SetScrollOptimize(v bool)            { r.scrollOptimize = v }
func (r *Renderer) SetSyncOutput(v bool)                { r.syncOutput = v }
func (r *Renderer) CursorPosition() layout.Position     { return r.cur.pos() }
func (r *Renderer) CursorKnown() bool                   { return r.cur.known() }
func (r *Renderer) LastSize() (int, int)                { return r.lastWidth, r.lastHeight }
func (r *Renderer) SetCursorPosition(p layout.Position) { r.cur.setPos(p); r.cur.atPhantom = false }
func (r *Renderer) SaveCursor()                         { r.saved = r.cur }
func (r *Renderer) RestoreCursor()                      { r.cur = r.saved }
func (r *Renderer) RequestClear()                       { r.forceClear = true }
func (r *Renderer) InvalidateCursor()                   { r.cur.forget(); r.cur.atPhantom = false }
