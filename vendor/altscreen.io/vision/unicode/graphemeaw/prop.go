package graphemeaw

import "unicode/utf8"

// Properties holds the segmentation and width properties of a single rune.
type Properties struct {
	entry uint16
}

// Class returns the grapheme cluster break class.
func (p Properties) Class() Class {
	return Class(p.entry & classMask)
}

// EastAsianWidth returns the East Asian Width property, as defined by UAX #11.
func (p Properties) EastAsianWidth() EastAsianWidth {
	return EastAsianWidth((p.entry & eawMask) >> eawShift)
}

// IsDefaultIgnorable reports whether the rune has the
// Default_Ignorable_Code_Point property, meaning it is normally rendered as
// nothing at all.
func (p Properties) IsDefaultIgnorable() bool {
	return p.entry&ignorableFlag != 0
}

// IsZeroWidth reports whether the rune's general category is Mn, Me, or Cf:
// the marks and format characters that advance the cursor by nothing.
func (p Properties) IsZeroWidth() bool {
	return p.entry&zeroWidthFlag != 0
}

// IsEmojiPresentation reports whether the rune defaults to emoji presentation
// rather than text presentation.
func (p Properties) IsEmojiPresentation() bool {
	return p.entry&emojiPresentationFlag != 0
}

// IsControl reports whether the rune has class CR, LF, or Control (GB4/GB5).
func (p Properties) IsControl() bool {
	c := p.Class()
	return c == CR || c == LF || c == Control
}

// IsExtend reports whether the rune has class Extend, ZWJ, InCBExtend, or
// InCBLinker — the set of classes that are ignored between other classes
// under rules GB9/GB9a/GB9c.
func (p Properties) IsExtend() bool {
	c := p.Class()
	return c == Extend || c == ZWJ || c == InCBExtend || c == InCBLinker
}

// IsHangul reports whether the rune has a Hangul jamo or syllable class
// (L, V, T, LV, or LVT).
func (p Properties) IsHangul() bool {
	c := p.Class()
	return c >= L && c <= LVT
}

// Width returns the cell width of the rune on its own, disregarding any
// cluster it belongs to. Callers measuring a whole cluster should use
// [Segmenter.Width], which also accounts for variation selectors and flags.
//
// ambiguousWide selects the East Asian Ambiguous policy; see [Options].
func (p Properties) Width(ambiguousWide bool) int {
	if p.IsDefaultIgnorable() || p.IsZeroWidth() {
		return 0
	}
	switch p.EastAsianWidth() {
	case EastAsianWide, EastAsianFullwidth:
		return 2
	case EastAsianAmbiguous:
		if ambiguousWide {
			return 2
		}
	}
	return 1
}

// Lookup returns properties for the first rune in s and the width in bytes of
// its encoding. The size will be 0 if s does not hold enough bytes to complete
// the encoding.
func Lookup(s []byte) (Properties, int) {
	v, sz := trie.lookup(s)
	return Properties{entry: v}, sz
}

// LookupString returns properties for the first rune in s and the width in
// bytes of its encoding. The size will be 0 if s does not hold enough bytes to
// complete the encoding.
func LookupString(s string) (Properties, int) {
	v, sz := trie.lookupString(s)
	return Properties{entry: v}, sz
}

// LookupRune returns properties for r.
func LookupRune(r rune) (Properties, int) {
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], r)
	return Lookup(buf[:n])
}
