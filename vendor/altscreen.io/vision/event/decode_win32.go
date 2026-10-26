package event

import "altscreen.io/vision/ansi"

const (
	win32RightAlt   = 0x0001
	win32LeftAlt    = 0x0002
	win32RightCtrl  = 0x0004
	win32LeftCtrl   = 0x0008
	win32Shift      = 0x0010
	win32NumLock    = 0x0020
	win32ScrollLock = 0x0040
	win32CapsLock   = 0x0080
	win32Enhanced   = 0x0100
)

func (d *decoder) dispatchWin32Input(params ansi.Params, consumed int) parseResult {
	vk, ch, kd, cks, rep := params.GetOr(0, 0), params.GetOr(2, 0), params.GetOr(3, 0) != 0, params.GetOr(4, 0), params.GetOr(5, 1)
	mods := win32Mods(cks)
	if vk == 0 {
		return d.win32Text(ch, kd, cks, mods, rep, consumed)
	}
	base := win32VKToKeyCode(vk, cks, d.win32LastCKS)
	text := ""
	if vk >= 0x60 && vk <= 0x69 {
		text = string(rune('0' + vk - 0x60))
	}
	if c, ok := map[int]rune{0x6a: '*', 0x6b: '+', 0x6c: ',', 0x6d: '-', 0x6e: '.', 0x6f: '/'}[vk]; ok {
		text = string(c)
	}
	r := rune(ch)
	altGr := cks&(win32LeftCtrl|win32RightAlt) == win32LeftCtrl|win32RightAlt
	cksNoLocks := cks &^ (win32NumLock | win32ScrollLock)
	if r != 0 && !isControlRune(r) {
		if r != ' ' {
			base = rune(r)
		}
		printable := cksNoLocks == 0 || cksNoLocks == win32Shift || cksNoLocks == win32CapsLock || cksNoLocks == win32Shift|win32CapsLock || altGr
		if printable {
			text = string(r)
			if altGr {
				mods &^= KeyModCtrl | KeyModAlt
			}
		}
	}
	key := NewKey(base, mods)
	key.Text = text
	key.Normalize()
	d.win32LastCKS = cks
	return d.emitWin32Key(key, kd, rep, consumed)
}

func (d *decoder) win32Text(ch int, kd bool, cks int, mods KeyMod, rep int, consumed int) parseResult {
	idx := 0
	if kd {
		idx = 1
	}
	r := rune(ch)
	if ch >= 0xd800 && ch <= 0xdbff {
		d.win32HighSurrogate[idx] = r
		return resultNone(consumed)
	}
	if ch >= 0xdc00 && ch <= 0xdfff && d.win32HighSurrogate[idx] != 0 {
		hi := d.win32HighSurrogate[idx]
		d.win32HighSurrogate[idx] = 0
		r = 0x10000 + ((hi - 0xd800) << 10) + (r - 0xdc00)
	} else {
		d.win32HighSurrogate[idx] = 0
	}
	altGr := cks&(win32LeftCtrl|win32RightAlt) == win32LeftCtrl|win32RightAlt
	cksNoLocks := cks &^ (win32NumLock | win32ScrollLock)
	printable := !isControlRune(r) && (cksNoLocks == 0 || cksNoLocks == win32Shift || cksNoLocks == win32CapsLock || cksNoLocks == win32Shift|win32CapsLock || altGr)
	if printable && altGr {
		mods &^= KeyModCtrl | KeyModAlt
	}
	key := NewKey(r, mods)
	if printable {
		key.Text = string(r)
	}
	key.Normalize()
	return d.emitWin32Key(key, kd, rep, consumed)
}

func (d *decoder) emitWin32Key(key Key, kd bool, rep int, consumed int) parseResult {
	mk := func() Event {
		if kd {
			return KeyPressEvent(key)
		}
		return KeyReleaseEvent(key)
	}
	first := mk()
	for i := 1; i < rep; i++ {
		d.pending = append(d.pending, mk())
	}
	return resultEvent(first, consumed)
}

func win32Mods(cks int) KeyMod {
	var m KeyMod
	if cks&(win32LeftCtrl|win32RightCtrl) != 0 {
		m |= KeyModCtrl
	}
	if cks&(win32LeftAlt|win32RightAlt) != 0 {
		m |= KeyModAlt
	}
	if cks&win32Shift != 0 {
		m |= KeyModShift
	}
	if cks&win32CapsLock != 0 {
		m |= KeyModCapsLock
	}
	if cks&win32NumLock != 0 {
		m |= KeyModNumLock
	}
	if cks&win32ScrollLock != 0 {
		m |= KeyModScrollLock
	}
	return m
}
func isControlRune(r rune) bool { return r < 0x20 || r == 0x7f }

func win32VKToKeyCode(vk, cks, last int) rune {
	switch vk {
	case 0x08:
		return KeyBackspace
	case 0x09:
		return KeyTab
	case 0x0d:
		return KeyEnter
	case 0x10:
		if cks&win32Shift != 0 {
			if cks&win32Enhanced != 0 {
				return KeyRightShift
			}
			return KeyLeftShift
		}
		if last&win32Enhanced != 0 {
			return KeyRightShift
		}
		return KeyLeftShift
	case 0x11:
		if cks&win32RightCtrl != 0 || last&win32RightCtrl != 0 {
			return KeyRightCtrl
		}
		return KeyLeftCtrl
	case 0x12:
		if cks&win32RightAlt != 0 || last&win32RightAlt != 0 {
			return KeyRightAlt
		}
		return KeyLeftAlt
	case 0x13:
		return KeyPause
	case 0x14:
		return KeyCapsLock
	case 0x1b:
		return KeyEscape
	case 0x20:
		return KeySpace
	case 0x21:
		return KeyPageUp
	case 0x22:
		return KeyPageDown
	case 0x23:
		return KeyEnd
	case 0x24:
		return KeyHome
	case 0x25:
		return KeyLeft
	case 0x26:
		return KeyUp
	case 0x27:
		return KeyRight
	case 0x28:
		return KeyDown
	case 0x2c:
		return KeyPrintScreen
	case 0x2d:
		return KeyInsert
	case 0x2e:
		return KeyDelete
	case 0x5b:
		return KeyLeftSuper
	case 0x5c:
		return KeyRightSuper
	case 0x5d:
		return KeyMenu
	case 0x90:
		return KeyNumLock
	case 0x91:
		return KeyScrollLock
	case 0xa0:
		return KeyLeftShift
	case 0xa1:
		return KeyRightShift
	case 0xa2:
		return KeyLeftCtrl
	case 0xa3:
		return KeyRightCtrl
	case 0xa4:
		return KeyLeftAlt
	case 0xa5:
		return KeyRightAlt
	case 0xad:
		return KeyVolumeMute
	case 0xae:
		return KeyVolumeDown
	case 0xaf:
		return KeyVolumeUp
	case 0xb0:
		return KeyMediaNext
	case 0xb1:
		return KeyMediaPrev
	case 0xb2:
		return KeyMediaStop
	case 0xb3:
		return KeyMediaPlayPause
	case 0x6a:
		return KeyKpMultiply
	case 0x6b:
		return KeyKpAdd
	case 0x6c:
		return KeyKpSeparator
	case 0x6d:
		return KeyKpSubtract
	case 0x6e:
		return KeyKpDecimal
	case 0x6f:
		return KeyKpDivide
	}
	// VK_F1 through VK_F24 are contiguous, and so are the function key
	// codes, so the range maps across directly.
	if vk >= 0x70 && vk <= 0x87 {
		return KeyF1 + rune(vk-0x70)
	}
	if vk >= 0x60 && vk <= 0x69 {
		return rune(int(KeyKp0) + vk - 0x60)
	}
	if vk >= '0' && vk <= '9' {
		return rune(vk)
	}
	if vk >= 'A' && vk <= 'Z' {
		return rune(vk + 32)
	}
	if c, ok := map[int]rune{0xba: ';', 0xbb: '+', 0xbc: ',', 0xbd: '-', 0xbe: '.', 0xbf: '/', 0xc0: '`', 0xdb: '[', 0xdc: '\\', 0xdd: ']', 0xde: '\''}[vk]; ok {
		return rune(c)
	}
	return 0
}
