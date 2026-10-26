package event

import (
	"fmt"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"altscreen.io/vision/ansi"
	winapi "altscreen.io/vision/internal/windows"
)

// lastSize remembers the last buffer size reported, so a resize record that
// repeats it can be suppressed.
type lastSize struct {
	size  winapi.Coord
	valid bool
}

// serializeKeyEvent writes the byte grammar for one key record.
//
// In virtual-terminal input mode it emits UTF-8, buffering a high surrogate
// until its low half arrives. The buffer is indexed by key direction so a
// key-down pair and a key-up pair never mix. Outside that mode a record with no
// virtual key code emits its character directly, while any other record emits a
// win32-input-mode sequence the shared decoder understands.
func serializeKeyEvent(kev winapi.KeyEventRecord, vtInput bool, pendingHighSurrogate *[2]uint16, buf []byte) []byte {
	idx := 0
	if kev.KeyDown {
		idx = 1
	}
	ch := kev.Char

	if vtInput {
		if high := pendingHighSurrogate[idx]; high != 0 {
			pendingHighSurrogate[idx] = 0
			if r := utf16.DecodeRune(rune(high), rune(ch)); r != unicode.ReplacementChar {
				buf = utf8.AppendRune(buf, r)
			}
			return buf
		}
		switch {
		case utf16.IsSurrogate(rune(ch)):
			// A high half opens a pair and waits for its low half. A low half
			// arriving with nothing pending has no high half to join, so it is
			// dropped rather than emitted: a surrogate is not a scalar value on
			// its own. The standard library names the pair's boundary only
			// internally, so the halves are told apart here.
			if ch < 0xDC00 {
				pendingHighSurrogate[idx] = ch
			}
		case kev.KeyDown && ch != 0:
			if r, ok := scalarFromUTF16(ch); ok {
				buf = utf8.AppendRune(buf, r)
			}
		}
		return buf
	}

	if kev.VirtualKeyCode == 0 {
		if kev.KeyDown && ch != 0 {
			if r, ok := scalarFromUTF16(ch); ok {
				buf = utf8.AppendRune(buf, r)
			}
		}
		return buf
	}

	down := 0
	if kev.KeyDown {
		down = 1
	}
	return fmt.Appendf(buf, "\x1b[%d;%d;%d;%d;%d;%d_",
		kev.VirtualKeyCode, kev.VirtualScanCode, ch, down, kev.ControlKeyState, kev.RepeatCount)
}

// serializeMouseEvent writes the SGR mouse byte grammar for one mouse record,
// diffing the button state against lastButtons to tell a press from a release
// and folding in the move bit, wheel codes, and modifier bits.
func serializeMouseEvent(mev winapi.MouseEventRecord, lastButtons *uint32, buf []byte) []byte {
	alt := mev.ControlKeyState&(winapi.LEFT_ALT_PRESSED|winapi.RIGHT_ALT_PRESSED) != 0
	ctrl := mev.ControlKeyState&(winapi.LEFT_CTRL_PRESSED|winapi.RIGHT_CTRL_PRESSED) != 0
	shift := mev.ControlKeyState&winapi.SHIFT_PRESSED != 0
	wheelDelta := highWordSigned(mev.ButtonState)

	var cb int
	var release, move, ok bool
	switch mev.EventFlags {
	case 0:
		cb, release, ok = mouseButtonChange(*lastButtons, mev.ButtonState)
	case winapi.MOUSE_MOVED:
		cb, _, ok = mouseButtonChange(*lastButtons, mev.ButtonState)
		move = true
	case winapi.MOUSE_WHEELED:
		if wheelDelta > 0 {
			cb = 64
		} else {
			cb = 65
		}
		ok = true
	case winapi.MOUSE_HWHEELED:
		if wheelDelta > 0 {
			cb = 67
		} else {
			cb = 66
		}
		ok = true
	}
	*lastButtons = mev.ButtonState
	if !ok {
		return buf
	}
	if move {
		cb |= 32
	}
	if shift {
		cb |= 4
	}
	if alt {
		cb |= 8
	}
	if ctrl {
		cb |= 16
	}
	final := 'M'
	if release {
		final = 'm'
	}
	return fmt.Appendf(buf, "\x1b[<%d;%d;%d%c", cb, int(mev.MousePosition.X)+1, int(mev.MousePosition.Y)+1, final)
}

// serializeResizeEvent writes an in-band resize sequence for one buffer-size
// record, suppressing it when the size has not changed. Pixel dimensions are
// unavailable from the console and reported as zero.
func serializeResizeEvent(wev winapi.WindowBufferSizeRecord, last *lastSize, buf []byte) []byte {
	if last.valid && last.size == wev.Size {
		return buf
	}
	last.size, last.valid = wev.Size, true
	return fmt.Appendf(buf, "\x1b[48;%d;%d;0;0t", wev.Size.Y, wev.Size.X)
}

// serializeFocusEvent writes the focus report for one focus record.
func serializeFocusEvent(fev winapi.FocusEventRecord, buf []byte) []byte {
	if fev.SetFocus {
		return append(buf, ansi.Focus...)
	}
	return append(buf, ansi.Blur...)
}

// serializeRecord dispatches one console record to the matching serializer.
// Menu records are reserved and produce nothing, as do unknown types.
func serializeRecord(rec winapi.InputRecord, vtInput bool, pendingHighSurrogate *[2]uint16, lastButtons *uint32, last *lastSize, buf []byte) []byte {
	switch rec.EventType {
	case winapi.KEY_EVENT:
		return serializeKeyEvent(rec.KeyEvent(), vtInput, pendingHighSurrogate, buf)
	case winapi.MOUSE_EVENT:
		return serializeMouseEvent(rec.MouseEvent(), lastButtons, buf)
	case winapi.WINDOW_BUFFER_SIZE_EVENT:
		return serializeResizeEvent(rec.WindowBufferSizeEvent(), last, buf)
	case winapi.FOCUS_EVENT:
		return serializeFocusEvent(rec.FocusEvent(), buf)
	default:
		return buf
	}
}

// scalarFromUTF16 returns the rune for a UTF-16 code unit, reporting false for a
// surrogate half, which is not a Unicode scalar value on its own.
func scalarFromUTF16(u uint16) (rune, bool) {
	if utf16.IsSurrogate(rune(u)) {
		return 0, false
	}
	return rune(u), true
}

// highWordSigned returns the signed high word of v, the form a console wheel
// delta arrives in.
func highWordSigned(v uint32) int16 {
	return int16(uint16(v >> 16))
}

// mouseButtonChange diffs two console button bitmasks into an SGR button code
// (left 0, middle 1, right 2) and whether the change was a release. When no bit
// changed but a button is held, it reports that button so a drag can be tagged.
// The bool result is false when there is nothing to report.
func mouseButtonChange(prev, current uint32) (button int, released, ok bool) {
	buttons := [...]struct {
		mask uint32
		code int
	}{
		{winapi.FROM_LEFT_1ST_BUTTON_PRESSED, 0},
		{winapi.RIGHTMOST_BUTTON_PRESSED, 2},
		{winapi.FROM_LEFT_2ND_BUTTON_PRESSED, 1},
	}
	for _, b := range buttons {
		was := prev&b.mask != 0
		now := current&b.mask != 0
		if was != now {
			return b.code, was, true
		}
	}
	if current != 0 {
		for _, b := range buttons {
			if current&b.mask != 0 {
				return b.code, false, true
			}
		}
	}
	return 0, false, false
}
