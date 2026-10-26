// Package event defines terminal event value types.
package event

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"altscreen.io/vision/ansi"
)

// KeyMod is a set of keyboard modifier flags.
type KeyMod uint16

const (
	// KeyModShift is the Shift key modifier.
	KeyModShift KeyMod = 1 << iota
	// KeyModAlt is the Alt key modifier.
	KeyModAlt
	// KeyModCtrl is the Control key modifier.
	KeyModCtrl
	// KeyModMeta is the Meta key modifier.
	KeyModMeta
	// KeyModHyper is the Hyper key modifier.
	KeyModHyper
	// KeyModSuper is the Super key modifier.
	KeyModSuper
	// KeyModCapsLock is the Caps Lock state.
	KeyModCapsLock
	// KeyModNumLock is the Num Lock state.
	KeyModNumLock
	// KeyModScrollLock is the Scroll Lock state.
	KeyModScrollLock
)

const (
	// KeyModNone means no key modifiers.
	KeyModNone KeyMod = 0
	// KeyModLockMask is the mask of lock-state bits.
	KeyModLockMask = KeyModCapsLock | KeyModNumLock | KeyModScrollLock
)

// Contains reports whether all flags in g are set.
func (m KeyMod) Contains(g KeyMod) bool { return m&g == g }

// Set returns m with g set.
func (m KeyMod) Set(g KeyMod) KeyMod { return m | g }

// Clear returns m with g cleared.
func (m KeyMod) Clear(g KeyMod) KeyMod { return m &^ g }

// KeyExtended is where the keys that are not characters begin. A key code at or
// below [unicode.MaxRune] is the character itself, so a printable key needs no
// separate field to carry its rune.
const KeyExtended rune = unicode.MaxRune + 1

// Keys that are characters keep the codepoint they send, so a caller can
// compare against a literal.
const (
	// KeyBackspace is the backspace key, which sends DEL.
	KeyBackspace rune = ansi.DEL
	// KeyTab is the tab key.
	KeyTab rune = ansi.HT
	// KeyEnter is the return key.
	KeyEnter rune = ansi.CR
	// KeyEscape is the escape key.
	KeyEscape rune = ansi.ESC
	// KeySpace is the space bar.
	KeySpace rune = ansi.SP
)

const (
	// KeyF1 is the F1 function key.
	KeyF1 rune = KeyExtended + 1 + iota
	// KeyF2 is the F2 function key.
	KeyF2
	// KeyF3 is the F3 function key.
	KeyF3
	// KeyF4 is the F4 function key.
	KeyF4
	// KeyF5 is the F5 function key.
	KeyF5
	// KeyF6 is the F6 function key.
	KeyF6
	// KeyF7 is the F7 function key.
	KeyF7
	// KeyF8 is the F8 function key.
	KeyF8
	// KeyF9 is the F9 function key.
	KeyF9
	// KeyF10 is the F10 function key.
	KeyF10
	// KeyF11 is the F11 function key.
	KeyF11
	// KeyF12 is the F12 function key.
	KeyF12
	// KeyF13 is the F13 function key.
	KeyF13
	// KeyF14 is the F14 function key.
	KeyF14
	// KeyF15 is the F15 function key.
	KeyF15
	// KeyF16 is the F16 function key.
	KeyF16
	// KeyF17 is the F17 function key.
	KeyF17
	// KeyF18 is the F18 function key.
	KeyF18
	// KeyF19 is the F19 function key.
	KeyF19
	// KeyF20 is the F20 function key.
	KeyF20
	// KeyF21 is the F21 function key.
	KeyF21
	// KeyF22 is the F22 function key.
	KeyF22
	// KeyF23 is the F23 function key.
	KeyF23
	// KeyF24 is the F24 function key.
	KeyF24
	// KeyF25 is the F25 function key.
	KeyF25
	// KeyF26 is the F26 function key.
	KeyF26
	// KeyF27 is the F27 function key.
	KeyF27
	// KeyF28 is the F28 function key.
	KeyF28
	// KeyF29 is the F29 function key.
	KeyF29
	// KeyF30 is the F30 function key.
	KeyF30
	// KeyF31 is the F31 function key.
	KeyF31
	// KeyF32 is the F32 function key.
	KeyF32
	// KeyF33 is the F33 function key.
	KeyF33
	// KeyF34 is the F34 function key.
	KeyF34
	// KeyF35 is the F35 function key.
	KeyF35
	// KeyFind is the legacy VT220 Find key.
	KeyFind
	// KeySelect is the legacy VT220 Select key.
	KeySelect
	// KeyUp is the up arrow key.
	KeyUp
	// KeyDown is the down arrow key.
	KeyDown
	// KeyLeft is the left arrow key.
	KeyLeft
	// KeyRight is the right arrow key.
	KeyRight
	// KeyHome is the Home key.
	KeyHome
	// KeyEnd is the End key.
	KeyEnd
	// KeyPageUp is the Page Up key.
	KeyPageUp
	// KeyPageDown is the Page Down key.
	KeyPageDown
	// KeyDelete is the Delete key.
	KeyDelete
	// KeyInsert is the Insert key.
	KeyInsert
	// KeyCapsLock is the Caps Lock key.
	KeyCapsLock
	// KeyScrollLock is the Scroll Lock key.
	KeyScrollLock
	// KeyNumLock is the Num Lock key.
	KeyNumLock
	// KeyPrintScreen is the Print Screen key.
	KeyPrintScreen
	// KeyPause is the Pause key.
	KeyPause
	// KeyMenu is the Menu key.
	KeyMenu
	// KeyKpEnter is the keypad Enter key.
	KeyKpEnter
	// KeyKpAdd is the keypad Add key.
	KeyKpAdd
	// KeyKpSubtract is the keypad Subtract key.
	KeyKpSubtract
	// KeyKpMultiply is the keypad Multiply key.
	KeyKpMultiply
	// KeyKpDivide is the keypad Divide key.
	KeyKpDivide
	// KeyKpDecimal is the keypad Decimal key.
	KeyKpDecimal
	// KeyKpEqual is the keypad Equal key.
	KeyKpEqual
	// KeyKpSeparator is the keypad Separator key.
	KeyKpSeparator
	// KeyKpLeft is the keypad Left key.
	KeyKpLeft
	// KeyKpRight is the keypad Right key.
	KeyKpRight
	// KeyKpUp is the keypad Up key.
	KeyKpUp
	// KeyKpDown is the keypad Down key.
	KeyKpDown
	// KeyKpPageUp is the keypad Page Up key.
	KeyKpPageUp
	// KeyKpPageDown is the keypad Page Down key.
	KeyKpPageDown
	// KeyKpHome is the keypad Home key.
	KeyKpHome
	// KeyKpEnd is the keypad End key.
	KeyKpEnd
	// KeyKpInsert is the keypad Insert key.
	KeyKpInsert
	// KeyKpDelete is the keypad Delete key.
	KeyKpDelete
	// KeyKpBegin is the keypad Begin key.
	KeyKpBegin
	// KeyKp0 is the keypad 0 key.
	KeyKp0
	// KeyKp1 is the keypad 1 key.
	KeyKp1
	// KeyKp2 is the keypad 2 key.
	KeyKp2
	// KeyKp3 is the keypad 3 key.
	KeyKp3
	// KeyKp4 is the keypad 4 key.
	KeyKp4
	// KeyKp5 is the keypad 5 key.
	KeyKp5
	// KeyKp6 is the keypad 6 key.
	KeyKp6
	// KeyKp7 is the keypad 7 key.
	KeyKp7
	// KeyKp8 is the keypad 8 key.
	KeyKp8
	// KeyKp9 is the keypad 9 key.
	KeyKp9
	// KeyMediaPlay is the Media Play key.
	KeyMediaPlay
	// KeyMediaPause is the Media Pause key.
	KeyMediaPause
	// KeyMediaPlayPause is the Media Play/Pause key.
	KeyMediaPlayPause
	// KeyMediaReverse is the Media Reverse key.
	KeyMediaReverse
	// KeyMediaStop is the Media Stop key.
	KeyMediaStop
	// KeyMediaRewind is the Media Rewind key.
	KeyMediaRewind
	// KeyMediaFastForward is the Media Fast Forward key.
	KeyMediaFastForward
	// KeyMediaNext is the Media Next key.
	KeyMediaNext
	// KeyMediaPrev is the Media Previous key.
	KeyMediaPrev
	// KeyMediaRecord is the Media Record key.
	KeyMediaRecord
	// KeyVolumeUp is the Volume Up key.
	KeyVolumeUp
	// KeyVolumeDown is the Volume Down key.
	KeyVolumeDown
	// KeyVolumeMute is the Volume Mute key.
	KeyVolumeMute
	// KeyLeftShift is the Left Shift key.
	KeyLeftShift
	// KeyRightShift is the Right Shift key.
	KeyRightShift
	// KeyLeftCtrl is the Left Control key.
	KeyLeftCtrl
	// KeyRightCtrl is the Right Control key.
	KeyRightCtrl
	// KeyLeftAlt is the Left Alt key.
	KeyLeftAlt
	// KeyRightAlt is the Right Alt key.
	KeyRightAlt
	// KeyLeftSuper is the Left Super key.
	KeyLeftSuper
	// KeyRightSuper is the Right Super key.
	KeyRightSuper
	// KeyLeftHyper is the Left Hyper key.
	KeyLeftHyper
	// KeyRightHyper is the Right Hyper key.
	KeyRightHyper
	// KeyLeftMeta is the Left Meta key.
	KeyLeftMeta
	// KeyRightMeta is the Right Meta key.
	KeyRightMeta
	// KeyIsoLevel3Shift is the ISO Level 3 Shift key.
	KeyIsoLevel3Shift
	// KeyIsoLevel5Shift is the ISO Level 5 Shift key.
	KeyIsoLevel5Shift
)

// Key is a key event payload.
//
// Equal has the binding semantics from the Rust type. The Go == operator compares every field. Use BindingKey when storing keys in maps by binding identity.
type Key struct {
	// Code is the key. A code at or below [unicode.MaxRune] is the character
	// itself; anything above is one of the constants from [KeyExtended] up.
	Code rune
	// Mod is the set of active keyboard modifiers.
	Mod KeyMod
	// Text is associated text. Empty means absent.
	Text string
	// ShiftedKey is the shifted-layer codepoint, or zero when absent.
	ShiftedKey rune
	// BaseKey is the base-layout codepoint, or zero when absent.
	BaseKey rune
}

// NewKey constructs a Key with the given code and modifiers.
func NewKey(code rune, modifiers KeyMod) Key { return Key{Code: code, Mod: modifiers} }

// Normalized returns the canonical form of k.
func (k Key) Normalized() Key { k.Normalize(); return k }

// Normalize applies canonical identity rules in place.
func (k *Key) Normalize() {
	if r := k.Code; r <= unicode.MaxRune {
		if unicode.IsUpper(r) {
			lower, ok := lowerRune(r)
			if ok && lower != r {
				if k.ShiftedKey == 0 {
					k.ShiftedKey = r
				}
				k.Code = lower
				if !k.Mod.Contains(KeyModCapsLock) {
					k.Mod |= KeyModShift
				}
			}
		} else if unicode.IsLower(r) && k.Mod&(KeyModShift|KeyModCapsLock) != 0 && k.ShiftedKey == 0 {
			upper := []rune(strings.ToUpper(string(r)))
			if len(upper) == 1 && upper[0] != r {
				k.ShiftedKey = upper[0]
			}
		}
	}
	if k.Text == "" && k.Mod.Clear(KeyModShift|KeyModCapsLock|KeyModNumLock) == 0 &&
		k.Code <= unicode.MaxRune && !unicode.IsControl(k.Code) {
		shifted := k.Mod.Contains(KeyModShift)
		if unicode.IsLower(k.Code) {
			shifted = k.Mod&(KeyModShift|KeyModCapsLock) != 0
		}
		g := k.Code
		if shifted && k.ShiftedKey != 0 {
			g = k.ShiftedKey
		}
		k.Text = string(g)
	}
}

// Equal reports equality using only code and binding modifiers.
func (k Key) Equal(o Key) bool {
	return k.Code == o.Code && k.Mod.Clear(KeyModLockMask) == o.Mod.Clear(KeyModLockMask)
}

// BindingKey returns the comparable map key for k's binding identity.
func (k Key) BindingKey() Key {
	return Key{Code: k.Code, Mod: k.Mod.Clear(KeyModLockMask)}
}

// Matches reports whether k matches a binding pattern.
func (k Key) Matches(pattern string) bool {
	if k.Text == pattern {
		return true
	}
	p, err := ParseKey(pattern)
	return err == nil && k.Equal(p)
}

// MatchesAny reports whether k matches any binding pattern.
func (k Key) MatchesAny(patterns ...string) bool {
	for _, p := range patterns {
		if k.Matches(p) {
			return true
		}
	}
	return false
}

// String returns the canonical binding spelling.
func (k Key) String() string {
	var b strings.Builder
	mods := k.Mod
	if mods.Contains(KeyModCtrl) {
		b.WriteString("ctrl+")
	}
	if mods.Contains(KeyModAlt) {
		b.WriteString("alt+")
	}
	if mods.Contains(KeyModShift) {
		b.WriteString("shift+")
	}
	if mods.Contains(KeyModSuper) {
		b.WriteString("super+")
	}
	if mods.Contains(KeyModHyper) {
		b.WriteString("hyper+")
	}
	if mods.Contains(KeyModMeta) {
		b.WriteString("meta+")
	}
	b.WriteString(k.keyName())
	return b.String()
}

func (k Key) keyName() string {
	if name, ok := keyCodeNames[k.Code]; ok {
		return name
	}
	if k.Code >= KeyF1 && k.Code <= KeyF35 {
		return "f" + strconv.Itoa(int(k.Code-KeyF1)+1)
	}
	if k.Code <= unicode.MaxRune {
		switch r := k.Code; r {
		case '+':
			return "plus"
		case '-':
			return "minus"
		case '=':
			return "equals"
		default:
			return string(r)
		}
	}
	return fmt.Sprintf("KeyCode(%d)", k.Code)
}

var keyCodeNames = map[rune]string{
	KeyFind: "find", KeySelect: "select", KeyUp: "up", KeyDown: "down", KeyLeft: "left", KeyRight: "right", KeyHome: "home", KeyEnd: "end", KeyPageUp: "pageup", KeyPageDown: "pagedown", KeyBackspace: "backspace", KeyDelete: "delete", KeyInsert: "insert", KeyTab: "tab", KeyEnter: "enter", KeySpace: "space", KeyEscape: "escape", KeyCapsLock: "capslock", KeyScrollLock: "scrolllock", KeyNumLock: "numlock", KeyPrintScreen: "printscreen", KeyPause: "pause", KeyMenu: "menu", KeyKpEnter: "kpenter", KeyKpAdd: "kpadd", KeyKpSubtract: "kpsubtract", KeyKpMultiply: "kpmultiply", KeyKpDivide: "kpdivide", KeyKpDecimal: "kpdecimal", KeyKpEqual: "kpequal", KeyKpSeparator: "kpseparator", KeyKpLeft: "kpleft", KeyKpRight: "kpright", KeyKpUp: "kpup", KeyKpDown: "kpdown", KeyKpPageUp: "kppageup", KeyKpPageDown: "kppagedown", KeyKpHome: "kphome", KeyKpEnd: "kpend", KeyKpInsert: "kpinsert", KeyKpDelete: "kpdelete", KeyKpBegin: "kpbegin", KeyKp0: "kp0", KeyKp1: "kp1", KeyKp2: "kp2", KeyKp3: "kp3", KeyKp4: "kp4", KeyKp5: "kp5", KeyKp6: "kp6", KeyKp7: "kp7", KeyKp8: "kp8", KeyKp9: "kp9", KeyMediaPlay: "mediaplay", KeyMediaPause: "mediapause", KeyMediaPlayPause: "mediaplaypause", KeyMediaReverse: "mediareverse", KeyMediaStop: "mediastop", KeyMediaRewind: "mediarewind", KeyMediaFastForward: "mediafastforward", KeyMediaNext: "medianext", KeyMediaPrev: "mediaprev", KeyMediaRecord: "mediarecord", KeyVolumeUp: "volumeup", KeyVolumeDown: "volumedown", KeyVolumeMute: "volumemute", KeyLeftShift: "leftshift", KeyRightShift: "rightshift", KeyLeftCtrl: "leftctrl", KeyRightCtrl: "rightctrl", KeyLeftAlt: "leftalt", KeyRightAlt: "rightalt", KeyLeftSuper: "leftsuper", KeyRightSuper: "rightsuper", KeyLeftHyper: "lefthyper", KeyRightHyper: "righthyper", KeyLeftMeta: "leftmeta", KeyRightMeta: "rightmeta", KeyIsoLevel3Shift: "isolevel3shift", KeyIsoLevel5Shift: "isolevel5shift",
}

// ParseKeyErrorKind identifies a key parse failure.
type ParseKeyErrorKind int

const (
	// ParseKeyEmpty means the input was empty or whitespace-only.
	ParseKeyEmpty ParseKeyErrorKind = iota
	// ParseKeyEmptyComponent means a plus-separated component was empty.
	ParseKeyEmptyComponent
	// ParseKeyUnknownModifier means a modifier token was not recognized.
	ParseKeyUnknownModifier
	// ParseKeyUnknownKey means the key token was not recognized.
	ParseKeyUnknownKey
	// ParseKeyInvalidFunctionKey means a function key number was out of range.
	ParseKeyInvalidFunctionKey
)

// ParseKeyError reports why a binding string did not parse.
type ParseKeyError struct {
	// Kind is the failure category.
	Kind ParseKeyErrorKind
	// Token is the offending token when useful.
	Token string
}

// Error returns the parse error text.
func (e *ParseKeyError) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case ParseKeyEmpty:
		return "empty key string"
	case ParseKeyEmptyComponent:
		return "empty `+`-separated component"
	case ParseKeyUnknownModifier:
		return fmt.Sprintf("unknown modifier: %q", e.Token)
	case ParseKeyUnknownKey:
		return fmt.Sprintf("unknown key name: %q", e.Token)
	case ParseKeyInvalidFunctionKey:
		return fmt.Sprintf("invalid function key: %q", e.Token)
	default:
		return "invalid key"
	}
}

// Is reports whether target is the same parse error kind.
func (e *ParseKeyError) Is(target error) bool {
	var t *ParseKeyError
	return errors.As(target, &t) && e.Kind == t.Kind
}

// ParseKey parses a binding string into a Key.
func ParseKey(s string) (Key, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Key{}, &ParseKeyError{Kind: ParseKeyEmpty}
	}
	if len([]rune(s)) == 1 {
		code, err := parseKeyCode(s)
		if err != nil {
			return Key{}, err
		}
		return Key{Code: code}.Normalized(), nil
	}
	modPart, keyPart := "", s
	if strings.HasSuffix(s, "+") {
		head := strings.TrimSuffix(s, "+")
		if strings.HasSuffix(head, "+") {
			modPart, keyPart = strings.TrimSuffix(head, "+"), "+"
		} else {
			return Key{}, &ParseKeyError{Kind: ParseKeyEmptyComponent}
		}
	} else if i := strings.LastIndexByte(s, '+'); i >= 0 {
		if i == 0 {
			return Key{}, &ParseKeyError{Kind: ParseKeyEmptyComponent}
		}
		modPart, keyPart = s[:i], s[i+1:]
	}
	if keyPart == "" {
		return Key{}, &ParseKeyError{Kind: ParseKeyEmptyComponent}
	}
	var mods KeyMod
	if modPart != "" {
		for _, tok := range strings.Split(modPart, "+") {
			if tok == "" {
				return Key{}, &ParseKeyError{Kind: ParseKeyEmptyComponent}
			}
			m, err := parseModifier(tok)
			if err != nil {
				return Key{}, err
			}
			mods |= m
		}
	}
	if strings.EqualFold(keyPart, "backtab") {
		return Key{Code: KeyTab, Mod: mods | KeyModShift}.Normalized(), nil
	}
	code, err := parseKeyCode(keyPart)
	if err != nil {
		return Key{}, err
	}
	return Key{Code: code, Mod: mods}.Normalized(), nil
}

// ParseKeyCode parses a key-code string.
func ParseKeyCode(s string) (rune, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, &ParseKeyError{Kind: ParseKeyEmpty}
	}
	code, err := parseKeyCode(s)
	return code, err
}

func parseModifier(token string) (KeyMod, error) {
	switch strings.ToLower(token) {
	case "ctrl", "control":
		return KeyModCtrl, nil
	case "alt", "opt", "option":
		return KeyModAlt, nil
	case "shift":
		return KeyModShift, nil
	case "super", "win", "cmd", "command":
		return KeyModSuper, nil
	case "hyper":
		return KeyModHyper, nil
	case "meta":
		return KeyModMeta, nil
	default:
		return 0, &ParseKeyError{Kind: ParseKeyUnknownModifier, Token: token}
	}
}

func parseKeyCode(token string) (rune, error) {
	runes := []rune(token)
	if len(runes) == 1 {
		return rune(runes[0]), nil
	}
	if len(token) > 16 || !isASCII(token) {
		return 0, &ParseKeyError{Kind: ParseKeyUnknownKey, Token: token}
	}
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "f") && len(lower) > 1 {
		allDigits := true
		for _, c := range lower[1:] {
			allDigits = allDigits && c >= '0' && c <= '9'
		}
		if allDigits {
			n, err := strconv.Atoi(lower[1:])
			if err != nil || n < 1 || n > int(KeyF35-KeyF1)+1 {
				return 0, &ParseKeyError{Kind: ParseKeyInvalidFunctionKey, Token: token}
			}
			return KeyF1 + rune(n-1), nil
		}
	}
	if code, ok := keyAliases[lower]; ok {
		return code, nil
	}
	switch lower {
	case "plus":
		return '+', nil
	case "minus", "dash", "hyphen":
		return '-', nil
	case "equals", "equal":
		return '=', nil
	default:
		return 0, &ParseKeyError{Kind: ParseKeyUnknownKey, Token: token}
	}
}

func lowerRune(r rune) (rune, bool) {
	if r == 'İ' {
		return r, false
	}
	lower := []rune(strings.ToLower(string(r)))
	if len(lower) != 1 {
		return r, false
	}
	return lower[0], true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

var keyAliases = map[string]rune{
	"up": KeyUp, "down": KeyDown, "left": KeyLeft, "right": KeyRight, "home": KeyHome, "end": KeyEnd, "find": KeyFind, "select": KeySelect, "pgup": KeyPageUp, "pageup": KeyPageUp, "pgdn": KeyPageDown, "pgdown": KeyPageDown, "pagedown": KeyPageDown, "backspace": KeyBackspace, "bs": KeyBackspace, "delete": KeyDelete, "del": KeyDelete, "insert": KeyInsert, "ins": KeyInsert, "tab": KeyTab, "enter": KeyEnter, "return": KeyEnter, "ret": KeyEnter, "space": KeySpace, "spc": KeySpace, "esc": KeyEscape, "escape": KeyEscape, "capslock": KeyCapsLock, "scrolllock": KeyScrollLock, "numlock": KeyNumLock, "printscreen": KeyPrintScreen, "prtsc": KeyPrintScreen, "pause": KeyPause, "menu": KeyMenu, "kpenter": KeyKpEnter, "kpadd": KeyKpAdd, "kpsubtract": KeyKpSubtract, "kpmultiply": KeyKpMultiply, "kpdivide": KeyKpDivide, "kpdecimal": KeyKpDecimal, "kpequal": KeyKpEqual, "kpseparator": KeyKpSeparator, "kpleft": KeyKpLeft, "kpright": KeyKpRight, "kpup": KeyKpUp, "kpdown": KeyKpDown, "kppgup": KeyKpPageUp, "kppageup": KeyKpPageUp, "kppgdn": KeyKpPageDown, "kppgdown": KeyKpPageDown, "kppagedown": KeyKpPageDown, "kphome": KeyKpHome, "kpend": KeyKpEnd, "kpinsert": KeyKpInsert, "kpdelete": KeyKpDelete, "kpbegin": KeyKpBegin, "kp0": KeyKp0, "kp1": KeyKp1, "kp2": KeyKp2, "kp3": KeyKp3, "kp4": KeyKp4, "kp5": KeyKp5, "kp6": KeyKp6, "kp7": KeyKp7, "kp8": KeyKp8, "kp9": KeyKp9, "mediaplay": KeyMediaPlay, "mediapause": KeyMediaPause, "mediaplaypause": KeyMediaPlayPause, "mediareverse": KeyMediaReverse, "mediastop": KeyMediaStop, "mediarewind": KeyMediaRewind, "mediafastforward": KeyMediaFastForward, "medianext": KeyMediaNext, "mediaprev": KeyMediaPrev, "mediarecord": KeyMediaRecord, "volumeup": KeyVolumeUp, "volumedown": KeyVolumeDown, "volumemute": KeyVolumeMute, "leftshift": KeyLeftShift, "rightshift": KeyRightShift, "leftctrl": KeyLeftCtrl, "rightctrl": KeyRightCtrl, "leftalt": KeyLeftAlt, "rightalt": KeyRightAlt, "leftsuper": KeyLeftSuper, "rightsuper": KeyRightSuper, "lefthyper": KeyLeftHyper, "righthyper": KeyRightHyper, "leftmeta": KeyLeftMeta, "rightmeta": KeyRightMeta, "isolevel3shift": KeyIsoLevel3Shift, "isolevel5shift": KeyIsoLevel5Shift,
}
