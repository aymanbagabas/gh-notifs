package ansi

import "strconv"

// Cursor movement.
//
// Every position here is zero-based and converted to the terminal's one-based
// wire form on the way out. The relative moves return the empty string for a
// count of zero and omit the parameter for a count of one, which is what keeps
// the renderer's per-frame output small.

// CUP moves the cursor to an absolute position: ESC [ row+1 ; col+1 H.
//
// The column is omitted when col is 0, and the origin is the bare ESC [ H.
func CUP(row, col int) string {
	var buf seqBuf
	return string(appendCUP(buf[:0], row, col))
}

// CursorPosition is an alias for [CUP].
func CursorPosition(row, col int) string { return CUP(row, col) }

// HVP moves the cursor to an absolute position: ESC [ row+1 ; col+1 f.
//
// It is the same movement as [CUP] with a different final byte.
func HVP(row, col int) string {
	switch {
	case row == 0 && col == 0:
		return "\x1b[f"
	case col == 0:
		return "\x1b[" + strconv.Itoa(row+1) + "f"
	default:
		return "\x1b[" + strconv.Itoa(row+1) + ";" + strconv.Itoa(col+1) + "f"
	}
}

// seqBuf is large enough for any CSI this package builds from a fixed number
// of integers: two parameters of ten digits, the introducer, a separator, and
// a final byte. It lets the string forms build without a heap buffer, leaving
// only the allocation the returned string itself needs. A variadic form that
// overruns it, such as a mode sequence naming several modes, grows through
// append and pays for one buffer instead of one per parameter.
type seqBuf [32]byte

// appendRelative appends a movement of n with the given final byte, omitting
// the parameter for 1 and appending nothing for 0.
func appendRelative(dst []byte, n int, final byte) []byte {
	switch {
	case n <= 0:
		return dst
	case n == 1:
		return append(dst, ESC, '[', final)
	default:
		dst = append(dst, ESC, '[')
		dst = strconv.AppendInt(dst, int64(n), 10)
		return append(dst, final)
	}
}

// relative renders a movement of n with the given final byte, omitting the
// parameter for 1 and returning nothing for 0.
func relative(n int, final byte) string {
	if n <= 0 {
		return ""
	}
	var buf seqBuf
	return string(appendRelative(buf[:0], n, final))
}

// appendAbsolute appends a move to a zero-based coordinate with the given final
// byte, omitting the parameter for 0.
func appendAbsolute(dst []byte, v int, final byte) []byte {
	if v == 0 {
		return append(dst, ESC, '[', final)
	}
	dst = append(dst, ESC, '[')
	dst = strconv.AppendInt(dst, int64(v+1), 10)
	return append(dst, final)
}

// absolute renders a move to a zero-based coordinate with the given final
// byte, omitting the parameter for 0.
func absolute(v int, final byte) string {
	var buf seqBuf
	return string(appendAbsolute(buf[:0], v, final))
}

// appendCUP appends a move to a zero-based row and column, omitting whichever
// trailing parameters are at their default.
func appendCUP(dst []byte, row, col int) []byte {
	dst = append(dst, ESC, '[')
	if row == 0 && col == 0 {
		return append(dst, 'H')
	}
	dst = strconv.AppendInt(dst, int64(row+1), 10)
	if col != 0 {
		dst = append(dst, ';')
		dst = strconv.AppendInt(dst, int64(col+1), 10)
	}
	return append(dst, 'H')
}

// CUU moves the cursor up n rows.
func CUU(n int) string { return relative(n, 'A') }

// CursorUp is an alias for [CUU].
func CursorUp(n int) string { return CUU(n) }

// CUD moves the cursor down n rows.
func CUD(n int) string { return relative(n, 'B') }

// CursorDown is an alias for [CUD].
func CursorDown(n int) string { return CUD(n) }

// CUF moves the cursor forward n columns.
func CUF(n int) string { return relative(n, 'C') }

// CursorForward is an alias for [CUF].
func CursorForward(n int) string { return CUF(n) }

// CUB moves the cursor backward n columns.
func CUB(n int) string { return relative(n, 'D') }

// CursorBackward is an alias for [CUB].
func CursorBackward(n int) string { return CUB(n) }

// CNL moves the cursor to the first column n rows down.
func CNL(n int) string { return relative(n, 'E') }

// CursorNextLine is an alias for [CNL].
func CursorNextLine(n int) string { return CNL(n) }

// CPL moves the cursor to the first column n rows up.
func CPL(n int) string { return relative(n, 'F') }

// CursorPreviousLine is an alias for [CPL].
func CursorPreviousLine(n int) string { return CPL(n) }

// HPR moves the cursor forward n columns, the horizontal-position-relative
// spelling of [CUF].
func HPR(n int) string { return relative(n, 'a') }

// VPR moves the cursor down n rows, the vertical-position-relative spelling of
// [CUD].
func VPR(n int) string { return relative(n, 'e') }

// CHT advances the cursor n horizontal tab stops.
func CHT(n int) string { return relative(n, 'I') }

// CursorHorizontalForwardTab is an alias for [CHT].
func CursorHorizontalForwardTab(n int) string { return CHT(n) }

// CHA moves the cursor to an absolute column.
func CHA(col int) string { return absolute(col, 'G') }

// CursorHorizontalAbsolute is an alias for [CHA].
func CursorHorizontalAbsolute(col int) string { return CHA(col) }

// HPA moves the cursor to an absolute column, the horizontal-position-absolute
// spelling of [CHA].
func HPA(col int) string { return absolute(col, '`') }

// VPA moves the cursor to an absolute row.
func VPA(row int) string { return absolute(row, 'd') }

// VerticalPositionAbsolute is an alias for [VPA].
func VerticalPositionAbsolute(row int) string { return VPA(row) }

// Cursor save, restore, and scrolling controls.
const (
	// SaveCursor saves the cursor position along with the pen and character
	// set: ESC 7.
	SaveCursor = "\x1b7"
	// DECSC is the standard name for [SaveCursor].
	DECSC = SaveCursor

	// RestoreCursor restores what [SaveCursor] stored: ESC 8.
	RestoreCursor = "\x1b8"
	// DECRC is the standard name for [RestoreCursor].
	DECRC = RestoreCursor

	// SaveCursorPosition saves only the cursor position: ESC [ s.
	SaveCursorPosition = "\x1b[s"
	// SCOSC is the standard name for [SaveCursorPosition].
	SCOSC = SaveCursorPosition

	// RestoreCursorPosition restores what [SaveCursorPosition] stored:
	// ESC [ u.
	RestoreCursorPosition = "\x1b[u"
	// SCORC is the standard name for [RestoreCursorPosition].
	SCORC = RestoreCursorPosition

	// Index moves the cursor down one row, scrolling if it is on the last
	// row: ESC D. The C1 byte of the same name is [IND].
	Index = "\x1bD"

	// ReverseIndex moves the cursor up one row, scrolling if it is on the
	// first row: ESC M. The C1 byte of the same name is [RI].
	ReverseIndex = "\x1bM"

	// Tab advances to the next tab stop. It is the literal HT byte, not a
	// CSI sequence, so the terminal's own tab stops decide where it lands.
	Tab = "\t"
)

// Cursor position requests.
const (
	// RequestCursorPosition asks for a cursor position report: ESC [ 6 n.
	// The reply is ESC [ row ; col R, one-based.
	RequestCursorPosition = "\x1b[6n"

	// RequestExtendedCursorPosition asks for a report that also carries the
	// page: ESC [ ? 6 n. The reply is ESC [ ? row ; col ; page R.
	RequestExtendedCursorPosition = "\x1b[?6n"
)

// CBT moves the cursor back n horizontal tab stops.
func CBT(n int) string { return relative(n, 'Z') }

// CursorBackwardTab is an alias for [CBT].
func CursorBackwardTab(n int) string { return CBT(n) }

// CursorShape is the visual shape of the text cursor, independent of whether
// it blinks.
//
// DECSCUSR interleaves the two: each shape has a blinking and a steady
// parameter. [NewCursorStyle] combines them, and [CursorStyle.Shape] and
// [CursorStyle.Blinking] take them apart again.
type CursorShape uint8

// Cursor shapes.
const (
	// CursorBlock is a full-cell block.
	CursorBlock CursorShape = iota
	// CursorUnderline is a horizontal underline at the bottom of the cell.
	CursorUnderline
	// CursorBar is a vertical bar at the left of the cell.
	CursorBar
)

// CursorStyle is the shape and blink of the cursor, as set by DECSCUSR.
type CursorStyle uint8

// Cursor styles. The values are the DECSCUSR parameter each one emits.
const (
	// CursorDefault restores the terminal's configured cursor.
	CursorDefault CursorStyle = iota
	// CursorBlinkingBlock is a blinking block.
	CursorBlinkingBlock
	// CursorSteadyBlock is a steady block.
	CursorSteadyBlock
	// CursorBlinkingUnderline is a blinking underline.
	CursorBlinkingUnderline
	// CursorSteadyUnderline is a steady underline.
	CursorSteadyUnderline
	// CursorBlinkingBar is a blinking vertical bar.
	CursorBlinkingBar
	// CursorSteadyBar is a steady vertical bar.
	CursorSteadyBar
)

// SetCursorStyle sets the cursor shape and blink with DECSCUSR:
// ESC [ style SP q.
func SetCursorStyle(style CursorStyle) string {
	return "\x1b[" + strconv.Itoa(int(style)) + " q"
}

// DECSCUSR is an alias for [SetCursorStyle].
func DECSCUSR(style CursorStyle) string { return SetCursorStyle(style) }

// CursorStyleFromParam returns the style a DECSCUSR parameter names.
//
// The second result is false for a parameter no style uses, which a terminal
// reporting its setting should never send.
func CursorStyleFromParam(n int) (CursorStyle, bool) {
	if n < int(CursorDefault) || n > int(CursorSteadyBar) {
		return 0, false
	}
	return CursorStyle(n), true
}

// NewCursorStyle returns the style that draws shape, blinking or steady.
func NewCursorStyle(shape CursorShape, blinking bool) CursorStyle {
	switch shape {
	case CursorUnderline:
		if blinking {
			return CursorBlinkingUnderline
		}
		return CursorSteadyUnderline
	case CursorBar:
		if blinking {
			return CursorBlinkingBar
		}
		return CursorSteadyBar
	default:
		if blinking {
			return CursorBlinkingBlock
		}
		return CursorSteadyBlock
	}
}

// Shape returns the shape s draws.
//
// The second result is false for [CursorDefault], which defers to the terminal
// rather than naming a shape.
func (s CursorStyle) Shape() (CursorShape, bool) {
	switch s {
	case CursorBlinkingBlock, CursorSteadyBlock:
		return CursorBlock, true
	case CursorBlinkingUnderline, CursorSteadyUnderline:
		return CursorUnderline, true
	case CursorBlinkingBar, CursorSteadyBar:
		return CursorBar, true
	default:
		return 0, false
	}
}

// Blinking reports whether s blinks.
//
// The second result is false for [CursorDefault], which defers to the terminal
// rather than choosing.
func (s CursorStyle) Blinking() (bool, bool) {
	switch s {
	case CursorBlinkingBlock, CursorBlinkingUnderline, CursorBlinkingBar:
		return true, true
	case CursorSteadyBlock, CursorSteadyUnderline, CursorSteadyBar:
		return false, true
	default:
		return false, false
	}
}

// SetPointerShape sets the mouse pointer shape with OSC 22, naming a CSS
// cursor such as "pointer" or "text".
func SetPointerShape(shape string) string {
	return "\x1b]22;" + shape + "\x1b\\"
}
