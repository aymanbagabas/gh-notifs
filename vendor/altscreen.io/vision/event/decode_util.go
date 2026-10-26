package event

import (
	"encoding/hex"
	"strconv"
	"strings"

	"altscreen.io/vision/ansi"
)

func introPrefixLen(b byte) int {
	if b == ansi.ESC {
		return 2
	}
	return 1
}

func isC1Introducer(b byte) bool {
	return b == 0x8f || b == 0x90 || b == 0x98 || b == 0x9b || b == 0x9d || b == 0x9e || b == 0x9f
}

func findStringTerminator(data []byte) (int, int, bool) {
	for i := 0; i < len(data); i++ {
		if data[i] == 0x9c {
			return i, 1, true
		}
		if data[i] == ansi.ESC && i+1 < len(data) && data[i+1] == '\\' {
			return i, 2, true
		}
	}
	return 0, 0, false
}

func hexDecode(data []byte) ([]byte, bool) {
	if len(data)%2 != 0 {
		return nil, false
	}
	out := make([]byte, hex.DecodedLen(len(data)))
	if _, err := hex.Decode(out, data); err != nil {
		return nil, false
	}
	return out, true
}

func decodeTermcapPayload(data []byte) []TermcapEntry {
	var out []TermcapEntry
	for _, entry := range strings.Split(string(data), ";") {
		parts := strings.SplitN(entry, "=", 2)
		nameBytes, ok := hexDecode([]byte(parts[0]))
		if !ok || len(nameBytes) == 0 {
			continue
		}
		te := TermcapEntry{Name: string(nameBytes)}
		if len(parts) == 2 {
			v, ok := hexDecode([]byte(parts[1]))
			if !ok {
				continue
			}
			te.Value = string(v)
			te.HasValue = true
		}
		out = append(out, te)
	}
	return out
}

func parseKittyOptions(data []byte) []KeyValue {
	if len(data) == 0 {
		return nil
	}
	var out []KeyValue
	for _, kv := range strings.Split(string(data), ",") {
		if kv == "" {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if parts[0] == "" {
			continue
		}
		v := ""
		if len(parts) == 2 {
			v = parts[1]
		}
		out = append(out, KeyValue{Key: parts[0], Value: v})
	}
	return out
}

func csiModifiers(params ansi.Params) KeyMod { return xtermModifiers(params.GetOr(1, 1)) }
func csiKittyPhase(params ansi.Params) int {
	if g, ok := params.Group(1); ok {
		if v, ok := g.Nth(1); ok {
			return v
		}
	}
	return 1
}

func keyEventForPhase(key Key, phase int) Event {
	if phase == 2 {
		return KeyRepeatEvent(key)
	}
	if phase == 3 {
		return KeyReleaseEvent(key)
	}
	return KeyPressEvent(key)
}

func xtermModifiers(n int) KeyMod {
	if n > 0 {
		n--
	}
	var m KeyMod
	if n&1 != 0 {
		m |= KeyModShift
	}
	if n&2 != 0 {
		m |= KeyModAlt
	}
	if n&4 != 0 {
		m |= KeyModCtrl
	}
	if n&8 != 0 {
		m |= KeyModMeta
	}
	return m
}
func keyWithMods(code rune, mods KeyMod) Key { return NewKey(code, mods).Normalized() }

func lookupLegacyKey(seq []byte, flags DecoderFlags) (Key, bool) {
	if len(seq) == 3 && seq[0] == ansi.ESC && seq[1] == '[' {
		if code, ok := map[byte]rune{'a': KeyUp, 'b': KeyDown, 'c': KeyRight, 'd': KeyLeft}[seq[2]]; ok {
			return NewKey(code, KeyModShift).Normalized(), true
		}
	}
	if len(seq) == 3 && seq[0] == ansi.ESC && seq[1] == 'O' {
		if code, ok := map[byte]rune{'a': KeyUp, 'b': KeyDown, 'c': KeyRight, 'd': KeyLeft}[seq[2]]; ok {
			return NewKey(code, KeyModCtrl).Normalized(), true
		}
	}
	if len(seq) >= 4 && seq[0] == ansi.ESC && seq[1] == '[' {
		mods, ok := map[byte]KeyMod{'$': KeyModShift, '^': KeyModCtrl, '@': KeyModShift | KeyModCtrl}[seq[len(seq)-1]]
		if ok {
			if n, err := strconv.Atoi(string(seq[2 : len(seq)-1])); err == nil {
				if key, ok := keyFromTilde(n, mods, flags); ok {
					return key, true
				}
			}
		}
	}
	return Key{}, false
}

func tildeCodeToKeyCode(n int) (rune, bool) {
	switch n {
	case 1, 7:
		return KeyHome, true
	case 2:
		return KeyInsert, true
	case 3:
		return KeyDelete, true
	case 4, 8:
		return KeyEnd, true
	case 5:
		return KeyPageUp, true
	case 6:
		return KeyPageDown, true
	}
	if fn := tildeFunctionKey[n]; fn != 0 {
		return KeyF1 + rune(fn-1), true
	}
	return 0, false
}

func keyFromTilde(n int, mods KeyMod, flags DecoderFlags) (Key, bool) {
	if n >= 11 {
		if fn := tildeFunctionKey[n]; fn != 0 {
			return Key{Code: KeyF1 + rune(fn-1), Mod: mods}.Normalized(), true
		}
	}
	code, ok := tildeCodeToKeyCode(n)
	if !ok {
		return Key{}, false
	}
	return NewKey(remapTilde(n, code, flags), mods).Normalized(), true
}

func remapTilde(n int, code rune, flags DecoderFlags) rune {
	if n == 1 && code == KeyHome && flags.Contains(DecoderFlagFindKey) {
		return KeyFind
	}
	if n == 4 && code == KeyEnd && flags.Contains(DecoderFlagSelectKey) {
		return KeySelect
	}
	return code
}

// tildeFunctionKey maps a CSI tilde parameter to a 1 based function key
// number. The numbering is not contiguous: the VT220 layout skips 16, 22, 27,
// 30, and 35.
var tildeFunctionKey = map[int]int{
	11: 1, 12: 2, 13: 3, 14: 4, 15: 5,
	17: 6, 18: 7, 19: 8, 20: 9, 21: 10,
	23: 11, 24: 12, 25: 13, 26: 14,
	28: 15, 29: 16,
	31: 17, 32: 18, 33: 19, 34: 20,
}
