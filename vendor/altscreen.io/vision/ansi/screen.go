package ansi

import "strconv"

func screenCSICount(n int, final string) string {
	switch {
	case n <= 1:
		return "\x1b[" + final
	default:
		return "\x1b[" + strconv.Itoa(n) + final
	}
}

// ECH erases n character cells at the cursor.
func ECH(n int) string { return screenCSICount(n, "X") }

// REP repeats the preceding printable character n times.
func REP(n int) string { return screenCSICount(n, "b") }

// ICH inserts n blank character cells at the cursor.
func ICH(n int) string { return screenCSICount(n, "@") }

// DCH deletes n character cells at the cursor.
func DCH(n int) string { return screenCSICount(n, "P") }

// EL erases in line.
func EL(n int) string {
	if n == 0 {
		return "\x1b[K"
	}
	return "\x1b[" + strconv.Itoa(n) + "K"
}

// ED erases in display.
func ED(n int) string {
	if n == 0 {
		return "\x1b[J"
	}
	return "\x1b[" + strconv.Itoa(n) + "J"
}

const (
	EraseLineRight     = "\x1b[K"
	EraseLineLeft      = "\x1b[1K"
	EraseEntireLine    = "\x1b[2K"
	EraseScreenBelow   = "\x1b[J"
	EraseScreenAbove   = "\x1b[1J"
	EraseEntireScreen  = "\x1b[2J"
	EraseEntireDisplay = "\x1b[3J"
)

// EraseToEOL erases from the cursor through the end of the line.
func EraseToEOL() string { return EraseLineRight }

// EraseLine erases the entire current line.
func EraseLine() string { return EraseEntireLine }

// EraseBelow erases from the cursor through the end of the screen.
func EraseBelow() string { return EraseScreenBelow }

// EraseScreen erases the visible screen.
func EraseScreen() string { return EraseEntireScreen }

// InsertLines inserts n blank lines.
func InsertLines(n int) string { return screenCSICount(n, "L") }

// DeleteLines deletes n lines.
func DeleteLines(n int) string { return screenCSICount(n, "M") }

// ScrollUp scrolls up by n lines.
func ScrollUp(n int) string { return screenCSICount(n, "S") }

// ScrollDown scrolls down by n lines.
func ScrollDown(n int) string { return screenCSICount(n, "T") }

// ScrollRegion sets zero-based top and bottom margins.
func ScrollRegion(top, bottom int) string {
	return "\x1b[" + strconv.Itoa(top+1) + ";" + strconv.Itoa(bottom+1) + "r"
}

// ResetScrollRegion resets top and bottom margins to the full screen.
func ResetScrollRegion() string { return "\x1b[r" }

// SetLeftRightMargins sets left and right margins.
func SetLeftRightMargins(left, right int) string {
	switch {
	case left == 0 && right == 0:
		return "\x1b[s"
	case right == 0:
		return "\x1b[" + strconv.Itoa(left) + "s"
	case left == 0:
		return "\x1b[;" + strconv.Itoa(right) + "s"
	default:
		return "\x1b[" + strconv.Itoa(left) + ";" + strconv.Itoa(right) + "s"
	}
}

const (
	SetTabEvery8Columns = "\x1b[?5W"
	HorizontalTabSet    = "\x1bH"
)

// HTS7 sets a horizontal tab stop with the 7-bit spelling ESC H.
func HTS7() string { return HorizontalTabSet }

// TBC clears tab stops.
func TBC(n int) string {
	if n == 0 {
		return "\x1b[g"
	}
	return "\x1b[" + strconv.Itoa(n) + "g"
}

// ResetTabStopsEvery8 clears stops and re-establishes one every eight columns through width.
func ResetTabStopsEvery8(width int) string {
	s := "\r" + TBC(3)
	moved := false
	for col := 8; col < width; col += 8 {
		s += CUF(8) + HorizontalTabSet
		moved = true
	}
	if moved {
		s += "\r"
	}
	return s
}
