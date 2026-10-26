package renderer

import (
	"image/color"

	termcolor "altscreen.io/vision/color"
	"altscreen.io/vision/style"
)

type colorCache struct {
	profile termcolor.Profile
	cache   map[termcolor.RGB]color.Color
}

func newColorCache(profile termcolor.Profile) colorCache {
	return colorCache{profile: profile, cache: map[termcolor.RGB]color.Color{}}
}
func (c *colorCache) Profile() termcolor.Profile { return c.profile }
func (c *colorCache) SetProfile(p termcolor.Profile) {
	if c.profile != p {
		c.profile = p
		clear(c.cache)
	}
}

func (c *colorCache) Convert(col color.Color) color.Color {
	if col == nil {
		return nil
	}
	switch c.profile {
	case termcolor.Disabled, termcolor.Ascii:
		return nil
	case termcolor.TrueColor:
		return col
	case termcolor.Ansi, termcolor.Ansi256:
		rgb := termcolor.ToRGB(col)
		if out, ok := c.cache[rgb]; ok {
			return out
		}
		out := c.profile.Convert(col)
		c.cache[rgb] = out
		return out
	default:
		return c.profile.Convert(col)
	}
}

func (c *colorCache) ConvertStyle(st style.Style) style.Style {
	switch c.profile {
	case termcolor.TrueColor:
		return st
	case termcolor.Disabled:
		return style.Empty
	case termcolor.Ascii:
		return st.Foreground(nil).Background(nil).UnderlineColor(nil)
	default:
		return st.Foreground(c.Convert(st.GetForeground())).
			Background(c.Convert(st.GetBackground())).
			UnderlineColor(c.Convert(st.GetUnderlineColor()))
	}
}

func (c *colorCache) Len() int { return len(c.cache) }
