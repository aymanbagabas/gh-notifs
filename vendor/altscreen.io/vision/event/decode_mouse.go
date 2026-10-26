package event

import (
	"unicode/utf8"

	"altscreen.io/vision/ansi"
)

func decodeSGRMouse(params ansi.Params, release bool) Event {
	if params.Len() < 3 {
		return nil
	}
	return buildMouseEvent(params.GetOr(0, 0), max0(params.GetOr(1, 0)-1), max0(params.GetOr(2, 0)-1), &release)
}

func decodeX10Mouse(cb, cx, cy byte) Event {
	return buildMouseEvent(int(cb)-32, int(cx)-33, int(cy)-33, nil)
}

func decodeURXVTMouse(params ansi.Params) Event {
	if params.Len() < 3 {
		return nil
	}
	return buildMouseEvent(params.GetOr(0, 0)-32, max0(params.GetOr(1, 0)-1), max0(params.GetOr(2, 0)-1), nil)
}

func decodeUTF8Mouse(data []byte) (Event, int, bool) {
	idx := 0
	cb, n, ok := readMouseRune(data[idx:])
	if !ok {
		return nil, 0, false
	}
	idx += n
	cx, n, ok := readMouseRune(data[idx:])
	if !ok {
		return nil, 0, false
	}
	idx += n
	cy, n, ok := readMouseRune(data[idx:])
	if !ok {
		return nil, 0, false
	}
	idx += n
	return buildMouseEvent(cb-32, cx-33, cy-33, nil), idx, true
}

func readMouseRune(data []byte) (int, int, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	if data[0] < 0x80 {
		return int(data[0]), 1, true
	}
	r, n := utf8.DecodeRune(data)
	if r == utf8.RuneError && n == 1 {
		return 0, 0, false
	}
	return int(r), n, true
}

func buildMouseEvent(cb, cx, cy int, release *bool) Event {
	mods := mouseMods(cb)
	move, wheel, extra := cb&32 != 0, cb&64 != 0, cb&128 != 0
	button := MouseNone
	if wheel {
		button = []MouseButton{MouseWheelUp, MouseWheelDown, MouseWheelLeft, MouseWheelRight}[cb&3]
	} else if extra {
		button = []MouseButton{MouseButton8, MouseButton9, MouseButton10, MouseButton11}[cb&3]
	} else {
		button = []MouseButton{MouseLeft, MouseMiddle, MouseRight, MouseNone}[cb&3]
	}
	m := NewMouse(max0(cx), max0(cy), button, mods)
	isRelease := cb&3 == 3 && !wheel && !extra
	if release != nil {
		isRelease = *release
	}
	if wheel {
		return MouseWheelEvent(m)
	}
	if isRelease {
		return MouseReleaseEvent(m)
	}
	if move {
		return MouseMoveEvent(m)
	}
	return MouseClickEvent(m)
}

func mouseMods(cb int) KeyMod {
	var m KeyMod
	if cb&4 != 0 {
		m |= KeyModShift
	}
	if cb&8 != 0 {
		m |= KeyModAlt
	}
	if cb&16 != 0 {
		m |= KeyModCtrl
	}
	return m
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
