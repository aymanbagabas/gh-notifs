package event

import "altscreen.io/vision/ansi"

type decoder struct {
	buf                []byte
	flags              DecoderFlags
	backspace          bool
	inPaste            bool
	utf8Mouse          bool
	expired            bool
	pending            []Event
	win32LastCKS       int
	win32HighSurrogate [2]rune
}

func newDecoder(flags DecoderFlags) *decoder {
	return &decoder{flags: flags, buf: make([]byte, 0, 256)}
}

func (d *decoder) hasPending() bool {
	return len(d.buf) > 0 && !d.inPaste && (d.buf[0] == ansi.ESC || isC1Introducer(d.buf[0]))
}

func (d *decoder) drain() []Event {
	if d.inPaste || len(d.buf) == 0 {
		return nil
	}
	d.expired = true
	evs := d.parse(nil)
	d.expired = false
	return evs
}

func (d *decoder) setUTF8Mouse(enabled bool)     { d.utf8Mouse = enabled }
func (d *decoder) inPasteMode() bool             { return d.inPaste }
func (d *decoder) backspaceMode() bool           { return d.backspace }
func (d *decoder) setBackspaceMode(enabled bool) { d.backspace = enabled }

func (d *decoder) endPaste() Event {
	if !d.inPaste {
		return nil
	}
	d.inPaste = false
	return PasteEndEvent{}
}

func (d *decoder) expireLeading(b byte) Event {
	if b == ansi.ESC {
		return KeyPressEvent(d.loneEscape())
	}
	if isC1Introducer(b) {
		return KeyPressEvent(NewKey(c1FallbackRune(b), KeyModCtrl|KeyModAlt).Normalized())
	}
	return nil
}

// loneEscape names the key a lone 0x1b is, which is Escape unless the caller
// asked for the Ctrl spelling of that byte.
//
// Every path where the byte resolves as a key of its own comes here: a source
// reaching its escape deadline, whether the ESC stood alone or headed a
// sequence that never finished, and the inner ESC of a run of them. Naming it
// separately in each of those is how the flag came to reach one and not the
// rest.
func (d *decoder) loneEscape() Key {
	if d.flags.Contains(DecoderFlagCtrlOpenBracket) {
		return NewKey('[', KeyModCtrl).Normalized()
	}
	return NewKey(KeyEscape, KeyModNone).Normalized()
}

func (d *decoder) parseOne(data []byte) (int, Event) {
	if len(d.pending) > 0 {
		ev := d.pending[0]
		d.pending = d.pending[1:]
		return 0, ev
	}
	if len(data) == 0 {
		return 0, nil
	}
	if d.inPaste {
		return d.parsePasteChunk(data)
	}
	r := d.tryParse(data)
	if r.incomplete {
		return 0, nil
	}
	if r.none {
		if r.consumed > 0 {
			return r.consumed, nil
		}
		return 1, UnknownEvent([]byte{data[0]})
	}
	if _, ok := r.event.(PasteStartEvent); ok {
		d.inPaste = true
	}
	if group, ok := r.event.(MultiEvent); ok {
		if len(group) == 0 {
			return r.consumed, nil
		}
		d.pending = append(d.pending, group[1:]...)
		return r.consumed, group[0]
	}
	return r.consumed, r.event
}

func c1FallbackRune(b byte) rune {
	r := rune(b - 0x40)
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// maxBufferedBytes caps a single undecoded sequence. Bytes stay buffered while
// a sequence is incomplete, so a terminal that opens one and never terminates
// it would grow the buffer for as long as it keeps sending. Reaching the cap
// means the parser cannot make sense of what is held, so the partial sequence
// is abandoned and decoding resumes with whatever arrives next.
//
// This is the same limit uncurses sets, where the read buffer is fixed at this
// size and is documented as the hard cap on any single sequence. It bounds a
// legitimate sequence too: an OSC carrying more than this, such as a large
// clipboard reply, is dropped rather than assembled.
//
// The check runs before the incoming bytes are added, so what is held can reach
// this cap plus one read before being dropped. uncurses reads into a fixed
// buffer and so cannot overshoot at all; the bound here is the cap plus the
// largest read, which is the same constant.
const maxBufferedBytes = defaultBufferCapacity

func (d *decoder) parse(data []byte) []Event {
	// Checked before appending, so a sequence is only abandoned once the
	// parser has already had a chance at a full buffer and made no progress.
	if len(d.buf) >= maxBufferedBytes {
		d.buf = d.buf[:0]
	}
	d.buf = append(d.buf, data...)
	var events []Event
	for {
		if len(d.pending) > 0 {
			events = append(events, d.pending[0])
			d.pending = d.pending[1:]
			continue
		}
		if len(d.buf) == 0 {
			break
		}
		if d.inPaste {
			n, ev := d.parsePasteChunk(d.buf)
			if n > 0 {
				d.buf = d.buf[n:]
			}
			if ev != nil {
				events = append(events, ev)
				continue
			}
			break
		}
		r := d.tryParse(d.buf)
		switch {
		case r.incomplete:
			if !d.expired {
				return events
			}
			b := d.buf[0]
			if ev := d.expireLeading(b); ev != nil {
				events = append(events, ev)
				d.buf = d.buf[1:]
				continue
			}
			events = append(events, UnknownEvent(append([]byte(nil), d.buf...)))
			d.buf = d.buf[:0]
		case r.none:
			if r.consumed > 0 {
				d.buf = d.buf[r.consumed:]
				continue
			}
			events = append(events, UnknownEvent([]byte{d.buf[0]}))
			d.buf = d.buf[1:]
		default:
			_, pasteStart := r.event.(PasteStartEvent)
			if group, ok := r.event.(MultiEvent); ok {
				events = append(events, group...)
			} else {
				events = append(events, r.event)
			}
			d.buf = d.buf[r.consumed:]
			if pasteStart {
				d.inPaste = true
			}
		}
	}
	return events
}

func (d *decoder) tryParse(buf []byte) parseResult {
	if len(buf) == 0 {
		return resultIncomplete()
	}
	var key Key
	switch b := buf[0]; b {
	case ansi.ESC:
		return d.parseEscape(buf)
	case '\b':
		key = NewKey('h', KeyModCtrl)
		if !d.flags.Contains(DecoderFlagCtrlH) {
			if d.backspace {
				key = NewKey(KeyBackspace, 0)
			} else if d.flags.Contains(DecoderFlagCtrlBackspace) {
				key = NewKey(KeyBackspace, KeyModCtrl)
			}
		}
	case '\t':
		key = NewKey(KeyTab, 0)
		if d.flags.Contains(DecoderFlagCtrlI) {
			key = NewKey('i', KeyModCtrl)
		}
	case '\n':
		key = NewKey('j', KeyModCtrl)
		if d.flags.Contains(DecoderFlagLFIsEnter) {
			key = NewKey(KeyEnter, 0)
		}
	case '\r':
		key = NewKey(KeyEnter, 0)
		if d.flags.Contains(DecoderFlagCtrlM) {
			key = NewKey('m', KeyModCtrl)
		}
	case 0:
		code := KeySpace
		if d.flags.Contains(DecoderFlagCtrlAt) {
			code = '@'
		}
		key = NewKey(code, KeyModCtrl)
	case 0x7f:
		code := KeyBackspace
		mod := KeyModNone
		if d.flags.Contains(DecoderFlagDELIsDelete) {
			code = KeyDelete
		} else if d.backspace {
			mod = KeyModCtrl
		}
		key = NewKey(code, mod)
	case 0x8f:
		return d.parseSS3(buf)
	case 0x90:
		return d.parseDCS(buf)
	case 0x98:
		return d.parseSOSPM(buf, 'X')
	case 0x9b:
		return d.parseCSI(buf)
	case 0x9d:
		return d.parseOSC(buf)
	case 0x9e:
		return d.parseSOSPM(buf, '^')
	case 0x9f:
		return d.parseAPC(buf)
	default:
		if b >= 0x01 && b <= 0x1a {
			key = NewKey(rune(b-1+'a'), KeyModCtrl)
		} else if b >= 0x1c && b <= 0x1f {
			key = NewKey(rune(b+0x40), KeyModCtrl)
		} else if b >= 0x80 && b <= 0x9f {
			key = NewKey(c1FallbackRune(b), KeyModCtrl|KeyModAlt)
		} else if b >= 0x80 {
			return d.parseUTF8(buf)
		} else {
			key = NewKey(rune(b), 0)
		}
	}
	return resultEvent(KeyPressEvent(key.Normalized()), 1)
}
