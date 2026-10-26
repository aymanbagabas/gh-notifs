// Package style defines terminal text styles and compact style transitions.
package style

import (
	"fmt"
	imagecolor "image/color"
	"io"

	"altscreen.io/vision/ansi"
)

// Link is an OSC 8 hyperlink target carried by a Style.
type Link struct {
	// URL is the target URI written into the OSC 8 sequence.
	URL string
	// Params is the OSC 8 parameter string written between the semicolons.
	Params string
}

// Attr is a bitset of SGR text attributes.
type Attr uint

const (
	// Bold is bold or intense text.
	Bold Attr = 1 << iota
	// Faint is decreased-intensity text.
	Faint
	// Italic is italic text.
	Italic
	// SlowBlink is slow blinking text.
	SlowBlink
	// RapidBlink is rapid blinking text.
	RapidBlink
	// Reverse swaps foreground and background.
	Reverse
	// Conceal hides text.
	Conceal
	// Strikethrough draws struck-through text.
	Strikethrough
)

// Contains reports whether all bits in flags are set.
func (a Attr) Contains(flags Attr) bool { return a&flags == flags }

// Set returns a with flags enabled.
func (a Attr) Set(flags Attr) Attr { return a | flags }

// Clear returns a with flags disabled.
func (a Attr) Clear(flags Attr) Attr { return a &^ flags }

// Underline is an SGR underline shape.
type Underline uint8

const (
	// UnderlineNone means no underline.
	UnderlineNone Underline = iota
	// UnderlineSingle is a single underline.
	UnderlineSingle
	// UnderlineDouble is a double underline.
	UnderlineDouble
	// UnderlineCurly is a curly underline.
	UnderlineCurly
	// UnderlineDotted is a dotted underline.
	UnderlineDotted
	// UnderlineDashed is a dashed underline.
	UnderlineDashed
)

// Style is a complete terminal text style. Its fields are unexported: build a
// style by chaining setters onto the zero value, and read one back with the
// Get accessors.
type Style struct {
	fg             imagecolor.Color
	bg             imagecolor.Color
	underlineColor imagecolor.Color
	underlineStyle Underline
	attrs          Attr
	link           *Link
}

// New returns an empty style.
func New() Style { return Style{} }

// Empty is the style with no colors, attributes, underline, or hyperlink.
var Empty Style

// IsEmpty reports whether s has no terminal-visible state.
func (s Style) IsEmpty() bool { return s.isSGREmpty() && !s.HasLink() }

// HasLink reports whether s carries an OSC 8 hyperlink. A link is present when
// its URL is non-empty, which is what Link stores and what OSC 8 needs: an
// empty URL is the sequence that closes a hyperlink, not one that opens it.
// HasLink answers the question GetLink is usually asked without copying the
// target out.
func (s Style) HasLink() bool { return s.link != nil && s.link.URL != "" }

func (s Style) isSGREmpty() bool {
	return s.fg == nil && s.bg == nil && s.underlineColor == nil && s.underlineStyle == UnderlineNone && s.attrs == 0
}

// Equal reports whether s and other describe the same style.
// Equal compares hyperlinks by content; == on Style compares them by pointer identity.
func (s Style) Equal(other Style) bool {
	if !colorEqual(s.fg, other.fg) || !colorEqual(s.bg, other.bg) || !colorEqual(s.underlineColor, other.underlineColor) || s.underlineStyle != other.underlineStyle || s.attrs != other.attrs {
		return false
	}
	return linkEqual(s, other)
}

func sgrEqual(a, b Style) bool {
	return colorEqual(a.fg, b.fg) && colorEqual(a.bg, b.bg) && colorEqual(a.underlineColor, b.underlineColor) && a.underlineStyle == b.underlineStyle && a.attrs == b.attrs
}

func colorEqual(a, b imagecolor.Color) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	ra, ga, ba, aa := a.RGBA()
	rb, gb, bb, ab := b.RGBA()
	return ra == rb && ga == gb && ba == bb && aa == ab
}

// Inherit returns s with unset fields filled from base.
func (s Style) Inherit(base Style) Style {
	if s.fg == nil {
		s.fg = base.fg
	}
	if s.bg == nil {
		s.bg = base.bg
	}
	if s.underlineColor == nil {
		s.underlineColor = base.underlineColor
	}
	if s.underlineStyle == UnderlineNone {
		s.underlineStyle = base.underlineStyle
	}
	s.attrs |= base.attrs
	if !s.HasLink() {
		s.link = base.link
	}
	return s
}

// Bold returns s with bold enabled.
func (s Style) Bold() Style { s.attrs |= Bold; return s }

// Faint returns s with faint enabled.
func (s Style) Faint() Style { s.attrs |= Faint; return s }

// Italic returns s with italic enabled.
func (s Style) Italic() Style { s.attrs |= Italic; return s }

// Underline returns s with a single underline.
func (s Style) Underline() Style { s.underlineStyle = UnderlineSingle; return s }

// Strikethrough returns s with strikethrough enabled.
func (s Style) Strikethrough() Style { s.attrs |= Strikethrough; return s }

// Blink returns s with slow blink enabled.
func (s Style) Blink() Style { s.attrs |= SlowBlink; return s }

// RapidBlink returns s with rapid blink enabled.
func (s Style) RapidBlink() Style { s.attrs |= RapidBlink; return s }

// Reverse returns s with reverse video enabled.
func (s Style) Reverse() Style { s.attrs |= Reverse; return s }

// Conceal returns s with conceal enabled.
func (s Style) Conceal() Style { s.attrs |= Conceal; return s }

// Foreground returns s with fg as the foreground color.
func (s Style) Foreground(fg imagecolor.Color) Style { s.fg = fg; return s }

// Background returns s with bg as the background color.
func (s Style) Background(bg imagecolor.Color) Style { s.bg = bg; return s }

// UnderlineColor returns s with c as the underline color.
func (s Style) UnderlineColor(c imagecolor.Color) Style { s.underlineColor = c; return s }

// UnderlineStyle returns s with u as the underline shape.
func (s Style) UnderlineStyle(u Underline) Style { s.underlineStyle = u; return s }

// Attrs returns s with attrs as its complete attribute set.
func (s Style) Attrs(attrs Attr) Style { s.attrs = attrs; return s }

// Link returns s with an OSC 8 hyperlink, or clears it when url is empty.
func (s Style) Link(url, params string) Style {
	if url == "" {
		s.link = nil
	} else {
		s.link = &Link{URL: url, Params: params}
	}
	return s
}

// GetBold reports whether bold is enabled.
func (s Style) GetBold() bool { return s.attrs.Contains(Bold) }

// GetFaint reports whether faint is enabled.
func (s Style) GetFaint() bool { return s.attrs.Contains(Faint) }

// GetItalic reports whether italic is enabled.
func (s Style) GetItalic() bool { return s.attrs.Contains(Italic) }

// GetUnderline reports whether the underline is a single underline. Ask
// GetUnderlineStyle for any other shape.
func (s Style) GetUnderline() bool { return s.underlineStyle == UnderlineSingle }

// GetStrikethrough reports whether strikethrough is enabled.
func (s Style) GetStrikethrough() bool { return s.attrs.Contains(Strikethrough) }

// GetBlink reports whether slow blink is enabled.
func (s Style) GetBlink() bool { return s.attrs.Contains(SlowBlink) }

// GetRapidBlink reports whether rapid blink is enabled.
func (s Style) GetRapidBlink() bool { return s.attrs.Contains(RapidBlink) }

// GetReverse reports whether reverse video is enabled.
func (s Style) GetReverse() bool { return s.attrs.Contains(Reverse) }

// GetConceal reports whether conceal is enabled.
func (s Style) GetConceal() bool { return s.attrs.Contains(Conceal) }

// GetForeground returns the foreground color, or nil when unset.
func (s Style) GetForeground() imagecolor.Color { return s.fg }

// GetBackground returns the background color, or nil when unset.
func (s Style) GetBackground() imagecolor.Color { return s.bg }

// GetUnderlineColor returns the underline color, or nil when unset.
func (s Style) GetUnderlineColor() imagecolor.Color { return s.underlineColor }

// GetUnderlineStyle returns the underline shape.
func (s Style) GetUnderlineStyle() Underline { return s.underlineStyle }

// GetAttrs returns the boolean SGR text attributes.
func (s Style) GetAttrs() Attr { return s.attrs }

// GetLink returns the OSC 8 hyperlink target and whether one is set. It returns
// a copy, so the style cannot be changed through it.
func (s Style) GetLink() (Link, bool) {
	if !s.HasLink() {
		return Link{}, false
	}
	return *s.link, true
}

// Opener returns the additive SGR and OSC 8 opening sequences for s.
func (s Style) Opener() string {
	var buf [sgrMax]byte
	out, _ := appendSGR(buf[:0], s)
	if s.HasLink() {
		out = ansi.AppendHyperlink(out, s.link.URL, s.link.Params)
	}
	return string(out)
}

// Closer returns the OSC 8 terminator and SGR reset needed to close s.
func (s Style) Closer() string {
	if s.HasLink() {
		if s.isSGREmpty() {
			return ansi.HyperlinkReset
		}
		return ansi.HyperlinkReset + reset
	}
	if s.isSGREmpty() {
		return ""
	}
	return reset
}

// Format writes the opener for %v and the closer for %#v.
func (s Style) Format(f fmt.State, verb rune) {
	out := s.Opener()
	if f.Flag('#') {
		out = s.Closer()
	}
	_, _ = io.WriteString(f, out)
}
