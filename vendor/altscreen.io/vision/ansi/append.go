package ansi

import "strconv"

// Append forms build a sequence into a buffer the caller owns.
//
// They exist because the [io.Writer] variants cannot avoid the sequence
// string: WriteX is io.WriteString(w, X(...)), so the string is still built on
// the heap before it is written, and staging into a local array inside WriteX
// would only move the allocation, because the buffer escapes through the
// writer interface. A caller that emits sequences on every frame holds one
// buffer, appends into it, and writes that.
//
// This is a partial family, not a counterpart to every sequence: it covers
// what the renderer emits per frame, and grows on demand when a new call site
// is shown to allocate in a loop. Generating one for all 165 sequences was
// measured and rejected; see the Performance section of AGENTS.md.

// AppendCUP appends [CUP] to dst and returns the extended buffer.
func AppendCUP(dst []byte, row, col int) []byte { return appendCUP(dst, row, col) }

// AppendCUU appends [CUU] to dst and returns the extended buffer.
func AppendCUU(dst []byte, n int) []byte { return appendRelative(dst, n, 'A') }

// AppendCUD appends [CUD] to dst and returns the extended buffer.
func AppendCUD(dst []byte, n int) []byte { return appendRelative(dst, n, 'B') }

// AppendCUF appends [CUF] to dst and returns the extended buffer.
func AppendCUF(dst []byte, n int) []byte { return appendRelative(dst, n, 'C') }

// AppendCUB appends [CUB] to dst and returns the extended buffer.
func AppendCUB(dst []byte, n int) []byte { return appendRelative(dst, n, 'D') }

// AppendCHT appends [CHT] to dst and returns the extended buffer.
func AppendCHT(dst []byte, n int) []byte { return appendRelative(dst, n, 'I') }

// AppendCBT appends [CBT] to dst and returns the extended buffer.
func AppendCBT(dst []byte, n int) []byte { return appendRelative(dst, n, 'Z') }

// AppendCHA appends [CHA] to dst and returns the extended buffer.
func AppendCHA(dst []byte, col int) []byte { return appendAbsolute(dst, col, 'G') }

// AppendHPA appends [HPA] to dst and returns the extended buffer.
func AppendHPA(dst []byte, col int) []byte { return appendAbsolute(dst, col, '`') }

// AppendVPA appends [VPA] to dst and returns the extended buffer.
func AppendVPA(dst []byte, row int) []byte { return appendAbsolute(dst, row, 'd') }

// AppendECH appends [ECH] to dst and returns the extended buffer.
func AppendECH(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'X') }

// AppendREP appends [REP] to dst and returns the extended buffer.
func AppendREP(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'b') }

// AppendICH appends [ICH] to dst and returns the extended buffer.
func AppendICH(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, '@') }

// AppendDCH appends [DCH] to dst and returns the extended buffer.
func AppendDCH(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'P') }

// AppendInsertLines appends [InsertLines] to dst and returns the extended
// buffer.
func AppendInsertLines(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'L') }

// AppendDeleteLines appends [DeleteLines] to dst and returns the extended
// buffer.
func AppendDeleteLines(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'M') }

// AppendScrollUp appends [ScrollUp] to dst and returns the extended buffer.
func AppendScrollUp(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'S') }

// AppendScrollDown appends [ScrollDown] to dst and returns the extended
// buffer.
func AppendScrollDown(dst []byte, n int) []byte { return appendScreenCSICount(dst, n, 'T') }

// AppendScrollRegion appends [ScrollRegion] to dst and returns the extended
// buffer.
func AppendScrollRegion(dst []byte, top, bottom int) []byte {
	dst = append(dst, ESC, '[')
	dst = strconv.AppendInt(dst, int64(top+1), 10)
	dst = append(dst, ';')
	dst = strconv.AppendInt(dst, int64(bottom+1), 10)
	return append(dst, 'r')
}

// AppendHyperlink appends [Hyperlink] to dst and returns the extended buffer.
//
// The string form is a concatenation, which the compiler sizes and allocates
// once. This form is for a caller already holding a buffer, so that a style
// diff crossing a link transition adds no allocation of its own.
func AppendHyperlink(dst []byte, url, params string) []byte {
	dst = append(dst, "\x1b]8;"...)
	dst = append(dst, params...)
	dst = append(dst, ';')
	dst = append(dst, url...)
	return append(dst, "\x1b\\"...)
}

// AppendSetMode appends [SetMode] to dst and returns the extended buffer.
func AppendSetMode(dst []byte, modes ...Mode) []byte { return appendModeSeq(dst, 'h', modes) }

// AppendResetMode appends [ResetMode] to dst and returns the extended buffer.
func AppendResetMode(dst []byte, modes ...Mode) []byte { return appendModeSeq(dst, 'l', modes) }

// appendScreenCSICount appends a CSI carrying a count with the given final
// byte, omitting the parameter at its default of one.
func appendScreenCSICount(dst []byte, n int, final byte) []byte {
	dst = append(dst, ESC, '[')
	if n > 1 {
		dst = strconv.AppendInt(dst, int64(n), 10)
	}
	return append(dst, final)
}

// appendModeSeq appends the DEC group followed by the ANSI group, each as one
// CSI with its parameters separated by semicolons, because their prefixes
// differ. Either group is skipped when no mode belongs to it.
func appendModeSeq(dst []byte, final byte, modes []Mode) []byte {
	for _, dec := range [...]bool{true, false} {
		open := false
		for _, m := range modes {
			if m.dec != dec {
				continue
			}
			if open {
				dst = append(dst, ';')
			} else {
				dst = append(dst, ESC, '[')
				if dec {
					dst = append(dst, '?')
				}
				open = true
			}
			dst = strconv.AppendInt(dst, int64(m.n), 10)
		}
		if open {
			dst = append(dst, final)
		}
	}
	return dst
}
