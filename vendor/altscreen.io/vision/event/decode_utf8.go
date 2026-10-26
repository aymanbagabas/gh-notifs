package event

import (
	"unicode/utf8"
)

func (d *decoder) parseUTF8(buf []byte) parseResult {
	b := buf[0]
	if b < 0xc0 {
		return resultNone(1)
	}
	n := 0
	if b < 0xe0 {
		n = 2
	} else if b < 0xf0 {
		n = 3
	} else if b < 0xf8 {
		n = 4
	} else {
		return resultNone(1)
	}
	if len(buf) < n {
		return resultIncomplete()
	}
	r, size := utf8.DecodeRune(buf[:n])
	if r == utf8.RuneError && size == 1 {
		return resultNone(1)
	}
	return resultEvent(KeyPressEvent(NewKey(r, 0).Normalized()), n)
}
