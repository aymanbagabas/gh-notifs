package event

import (
	"altscreen.io/vision/ansi"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/term"
)

func (d *decoder) parseCSI(buf []byte) parseResult {
	p := introPrefixLen(buf[0])
	if len(buf) < p+1 {
		return resultIncomplete()
	}
	i := p
	hasPrivate := false
	if i < len(buf) && (buf[i] == '?' || buf[i] == '<' || buf[i] == '>' || buf[i] == '=') {
		hasPrivate = true
		i++
	}
	for i < len(buf) {
		b := buf[i]
		if b >= 0x40 && b <= 0x7e {
			return d.dispatchCSI(buf, buf[p:i+1], hasPrivate, i+1)
		}
		if !((b >= 0x30 && b <= 0x3f) || b == ';' || b == ':' || (b >= 0x20 && b <= 0x2f)) {
			return resultNone(i + 1)
		}
		i++
	}
	// The grammar ran out of bytes without reaching a final one. URxvt ends
	// some keys with an intermediate instead of a final byte, so a body that
	// stops there may be one of those rather than a sequence still arriving.
	//
	// The rule is that such a key is named only when nothing else can complete
	// the body. It is a prefix of every sequence that could put a final byte
	// after the same intermediate, and those two readings cannot both be right,
	// so the grammar takes precedence and the table answers last. A byte
	// arriving behind the key therefore makes an ordinary sequence of it, and
	// the key is not recovered.
	//
	// The table is also what knows which of these exist, so nothing here has to
	// guess at their shape, and a body it does not name is simply incomplete.
	if k, ok := lookupLegacyKey(buf, d.flags); ok {
		return resultEvent(KeyPressEvent(k), len(buf))
	}
	return resultIncomplete()
}

func (d *decoder) dispatchCSI(buf, seq []byte, hasPrivate bool, consumed int) parseResult {
	view, _, ok := ansi.ParseControlSequence(seq)
	if !ok {
		// Not a sequence ECMA-48 would name. URxvt terminates some keys with
		// `$`, which is an intermediate byte, so the legacy table still gets a
		// look before this is called unknown.
		raw := buf[:consumed]
		if k, found := lookupLegacyKey(raw, d.flags); found {
			return resultEvent(KeyPressEvent(k), consumed)
		}
		return resultEvent(UnknownCsiEvent(append([]byte(nil), raw...)), consumed)
	}
	// These two dispatch here rather than in recognizeCSI, because each needs
	// something it does not have: the mouse report reads its coordinates from
	// the bytes after the sequence, and the win32 record mutates pending state.
	// They repeat the region checks recognizeCSI would otherwise apply, and an
	// intermediate byte selects a different control function, so a sequence
	// carrying one is not the function being matched here.
	noInter := view.Intermediate() == 0
	if !hasPrivate && noInter && view.Final() == 'M' && len(view.Params().Raw()) == 0 {
		if d.utf8Mouse {
			if ev, n, ok := decodeUTF8Mouse(buf[consumed:]); ok {
				return resultEvent(ev, consumed+n)
			}
			return resultIncomplete()
		}
		if len(buf) >= consumed+3 {
			return resultEvent(decodeX10Mouse(buf[consumed], buf[consumed+1], buf[consumed+2]), consumed+3)
		}
		return resultIncomplete()
	}
	if view.Final() == '_' && !hasPrivate && noInter && countByte(view.Params().Raw(), ';') == 5 {
		return d.dispatchWin32Input(view.Params(), consumed)
	}
	if ev := recognizeCSI(view, buf[:consumed], d.flags); ev != nil {
		return resultEvent(ev, consumed)
	}
	return resultEvent(UnknownCsiEvent(append([]byte(nil), buf[:consumed]...)), consumed)
}

func recognizeCSI(v ansi.ControlSequence, raw []byte, flags DecoderFlags) Event {
	params := v.Params()
	noPriv, noInter := v.Private() == 0, v.Intermediate() == 0
	if noPriv && noInter && v.Final() == 'M' && params.Len() == 3 {
		if ev := decodeURXVTMouse(params); ev != nil {
			return ev
		}
	}
	if v.Private() == '<' && noInter && (v.Final() == 'M' || v.Final() == 'm') && params.Len() == 3 {
		return decodeSGRMouse(params, v.Final() == 'm')
	}
	if noPriv && noInter && v.Final() == 'u' {
		if ev := decodeKittyKey(params); ev != nil {
			return ev
		}
	}
	if noPriv && noInter && params.IsEmpty() {
		if v.Final() == 'I' {
			return FocusInEvent{}
		}
		if v.Final() == 'O' {
			return FocusOutEvent{}
		}
	}
	if noInter && v.Final() == 'R' && params.Len() >= 2 {
		row, col := max1(params.GetOr(0, 1)), max1(params.GetOr(1, 1))
		cpr := CursorPositionEvent{Position: layout.Pos(col-1, row-1)}
		if v.Private() == '?' {
			cpr.Page = params.GetOr(2, 0)
			return cpr
		}
		if noPriv && params.Len() == 2 {
			if row == 1 && col-1 <= 15 {
				return MultiEvent{KeyPressEvent(Key{Code: KeyF3, Mod: xtermModifiers(col)}.Normalized()), cpr}
			}
			return cpr
		}
	}
	if v.Private() == '?' && noInter && v.Final() == 'n' && params.Len() == 2 && params.GetOr(0, 0) == 997 {
		if params.GetOr(1, 0) == 1 {
			return ColorSchemeEvent(ColorSchemeDark)
		}
		if params.GetOr(1, 0) == 2 {
			return ColorSchemeEvent(ColorSchemeLight)
		}
	}
	if v.Private() == '?' && noInter && v.Final() == 'n' && params.Len() == 2 && params.GetOr(0, 0) == 999 {
		if params.GetOr(1, 0) == 1 {
			return VisibilityEvent(VisibilityVisible)
		}
		if params.GetOr(1, 0) == 2 {
			return VisibilityEvent(VisibilityHidden)
		}
	}
	if v.Private() == '?' && noInter && v.Final() == 'c' {
		return PrimaryDeviceAttributesEvent(optionalInts(params))
	}
	if v.Private() == '>' && noInter && v.Final() == 'c' {
		return SecondaryDeviceAttributesEvent(optionalInts(params))
	}
	if v.Final() == 'y' && v.Intermediate() == '$' && params.Len() == 2 {
		mode := ansi.AnsiMode(params.GetOr(0, 0))
		if !noPriv {
			mode = ansi.DecMode(params.GetOr(0, 0))
		}
		return ModeReportEvent{Mode: mode, Setting: ansi.ModeSettingFromValue(params.GetOr(1, 0))}
	}
	if v.Private() == '?' && noInter && v.Final() == 'u' && params.Len() == 1 {
		return KittyKeyboardEnhancementsEvent(ansi.KittyKeyboardFlags(params.GetOr(0, 0)))
	}
	// The discriminators below each need their own parameters, and they check
	// for them. The report itself needs only one: CSI 1 t and CSI 2 t are how
	// xterm answers CSI 11 t with the window state.
	if noPriv && noInter && v.Final() == 't' && !params.IsEmpty() {
		switch params.GetOr(0, 0) {
		case 4:
			if params.Len() == 3 {
				return WindowPixelSizeEvent{Width: params.GetOr(2, 0), Height: params.GetOr(1, 0)}
			}
		case 6:
			if params.Len() == 3 {
				return CellPixelSizeEvent{Width: params.GetOr(2, 0), Height: params.GetOr(1, 0)}
			}
		case 8:
			if params.Len() == 3 {
				return WindowCellSizeEvent{Width: params.GetOr(2, 0), Height: params.GetOr(1, 0)}
			}
		case 48:
			// The rows and columns are what make this a resize, so a reply
			// carrying fewer than three parameters is not one. Reading it as
			// a resize turns a truncated report into a zero by zero geometry,
			// which every sibling here already refuses by checking its own
			// arity.
			if params.Len() >= 3 {
				return ResizeEvent(term.Winsize{Row: params.GetOr(1, 0), Col: params.GetOr(2, 0), YPixel: params.GetOr(3, 0), XPixel: params.GetOr(4, 0)})
			}
		}
		return WindowOpEvent{Op: params.GetOr(0, 0), Args: optionalInts(params.SliceFrom(1))}
	}
	if v.Private() == '>' && noInter && v.Final() == 'm' && params.Len() == 2 && params.GetOr(0, 0) == 4 {
		return ModifyOtherKeysEvent(ModifyOtherKeysModeFromValue(params.GetOr(1, 0)))
	}
	if noPriv && noInter {
		if ev := recognizeCSIKey(v.Final(), params, flags); ev != nil {
			return ev
		}
	}
	if k, ok := lookupLegacyKey(raw, flags); ok {
		return KeyPressEvent(k)
	}
	return nil
}

func recognizeCSIKey(final byte, params ansi.Params, flags DecoderFlags) Event {
	if final == '~' {
		return recognizeTilde(params, flags)
	}
	if params.Len() > 2 {
		return nil
	}
	if code, ok := map[byte]rune{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'E': KeyKpBegin, 'H': KeyHome, 'F': KeyEnd}[final]; ok {
		return keyEventForPhase(keyWithMods(code, csiModifiers(params)), csiKittyPhase(params))
	}
	if final >= 'P' && final <= 'S' {
		return keyEventForPhase(Key{Code: map[byte]rune{'P': KeyF1, 'Q': KeyF2, 'R': KeyF3, 'S': KeyF4}[final], Mod: csiModifiers(params)}.Normalized(), csiKittyPhase(params))
	}
	if final == 'Z' {
		return keyEventForPhase(NewKey(KeyTab, KeyModShift).Normalized(), csiKittyPhase(params))
	}
	return nil
}

func recognizeTilde(params ansi.Params, flags DecoderFlags) Event {
	code := params.GetOr(0, 0)
	n := params.Len()
	if code == 27 && n == 3 {
		mods := xtermModifiers(params.GetOr(1, 1))
		r := rune(params.GetOr(2, 0))
		key := NewKey(r, mods)
		switch r {
		case 8:
			key = NewKey(KeyBackspace, mods)
		case 9:
			key = NewKey(KeyTab, mods)
		case 13:
			key = NewKey(KeyEnter, mods)
		case 27:
			key = NewKey(KeyEscape, mods)
		case 32:
			key = NewKey(KeySpace, mods)
		case 127:
			key = NewKey(KeyBackspace, mods)
		}
		return KeyPressEvent(key.Normalized())
	}
	if n > 2 {
		return nil
	}
	if code == 200 && n == 1 {
		return PasteStartEvent{}
	}
	if code == 201 && n == 1 {
		return PasteEndEvent{}
	}
	if key, ok := keyFromTilde(code, csiModifiers(params), flags); ok {
		return keyEventForPhase(key, csiKittyPhase(params))
	}
	return nil
}

func optionalInts(params ansi.Params) []OptionalInt {
	out := make([]OptionalInt, 0, params.Len())
	for _, g := range params.Groups() {
		v, ok := g.First()
		out = append(out, OptionalInt{Value: v, Ok: ok})
	}
	return out
}

func countByte(b []byte, c byte) int {
	n := 0
	for _, x := range b {
		if x == c {
			n++
		}
	}
	return n
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
