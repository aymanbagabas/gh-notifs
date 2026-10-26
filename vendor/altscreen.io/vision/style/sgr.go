package style

import (
	imagecolor "image/color"
	"strconv"

	termcolor "altscreen.io/vision/color"
)

const reset = "\x1b[m"

// sgrMax is the longest SGR sequence the builder can produce: every attribute,
// an underline style, and three extended colors. Staging buffers start at this
// size so a transition never has to grow the slice; append still handles
// anything longer.
const sgrMax = 128

// attrCodes are the SGR parameters that set each attribute.
var attrCodes = [...]struct {
	flag Attr
	code string
}{
	{Bold, "1"},
	{Faint, "2"},
	{Italic, "3"},
	{SlowBlink, "5"},
	{RapidBlink, "6"},
	{Reverse, "7"},
	{Conceal, "8"},
	{Strikethrough, "9"},
}

// attrResetCodes are the SGR parameters that clear each attribute. Bold and
// faint share one reset, as do the two blinks, so those are handled apart.
var attrResetCodes = [...]struct {
	flag Attr
	code string
}{
	{Italic, "23"},
	{Reverse, "27"},
	{Conceal, "28"},
	{Strikethrough, "29"},
}

func sgr(s Style) string {
	var buf [sgrMax]byte
	out, ok := appendSGR(buf[:0], s)
	if !ok {
		return ""
	}
	return string(out)
}

// appendSGR appends the full SGR sequence for s and reports whether it wrote
// anything. A style with no attributes and no colors produces nothing.
func appendSGR(dst []byte, s Style) ([]byte, bool) {
	start := len(dst)
	dst = append(dst, '\x1b', '[')
	body := len(dst)

	for _, x := range attrCodes {
		if s.attrs.Contains(x.flag) {
			dst = appendParam(dst, body, x.code)
		}
	}
	if p := underlineParam(s.underlineStyle); p != "" {
		dst = appendParam(dst, body, p)
	}
	if s.fg != nil {
		dst = appendFg(appendSep(dst, body), s.fg)
	}
	if s.bg != nil {
		dst = appendBg(appendSep(dst, body), s.bg)
	}
	if s.underlineColor != nil {
		dst = appendUnderlineColor(appendSep(dst, body), s.underlineColor)
	}

	if len(dst) == body {
		return dst[:start], false
	}
	return append(dst, 'm'), true
}

// appendSep writes the parameter separator unless the body is still empty.
func appendSep(dst []byte, body int) []byte {
	if len(dst) > body {
		return append(dst, ';')
	}
	return dst
}

func appendParam(dst []byte, body int, param string) []byte {
	return append(appendSep(dst, body), param...)
}

func underlineParam(u Underline) string {
	switch u {
	case UnderlineSingle:
		return "4"
	case UnderlineDouble:
		return "4:2"
	case UnderlineCurly:
		return "4:3"
	case UnderlineDotted:
		return "4:4"
	case UnderlineDashed:
		return "4:5"
	default:
		return ""
	}
}

func appendFg(dst []byte, c imagecolor.Color) []byte {
	if n, ok := c.(termcolor.Named); ok {
		i := int(n)
		if i >= 8 {
			return strconv.AppendInt(dst, int64(90+i-8), 10)
		}
		return strconv.AppendInt(dst, int64(30+i), 10)
	}
	switch c := c.(type) {
	case termcolor.Indexed:
		return strconv.AppendInt(append(dst, "38;5;"...), int64(c), 10)
	case termcolor.RGB:
		return appendRGB(dst, "38;2;", c, ';')
	default:
		return appendRGB(dst, "38;2;", termcolor.ToRGB(c), ';')
	}
}

func appendBg(dst []byte, c imagecolor.Color) []byte {
	if n, ok := c.(termcolor.Named); ok {
		i := int(n)
		if i >= 8 {
			return strconv.AppendInt(dst, int64(100+i-8), 10)
		}
		return strconv.AppendInt(dst, int64(40+i), 10)
	}
	switch c := c.(type) {
	case termcolor.Indexed:
		return strconv.AppendInt(append(dst, "48;5;"...), int64(c), 10)
	case termcolor.RGB:
		return appendRGB(dst, "48;2;", c, ';')
	default:
		return appendRGB(dst, "48;2;", termcolor.ToRGB(c), ';')
	}
}

func appendUnderlineColor(dst []byte, c imagecolor.Color) []byte {
	if n, ok := c.(termcolor.Named); ok {
		return strconv.AppendInt(append(dst, "58:5:"...), int64(n), 10)
	}
	switch c := c.(type) {
	case termcolor.Indexed:
		return strconv.AppendInt(append(dst, "58:5:"...), int64(c), 10)
	case termcolor.RGB:
		return appendRGB(dst, "58:2::", c, ':')
	default:
		return appendRGB(dst, "58:2::", termcolor.ToRGB(c), ':')
	}
}

func appendRGB(dst []byte, prefix string, rgb termcolor.RGB, sep byte) []byte {
	dst = append(dst, prefix...)
	dst = strconv.AppendUint(dst, uint64(rgb.R), 10)
	dst = append(dst, sep)
	dst = strconv.AppendUint(dst, uint64(rgb.G), 10)
	dst = append(dst, sep)
	return strconv.AppendUint(dst, uint64(rgb.B), 10)
}
