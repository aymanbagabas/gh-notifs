package ansi

import "strconv"

// KittyKeyboardFlags are progressive keyboard reporting flags.
type KittyKeyboardFlags uint8

const (
	KittyDisambiguateEscapeCodes KittyKeyboardFlags = 1 << iota
	KittyReportEventTypes
	KittyReportAlternateKeys
	KittyReportAllKeysAsEscape
	KittyReportAssociatedText
)

const KittyKeyboardAll = KittyDisambiguateEscapeCodes | KittyReportEventTypes | KittyReportAlternateKeys | KittyReportAllKeysAsEscape | KittyReportAssociatedText

// Bits returns the raw flag mask.
func (f KittyKeyboardFlags) Bits() uint8 { return uint8(f) }

// Contains reports whether all flags in g are set.
func (f KittyKeyboardFlags) Contains(g KittyKeyboardFlags) bool { return f&g == g }

const RequestKittyKeyboard = "\x1b[?u"

// KittyKeyboardMode is the operation mode for SetKittyKeyboard.
type KittyKeyboardMode uint8

const (
	KittyKeyboardSet KittyKeyboardMode = iota + 1
	KittyKeyboardAdd
	KittyKeyboardRemove
)

// SetKittyKeyboard sets, adds, or removes keyboard enhancement flags.
func SetKittyKeyboard(flags KittyKeyboardFlags, mode KittyKeyboardMode) string {
	return "\x1b[=" + strconv.Itoa(int(flags.Bits())) + ";" + strconv.Itoa(int(mode)) + "u"
}

// PushKittyKeyboard pushes a keyboard enhancement stack frame.
func PushKittyKeyboard(flags KittyKeyboardFlags) string {
	return "\x1b[>" + strconv.Itoa(int(flags.Bits())) + "u"
}

// PopKittyKeyboard pops keyboard enhancement stack frames.
func PopKittyKeyboard(count int) string {
	if count <= 1 {
		return "\x1b[<u"
	}
	return "\x1b[<" + strconv.Itoa(count) + "u"
}
