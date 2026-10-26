package event

// parseEscape decodes what follows an ESC byte.
//
//	ESC ─┬─ [ / O / ] / P / _ / X / ^ ──▶ sequence decoder
//	     ├─ printable byte ─────────────▶ alt+key
//	     ├─ C0 byte or DEL ─────────────▶ alt+<the key that byte is>
//	     └─ no byte yet ────────────────▶ incomplete until the deadline
//
// The C0 leg asks the bare mapping what the byte is and adds Alt rather than
// naming a key a second time. One mapping means ESC CR and a bare CR cannot
// disagree about which key they are, and every byte that mapping names gets an
// Alt spelling for free. ESC is itself a C0 byte, so a run of them takes the
// same leg.
func (d *decoder) parseEscape(buf []byte) parseResult {
	if len(buf) < 2 {
		if d.expired {
			return resultEvent(KeyPressEvent(d.loneEscape()), 1)
		}
		return resultIncomplete()
	}
	switch b := buf[1]; {
	case b == '[':
		return d.parseCSI(buf)
	case b == 'O':
		return d.parseSS3(buf)
	case b == ']':
		return d.parseOSC(buf)
	case b == 'P':
		return d.parseDCS(buf)
	case b == '_':
		return d.parseAPC(buf)
	case b == 'X':
		return d.parseSOSPM(buf, 'X')
	case b == '^':
		return d.parseSOSPM(buf, '^')
	case b >= 0x20 && b < 0x7f:
		return resultEvent(KeyPressEvent(NewKey(rune(b), KeyModAlt).Normalized()), 2)
	case b < 0x20 || b == 0x7f:
		// A C0 byte or DEL: the key that byte is, with Alt.
		//
		// The bare mapping is asked rather than a key named again here. Naming
		// them twice is how ESC CR came to disagree with a bare CR about which
		// key it was, and asking covers every byte that mapping names rather
		// than the few anyone thought to list. Whatever renames one of them
		// renames both spellings with it, so the flags that make 0x09 and 0x0d
		// into ctrl+i and ctrl+m, and 0x7f into delete, reach this spelling too.
		//
		// ESC is a C0 byte itself, so a run of them takes this leg as well: the
		// tail parses as its own sequence and comes back to be promoted. An
		// inner key already carrying Alt has nothing to gain from the outer ESC,
		// so that one stands alone instead, which is what makes the pair
		// "ESC ESC a" an Escape and then alt+a.
		//
		// Among these is ESC CR, the legacy spelling of alt+enter, which is what
		// a terminal with no Kitty keyboard support sends for it and what an
		// editor binds shift+enter to when it has nothing better. A reader
		// pressing Escape and then Enter is separated from it by the escape
		// deadline rather than by a read boundary: a lone ESC resolves once the
		// deadline passes, and the source emits it before reading again, so the
		// two meet here only while the deadline is still running. That is the
		// trade ESC followed by any printable byte already makes.
		r := d.tryParse(buf[1:])
		if k, ok := r.event.(KeyPressEvent); ok && !Key(k).Mod.Contains(KeyModAlt) {
			key := Key(k)
			key.Mod |= KeyModAlt
			key.Text = ""
			return resultEvent(KeyPressEvent(key.Normalized()), r.consumed+1)
		}
		if r.incomplete {
			return resultIncomplete()
		}
		return resultEvent(KeyPressEvent(d.loneEscape()), 1)
	default:
		return resultEvent(KeyPressEvent(d.loneEscape()), 1)
	}
}
