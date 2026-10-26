package event

import (
	"unicode/utf8"

	"altscreen.io/vision/ansi"
)

func decodeKittyKey(params ansi.Params) Event {
	codeGroup, ok := params.Group(0)
	if !ok || codeGroup.IsEmpty() {
		return nil
	}
	code, ok := codeGroup.First()
	if !ok {
		return nil
	}
	modGroup, ok := params.Group(1)
	if !ok {
		modGroup = ansi.EmptyGroup
	}
	textGroup, ok := params.Group(2)
	if !ok {
		textGroup = ansi.EmptyGroup
	}
	mods := decodeKittyModifiers(1)
	if v, ok := modGroup.First(); ok {
		mods = decodeKittyModifiers(v)
	}
	key, ok := kittyCodeToKey(code, mods)
	if !ok {
		return nil
	}
	if v, ok := codeGroup.Nth(1); ok && v != 0 {
		if r := rune(v); utf8.ValidRune(r) {
			key.ShiftedKey = r
		}
	}
	if v, ok := codeGroup.Nth(2); ok && v != 0 {
		if r := rune(v); utf8.ValidRune(r) {
			key.BaseKey = r
		}
	}
	if !textGroup.IsEmpty() {
		var rs []rune
		for _, p := range textGroup.Values() {
			if p.Ok {
				r := rune(p.Value)
				if utf8.ValidRune(r) {
					rs = append(rs, r)
				}
			}
		}
		if len(rs) > 0 {
			key.Text = string(rs)
		}
	}
	key.Normalize()
	return keyEventForPhase(key, csiKittyPhase(params))
}

func decodeKittyModifiers(m int) KeyMod {
	m--
	var mods KeyMod
	if m&1 != 0 {
		mods |= KeyModShift
	}
	if m&2 != 0 {
		mods |= KeyModAlt
	}
	if m&4 != 0 {
		mods |= KeyModCtrl
	}
	if m&8 != 0 {
		mods |= KeyModSuper
	}
	if m&16 != 0 {
		mods |= KeyModHyper
	}
	if m&32 != 0 {
		mods |= KeyModMeta
	}
	if m&64 != 0 {
		mods |= KeyModCapsLock
	}
	if m&128 != 0 {
		mods |= KeyModNumLock
	}
	return mods
}

func kittyCodeToKey(code int, mods KeyMod) (Key, bool) {
	switch code {
	case 9:
		return NewKey(KeyTab, mods), true
	case 13:
		return NewKey(KeyEnter, mods), true
	case 27:
		return NewKey(KeyEscape, mods), true
	case 32:
		return NewKey(KeySpace, mods), true
	case 127:
		return NewKey(KeyBackspace, mods), true
	}
	if code >= 57364 && code <= 57398 {
		return Key{Code: KeyF1 + rune(code-57364), Mod: mods}, true
	}
	if k, ok := map[int]rune{57344: KeyEscape, 57345: KeyEnter, 57346: KeyTab, 57347: KeyBackspace, 57348: KeyInsert, 57349: KeyDelete, 57350: KeyLeft, 57351: KeyRight, 57352: KeyUp, 57353: KeyDown, 57354: KeyPageUp, 57355: KeyPageDown, 57356: KeyHome, 57357: KeyEnd, 57358: KeyCapsLock, 57359: KeyScrollLock, 57360: KeyNumLock, 57361: KeyPrintScreen, 57362: KeyPause, 57363: KeyMenu, 57399: KeyKp0, 57400: KeyKp1, 57401: KeyKp2, 57402: KeyKp3, 57403: KeyKp4, 57404: KeyKp5, 57405: KeyKp6, 57406: KeyKp7, 57407: KeyKp8, 57408: KeyKp9, 57409: KeyKpDecimal, 57410: KeyKpDivide, 57411: KeyKpMultiply, 57412: KeyKpSubtract, 57413: KeyKpAdd, 57414: KeyKpEnter, 57415: KeyKpEqual, 57416: KeyKpSeparator, 57417: KeyKpLeft, 57418: KeyKpRight, 57419: KeyKpUp, 57420: KeyKpDown, 57421: KeyKpPageUp, 57422: KeyKpPageDown, 57423: KeyKpHome, 57424: KeyKpEnd, 57425: KeyKpInsert, 57426: KeyKpDelete, 57427: KeyKpBegin, 57428: KeyMediaPlay, 57429: KeyMediaPause, 57430: KeyMediaPlayPause, 57431: KeyMediaReverse, 57432: KeyMediaStop, 57433: KeyMediaFastForward, 57434: KeyMediaRewind, 57435: KeyMediaNext, 57436: KeyMediaPrev, 57437: KeyMediaRecord, 57438: KeyVolumeDown, 57439: KeyVolumeUp, 57440: KeyVolumeMute, 57441: KeyLeftShift, 57442: KeyLeftCtrl, 57443: KeyLeftAlt, 57444: KeyLeftSuper, 57445: KeyLeftHyper, 57446: KeyLeftMeta, 57447: KeyRightShift, 57448: KeyRightCtrl, 57449: KeyRightAlt, 57450: KeyRightSuper, 57451: KeyRightHyper, 57452: KeyRightMeta, 57453: KeyIsoLevel3Shift, 57454: KeyIsoLevel5Shift}[code]; ok {
		return NewKey(k, mods), true
	}
	r := rune(code)
	if utf8.ValidRune(r) {
		return NewKey(r, mods), true
	}
	return Key{}, false
}
