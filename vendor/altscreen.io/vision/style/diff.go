package style

import (
	"io"

	"altscreen.io/vision/ansi"
	termcolor "altscreen.io/vision/color"
)

// WriteDiff writes the compact visual transition from from to to, returning
// the number of bytes written. Nothing is written when the two look the same.
func WriteDiff(w io.Writer, from, to Style) (int, error) {
	var buf [sgrMax]byte
	out, wrote := AppendDiff(buf[:0], from, to)
	if !wrote {
		return 0, nil
	}
	return w.Write(out)
}

// AppendDiff appends the compact visual transition from from to to, and
// reports whether anything was appended. The caller owns dst, so a renderer
// can reuse one staging buffer across a frame instead of allocating per cell.
func AppendDiff(dst []byte, from, to Style) ([]byte, bool) {
	dst, wroteSGR := appendSGRDiff(dst, from, to)
	wroteLink := !linkEqual(from, to)
	if wroteLink {
		if to.HasLink() {
			dst = ansi.AppendHyperlink(dst, to.link.URL, to.link.Params)
		} else {
			dst = append(dst, ansi.HyperlinkReset...)
		}
	}
	return dst, wroteSGR || wroteLink
}

func appendSGRDiff(dst []byte, from, to Style) ([]byte, bool) {
	if sgrEqual(from, to) {
		return dst, false
	}
	if to.isSGREmpty() {
		return append(dst, reset...), true
	}
	if from.isSGREmpty() {
		return appendSGR(dst, to)
	}

	start := len(dst)
	dst = append(dst, '\x1b', '[')
	body := len(dst)

	removed := from.attrs &^ to.attrs
	if removed&(Bold|Faint) != 0 {
		dst = appendParam(dst, body, "22")
		if to.attrs.Contains(Bold) {
			dst = appendParam(dst, body, "1")
		}
		if to.attrs.Contains(Faint) {
			dst = appendParam(dst, body, "2")
		}
	}
	if removed&(SlowBlink|RapidBlink) != 0 {
		dst = appendParam(dst, body, "25")
		if to.attrs.Contains(SlowBlink) {
			dst = appendParam(dst, body, "5")
		}
		if to.attrs.Contains(RapidBlink) {
			dst = appendParam(dst, body, "6")
		}
	}
	for _, x := range attrResetCodes {
		if removed.Contains(x.flag) {
			dst = appendParam(dst, body, x.code)
		}
	}

	added := to.attrs &^ from.attrs
	boldFaintHandled := removed&(Bold|Faint) != 0
	blinkHandled := removed&(SlowBlink|RapidBlink) != 0
	for _, x := range attrCodes {
		if boldFaintHandled && (x.flag == Bold || x.flag == Faint) {
			continue
		}
		if blinkHandled && (x.flag == SlowBlink || x.flag == RapidBlink) {
			continue
		}
		if added.Contains(x.flag) {
			dst = appendParam(dst, body, x.code)
		}
	}

	if from.underlineStyle != to.underlineStyle {
		if to.underlineStyle == UnderlineNone {
			dst = appendParam(dst, body, "24")
		} else {
			dst = appendParam(dst, body, underlineParam(to.underlineStyle))
		}
	}
	if !colorEqual(from.fg, to.fg) {
		if to.fg == nil {
			dst = appendParam(dst, body, "39")
		} else {
			dst = appendFg(appendSep(dst, body), to.fg)
		}
	}
	if !colorEqual(from.bg, to.bg) {
		if to.bg == nil {
			dst = appendParam(dst, body, "49")
		} else {
			dst = appendBg(appendSep(dst, body), to.bg)
		}
	}
	if !colorEqual(from.underlineColor, to.underlineColor) {
		if to.underlineColor == nil {
			dst = appendParam(dst, body, "59")
		} else {
			dst = appendUnderlineColor(appendSep(dst, body), to.underlineColor)
		}
	}

	if len(dst) == body {
		return dst[:start], false
	}
	return append(dst, 'm'), true
}

// Convert returns style converted for profile.
func Convert(style Style, profile termcolor.Profile) Style {
	switch profile {
	case termcolor.Disabled:
		return Style{}
	case termcolor.Ascii:
		style.fg, style.bg, style.underlineColor = nil, nil, nil
		return style
	default:
		style.fg = profile.Convert(style.fg)
		style.bg = profile.Convert(style.bg)
		style.underlineColor = profile.Convert(style.underlineColor)
		return style
	}
}

// linkEqual compares two styles' hyperlinks by content. Styles without a link
// are equal however that absence is spelled, since a link is only present when
// its URL is non-empty.
func linkEqual(a, b Style) bool {
	if !a.HasLink() || !b.HasLink() {
		return a.HasLink() == b.HasLink()
	}
	return *a.link == *b.link
}
