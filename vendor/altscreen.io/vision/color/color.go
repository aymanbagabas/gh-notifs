// Package color defines terminal colors, palettes, and capability profiles.
package color

import (
	"errors"
	"fmt"
	"image/color"
	"math"
)

// Named is one of the 16 ANSI named colors.
type Named uint8

const (
	// Black is ANSI palette color 0.
	Black Named = iota
	// Red is ANSI palette color 1.
	Red
	// Green is ANSI palette color 2.
	Green
	// Yellow is ANSI palette color 3.
	Yellow
	// Blue is ANSI palette color 4.
	Blue
	// Magenta is ANSI palette color 5.
	Magenta
	// Cyan is ANSI palette color 6.
	Cyan
	// White is ANSI palette color 7.
	White
	// BrightBlack is ANSI palette color 8.
	BrightBlack
	// BrightRed is ANSI palette color 9.
	BrightRed
	// BrightGreen is ANSI palette color 10.
	BrightGreen
	// BrightYellow is ANSI palette color 11.
	BrightYellow
	// BrightBlue is ANSI palette color 12.
	BrightBlue
	// BrightMagenta is ANSI palette color 13.
	BrightMagenta
	// BrightCyan is ANSI palette color 14.
	BrightCyan
	// BrightWhite is ANSI palette color 15.
	BrightWhite
)

// Indexed is a color from the 256-color palette.
type Indexed uint8

// RGB is a 24-bit color.
type RGB struct{ R, G, B uint8 }

// NamedFromIndex converts a 0..15 ANSI palette index to a named color.
func NamedFromIndex(v uint8) (Named, bool) {
	if v > uint8(BrightWhite) {
		return 0, false
	}
	return Named(v), true
}

// RGBA returns alpha-premultiplied 16-bit channels.
func (n Named) RGBA() (r, g, b, a uint32) { return Indexed(n).RGBA() }

// RGBA returns alpha-premultiplied 16-bit channels.
func (i Indexed) RGBA() (r, g, b, a uint32) { return indexedToRGB(uint8(i)).RGBA() }

// RGBA returns alpha-premultiplied 16-bit channels.
func (c RGB) RGBA() (r, g, b, a uint32) {
	return uint32(c.R) * 0x101, uint32(c.G) * 0x101, uint32(c.B) * 0x101, 0xffff
}

// String returns the named color name or its numeric value if out of range.
func (n Named) String() string {
	if int(n) < len(namedNames) {
		return namedNames[n]
	}
	return fmt.Sprintf("Named(%d)", n)
}

// String returns the xterm palette index.
func (i Indexed) String() string { return fmt.Sprintf("Indexed(%d)", i) }

// String returns the color as #rrggbb.
func (c RGB) String() string { return Hex(c) }

var namedNames = [...]string{
	"Black", "Red", "Green", "Yellow", "Blue", "Magenta", "Cyan", "White",
	"BrightBlack", "BrightRed", "BrightGreen", "BrightYellow", "BrightBlue", "BrightMagenta", "BrightCyan", "BrightWhite",
}

// ToRGB converts c to 24-bit RGB.
func ToRGB(c color.Color) RGB {
	switch c := c.(type) {
	case nil:
		return RGB{}
	case RGB:
		return c
	case Named:
		return indexedToRGB(uint8(c))
	case Indexed:
		return indexedToRGB(uint8(c))
	default:
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		return RGB{rgba.R, rgba.G, rgba.B}
	}
}

// Hex formats c as a lower-case #rrggbb string.
func Hex(c color.Color) string {
	rgb := ToRGB(c)
	return fmt.Sprintf("#%02x%02x%02x", rgb.R, rgb.G, rgb.B)
}

// ParseHex parses rgb, rrggbb, or rrggbbaa hex into RGB.
func ParseHex(s string) (RGB, error) {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	var rgb RGB
	switch len(s) {
	case 3:
		r, ok := hexNibble(s[0])
		if !ok {
			return RGB{}, errInvalidHex
		}
		g, ok := hexNibble(s[1])
		if !ok {
			return RGB{}, errInvalidHex
		}
		b, ok := hexNibble(s[2])
		if !ok {
			return RGB{}, errInvalidHex
		}
		rgb = RGB{r * 17, g * 17, b * 17}
	case 6, 8:
		r, ok := hexByte(s[0], s[1])
		if !ok {
			return RGB{}, errInvalidHex
		}
		g, ok := hexByte(s[2], s[3])
		if !ok {
			return RGB{}, errInvalidHex
		}
		b, ok := hexByte(s[4], s[5])
		if !ok {
			return RGB{}, errInvalidHex
		}
		if len(s) == 8 {
			if _, ok := hexByte(s[6], s[7]); !ok {
				return RGB{}, errInvalidHex
			}
		}
		rgb = RGB{r, g, b}
	default:
		return RGB{}, errInvalidHex
	}
	return rgb, nil
}

var errInvalidHex = errors.New("invalid hex color")

func hexNibble(b byte) (uint8, bool) {
	switch {
	case '0' <= b && b <= '9':
		return b - '0', true
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10, true
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

func hexByte(hi, lo byte) (uint8, bool) {
	h, ok := hexNibble(hi)
	if !ok {
		return 0, false
	}
	l, ok := hexNibble(lo)
	if !ok {
		return 0, false
	}
	return h*16 + l, true
}

// HSL constructs RGB from hue, saturation, and lightness.
func HSL(h, s, l float64) RGB {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = min(max(s, 0), 1)
	l = min(max(l, 0), 1)
	chroma := (1 - math.Abs(2*l-1)) * s
	hp := h / 60
	x := chroma * (1 - math.Abs(math.Mod(hp, 2)-1))
	var r1, g1, b1 float64
	switch int(hp) {
	case 0:
		r1, g1 = chroma, x
	case 1:
		r1, g1 = x, chroma
	case 2:
		g1, b1 = chroma, x
	case 3:
		g1, b1 = x, chroma
	case 4:
		r1, b1 = x, chroma
	default:
		r1, b1 = chroma, x
	}
	m := l - chroma/2
	return RGB{hslByte(r1 + m), hslByte(g1 + m), hslByte(b1 + m)}
}

func hslByte(v float64) uint8 { return uint8(min(max(math.Round(v*255), 0), 255)) }

// ToHSL converts c to hue, saturation, and lightness.
func ToHSL(c color.Color) (h, s, l float64) {
	rgb := ToRGB(c)
	r := float64(rgb.R) / 255
	g := float64(rgb.G) / 255
	b := float64(rgb.B) / 255
	maxv := max(max(r, g), b)
	minv := min(min(r, g), b)
	delta := maxv - minv
	l = (maxv + minv) / 2
	if delta == 0 {
		return 0, 0, l
	}
	s = delta / (1 - math.Abs(2*l-1))
	switch maxv {
	case r:
		h = 60 * math.Mod((g-b)/delta, 6)
		if h < 0 {
			h += 360
		}
	case g:
		h = 60 * ((b-r)/delta + 2)
	default:
		h = 60 * ((r-g)/delta + 4)
	}
	return h, s, l
}

var xtermColors = makeXtermColors()

func makeXtermColors() [256]RGB {
	var table [256]RGB
	copy(table[:], []RGB{
		{0, 0, 0},
		{128, 0, 0},
		{0, 128, 0},
		{128, 128, 0},
		{0, 0, 128},
		{128, 0, 128},
		{0, 128, 128},
		{192, 192, 192},
		{128, 128, 128},
		{255, 0, 0},
		{0, 255, 0},
		{255, 255, 0},
		{0, 0, 255},
		{255, 0, 255},
		{0, 255, 255},
		{255, 255, 255},
	})
	cube := [...]uint8{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	i := 16
	for _, r := range cube {
		for _, g := range cube {
			for _, b := range cube {
				table[i] = RGB{r, g, b}
				i++
			}
		}
	}
	for i := 232; i < 256; i++ {
		v := uint8(8 + 10*(i-232))
		table[i] = RGB{v, v, v}
	}
	return table
}

func indexedToRGB(idx uint8) RGB { return xtermColors[idx] }
