package event

func (d *decoder) parseAPC(buf []byte) parseResult {
	p := introPrefixLen(buf[0])
	end, st, ok := findStringTerminator(buf[p:])
	if !ok {
		return resultIncomplete()
	}
	payload := buf[p : p+end]
	if len(payload) > 0 && payload[0] == 'G' {
		rest := payload[1:]
		opts, body := rest, []byte{}
		if i := indexByte(rest, ';'); i >= 0 {
			opts, body = rest[:i], rest[i+1:]
		}
		return resultEvent(KittyGraphicsEvent{Options: parseKittyOptions(opts), Payload: append([]byte(nil), body...)}, p+end+st)
	}
	return resultEvent(UnknownApcEvent(append([]byte(nil), payload...)), p+end+st)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
