package renderer

import (
	"unicode/utf8"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/buffer"
	"altscreen.io/vision/style"
)

const csiLen = 2

func DigitCount(n int) int {
	switch {
	case n < 10:
		return 1
	case n < 100:
		return 2
	case n < 1000:
		return 3
	case n < 10000:
		return 4
	default:
		return 5
	}
}

func csiOptionalParamCost(n int) int {
	if n <= 1 {
		return csiLen + 1
	}
	return csiLen + DigitCount(n) + 1
}

func csiAbsolutePositionCost(arg int) int {
	if arg == 0 {
		return csiLen + 1
	}
	return csiLen + DigitCount(arg+1) + 1
}

func csiEraseCost(n int) int {
	if n == 0 {
		return csiLen + 1
	}
	return csiLen + DigitCount(n) + 1
}

func CUPCost(row, col int) int {
	if row == 0 && col == 0 {
		return csiLen + 1
	}
	if col == 0 {
		return csiLen + DigitCount(row+1) + 1
	}
	return csiLen + DigitCount(row+1) + 1 + DigitCount(col+1) + 1
}
func CUUCost(n int) int   { return csiOptionalParamCost(n) }
func CUDCost(n int) int   { return csiOptionalParamCost(n) }
func CUFCost(n int) int   { return csiOptionalParamCost(n) }
func CUBCost(n int) int   { return csiOptionalParamCost(n) }
func CHACost(col int) int { return csiAbsolutePositionCost(col) }
func HPACost(col int) int { return csiAbsolutePositionCost(col) }
func VPACost(row int) int { return csiAbsolutePositionCost(row) }
func CHTCost(n int) int   { return csiOptionalParamCost(n) }
func CBTCost(n int) int   { return csiOptionalParamCost(n) }

const RICost = 2

func ICHCost(n int) int { return csiOptionalParamCost(n) }
func DCHCost(n int) int { return csiOptionalParamCost(n) }
func ECHCost(n int) int { return csiOptionalParamCost(n) }
func REPCost(n int) int { return csiOptionalParamCost(n) }
func ILCost(n int) int  { return csiOptionalParamCost(n) }
func DLCost(n int) int  { return csiOptionalParamCost(n) }
func SUCost(n int) int  { return csiOptionalParamCost(n) }
func SDCost(n int) int  { return csiOptionalParamCost(n) }
func ELCost(n int) int  { return csiEraseCost(n) }
func EDCost(n int) int  { return csiEraseCost(n) }
func DECSTBMCost(top, bottom int) int {
	return csiLen + DigitCount(top+1) + 1 + DigitCount(bottom+1) + 1
}

const (
	DECSTBMResetCost = 3
	CRCost           = 1
	HomeCost         = 3
)

func LFCost(n int) int  { return n }
func BSCost(n int) int  { return n }
func TabCost(n int) int { return n }

// crossableUnderEveryPolicy reports whether content draws exactly want columns
// however the terminal measures it.
//
// Neither pass knows the width policy the row was painted under, and the
// failure direction matters: refusing a cell whose width moves with the policy
// costs a cursor sequence, while accepting one lands the cursor somewhere the
// planner did not record. An East Asian Ambiguous glyph is one column normally
// and two in a CJK locale, and a lone regional indicator is two under grapheme
// measurement and one under wcwidth, so both are declined.
func crossableUnderEveryPolicy(content string, want int) bool {
	// Printable ASCII is one column under every policy, and it is what the
	// planner meets on nearly every candidate, so answer it without consulting
	// a table four times. A blank is included because a run of them is the
	// commonest candidate of all.
	if want == 1 && len(content) == 1 {
		if b := content[0]; b >= 0x20 && b < 0x7f {
			return true
		}
	}
	for _, mode := range [...]ansi.WidthMethod{ansi.GraphemeWidth, ansi.WcWidth} {
		for _, eawWide := range [...]bool{false, true} {
			if ansi.StringWidth([]byte(content), mode, eawWide) != want {
				return false
			}
		}
	}
	return true
}

// overwriteEligible reports whether an overwrite move may cross line[from:to]
// with st as the active pen, and returns the byte cost of the content it would
// write.
//
// This move travels by drawing, so what it draws has to span exactly the
// columns it claims to cross, and the pen has to survive the trip. A range is
// refused when:
//
//   - it opens on a continuation, which begins inside a glyph whose bytes have
//     already gone out, so the walk would arrive a column short
//   - the last cluster reaches past to, so drawing it would carry the cursor a
//     column too far
//   - it meets a continuation nothing stepped over, so no cell owns it and
//     nothing will draw its column while the walk still counts it crossed
//   - an occupied cell has a style the active pen does not match, which would
//     need an interleaved pen change
//   - a cell does not draw the width the row credits it with. A cell can claim
//     a width its content does not have: a combining mark or a control
//     character draws nothing, a narrow cell can hold a wide glyph, and a wide
//     one can hold a narrow glyph. Measuring the content answers that, where
//     counting code points only guessed
//   - an occupied cell holds anything other than exactly one code point. The
//     move is priced in bytes, and a joined emoji is closer to thirty of those
//     than to the two columns it occupies, so taking one code point at a time
//     keeps the price and the distance equal and keeps the move off sequences
//     whose rendered width a terminal may disagree about
//
// Both the planner and the emitter decide eligibility through this, because a
// planner that priced a move the emitter refuses would pick one it cannot
// produce.
func overwriteEligible(line []buffer.Cell, st style.Style, from, to int) (int, bool) {
	if from < 0 || to > len(line) || from > to {
		return 0, false
	}
	if from < len(line) && line[from].IsContinuation() {
		return 0, false
	}
	cost := 0
	i := from
	for i < to {
		cell := line[i]
		if cell.IsContinuation() {
			return 0, false
		}
		if !cell.Style.Equal(st) {
			return 0, false
		}
		content := cell.Content()
		if !singleCodePoint(content) {
			return 0, false
		}
		if !crossableUnderEveryPolicy(content, max(cell.Width(), 1)) {
			return 0, false
		}
		cost += len(content)
		i += max(cell.Width(), 1)
	}
	if i != to {
		return 0, false
	}
	return cost, true
}

// singleCodePoint reports whether s is exactly one code point.
func singleCodePoint(s string) bool {
	if s == "" {
		return false
	}
	_, size := utf8.DecodeRuneInString(s)
	return size == len(s)
}

// OverwriteCost returns the byte cost of re-emitting line[from:to] as a
// forward-move candidate, and whether the range is eligible at all.
//
// The cost is the summed byte length of the cells' content, which is what the
// move writes, so it compares directly against the byte cost of a cursor
// sequence. See [overwriteEligible] for what makes a range ineligible.
func OverwriteCost(line []buffer.Cell, st style.Style, from, to int) (int, bool) {
	return overwriteEligible(line, st, from, to)
}
