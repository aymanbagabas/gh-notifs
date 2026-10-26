package style

import (
	imagecolor "image/color"

	"altscreen.io/vision/ansi"
	termcolor "altscreen.io/vision/color"
)

// Read applies SGR parameters to s.
func Read(params ansi.Params, s *Style) {
	if params.IsEmpty() {
		*s = Style{}
		return
	}
	groups := params.Groups()
	for i := 0; i < len(groups); i++ {
		g := groups[i]
		switch mainParam(g) {
		case 0:
			*s = Style{}
		case 1:
			s.attrs |= Bold
		case 2:
			s.attrs |= Faint
		case 3:
			s.attrs |= Italic
		case 4:
			if v, ok := g.Nth(1); ok {
				s.underlineStyle = underlineFromValue(v)
			} else {
				s.underlineStyle = UnderlineSingle
			}
		case 5:
			s.attrs |= SlowBlink
		case 6:
			s.attrs |= RapidBlink
		case 7:
			s.attrs |= Reverse
		case 8:
			s.attrs |= Conceal
		case 9:
			s.attrs |= Strikethrough
		case 21:
			s.underlineStyle = UnderlineDouble
		case 22:
			s.attrs = s.attrs.Clear(Bold | Faint)
		case 23:
			s.attrs = s.attrs.Clear(Italic)
		case 24:
			s.underlineStyle = UnderlineNone
		case 25:
			s.attrs = s.attrs.Clear(SlowBlink | RapidBlink)
		case 27:
			s.attrs = s.attrs.Clear(Reverse)
		case 28:
			s.attrs = s.attrs.Clear(Conceal)
		case 29:
			s.attrs = s.attrs.Clear(Strikethrough)
		case 30, 31, 32, 33, 34, 35, 36, 37:
			s.fg, _ = termcolor.NamedFromIndex(uint8(mainParam(g) - 30))
		case 38:
			s.fg = readExtendedColor(groups, &i)
		case 39:
			s.fg = nil
		case 40, 41, 42, 43, 44, 45, 46, 47:
			s.bg, _ = termcolor.NamedFromIndex(uint8(mainParam(g) - 40))
		case 48:
			s.bg = readExtendedColor(groups, &i)
		case 49:
			s.bg = nil
		case 58:
			s.underlineColor = readExtendedColor(groups, &i)
		case 59:
			s.underlineColor = nil
		case 90, 91, 92, 93, 94, 95, 96, 97:
			s.fg, _ = termcolor.NamedFromIndex(uint8(mainParam(g) - 90 + 8))
		case 100, 101, 102, 103, 104, 105, 106, 107:
			s.bg, _ = termcolor.NamedFromIndex(uint8(mainParam(g) - 100 + 8))
		}
	}
}

func mainParam(g ansi.Group) int {
	if v, ok := g.First(); ok {
		return v
	}
	return 0
}

func nthOrZero(g ansi.Group, n int) int {
	if v, ok := g.Nth(n); ok {
		return v
	}
	return 0
}

func underlineFromValue(v int) Underline {
	switch v {
	case 0:
		return UnderlineNone
	case 2:
		return UnderlineDouble
	case 3:
		return UnderlineCurly
	case 4:
		return UnderlineDotted
	case 5:
		return UnderlineDashed
	default:
		return UnderlineSingle
	}
}

func readExtendedColor(groups []ansi.Group, i *int) imagecolor.Color {
	g := groups[*i]
	if g.Len() > 1 {
		switch nthOrZero(g, 1) {
		case 5:
			return indexToColor(uint8(nthOrZero(g, 2)))
		case 2:
			if g.Len() >= 6 {
				return termcolor.RGB{R: uint8(nthOrZero(g, 3)), G: uint8(nthOrZero(g, 4)), B: uint8(nthOrZero(g, 5))}
			}
			return termcolor.RGB{R: uint8(nthOrZero(g, 2)), G: uint8(nthOrZero(g, 3)), B: uint8(nthOrZero(g, 4))}
		}
		return nil
	}

	(*i)++
	if *i >= len(groups) {
		return nil
	}
	switch mainParam(groups[*i]) {
	case 5:
		(*i)++
		if *i >= len(groups) {
			return indexToColor(0)
		}
		return indexToColor(uint8(mainParam(groups[*i])))
	case 2:
		return termcolor.RGB{R: uint8(nextParam(groups, i)), G: uint8(nextParam(groups, i)), B: uint8(nextParam(groups, i))}
	default:
		return nil
	}
}

func nextParam(groups []ansi.Group, i *int) int {
	(*i)++
	if *i >= len(groups) {
		return 0
	}
	return mainParam(groups[*i])
}

func indexToColor(n uint8) imagecolor.Color {
	if c, ok := termcolor.NamedFromIndex(n); ok {
		return c
	}
	return termcolor.Indexed(n)
}
