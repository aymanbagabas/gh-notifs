package event

func (d *decoder) parseSOSPM(buf []byte, kind byte) parseResult {
	p := introPrefixLen(buf[0])
	end, st, ok := findStringTerminator(buf[p:])
	if !ok {
		return resultIncomplete()
	}
	payload := append([]byte(nil), buf[p:p+end]...)
	if kind == 'X' {
		return resultEvent(UnknownSosEvent(payload), p+end+st)
	}
	return resultEvent(UnknownPmEvent(payload), p+end+st)
}
