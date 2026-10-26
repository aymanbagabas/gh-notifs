package event

func (d *decoder) parseSS3(buf []byte) parseResult {
	p := introPrefixLen(buf[0])
	if len(buf) < p+1 {
		return resultIncomplete()
	}
	consumed := p + 1
	raw := buf[:consumed]
	if ev := recognizeSS3(buf[p], raw, d.flags); ev != nil {
		return resultEvent(ev, consumed)
	}
	return resultEvent(UnknownSs3Event(append([]byte(nil), raw...)), consumed)
}

func recognizeSS3(final byte, raw []byte, flags DecoderFlags) Event {
	if code, ok := map[byte]rune{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'H': KeyHome, 'F': KeyEnd, 'M': KeyKpEnter, 'j': KeyKpMultiply, 'k': KeyKpAdd, 'm': KeyKpSubtract, 'n': KeyKpDecimal, 'o': KeyKpDivide, 'p': KeyKp0, 'q': KeyKp1, 'r': KeyKp2, 's': KeyKp3, 't': KeyKp4, 'u': KeyKp5, 'v': KeyKp6, 'w': KeyKp7, 'x': KeyKp8, 'y': KeyKp9}[final]; ok {
		return KeyPressEvent(NewKey(code, 0).Normalized())
	}
	if final >= 'P' && final <= 'S' {
		return KeyPressEvent(Key{Code: KeyF1 + rune(final-'P')}.Normalized())
	}
	if k, ok := lookupLegacyKey(raw, flags); ok {
		return KeyPressEvent(k)
	}
	return nil
}
