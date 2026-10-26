package event

func (d *decoder) parsePasteChunk(data []byte) (int, Event) {
	for scan := 0; scan < len(data); {
		if data[scan] != 0x1b {
			scan++
			continue
		}
		r := d.tryParse(data[scan:])
		if _, ok := r.event.(PasteEndEvent); ok {
			d.inPaste = false
			if scan == 0 {
				return r.consumed, PasteEndEvent{}
			}
			d.pending = append(d.pending, PasteEndEvent{})
			return scan + r.consumed, PasteChunkEvent(append([]byte(nil), data[:scan]...))
		}
		if r.incomplete {
			if scan == 0 {
				return 0, nil
			}
			return scan, PasteChunkEvent(append([]byte(nil), data[:scan]...))
		}
		step := r.consumed
		if step < 1 {
			step = 1
		}
		scan += step
	}
	if len(data) == 0 {
		return 0, nil
	}
	return len(data), PasteChunkEvent(append([]byte(nil), data...))
}
