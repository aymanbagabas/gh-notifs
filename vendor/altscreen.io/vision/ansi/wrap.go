package ansi

import "sort"

// DefaultBreakpoints are the default word wrap break characters.
const DefaultBreakpoints = "-,.;: "

// Hardwrap wraps s so no visible line exceeds limit columns.
func Hardwrap(s string, limit int, preserveSpace bool) string {
	return HardwrapMode(s, limit, preserveSpace, WcWidth, false)
}

// HardwrapMode is Hardwrap with explicit width measurement options.
func HardwrapMode(s string, limit int, preserveSpace bool, mode WidthMethod, ambiguousWide bool) string {
	if limit == 0 {
		return s
	}
	out := ""
	col := 0
	tokens := Tokenize([]byte(s), mode, ambiguousWide)
	for {
		tok, ok := tokens.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case TokenEscape:
			out += tokenBytesString(tok.Bytes)
		case TokenControl:
			if tok.Control == '\n' {
				out += "\n"
				col = 0
			} else {
				out += string([]byte{tok.Control})
			}
		case TokenText:
			if tok.Width == 0 {
				out += tokenBytesString(tok.Bytes)
				continue
			}
			if col+tok.Width > limit {
				out += "\n"
				col = 0
				if !preserveSpace && string(tok.Bytes) == " " {
					continue
				}
			}
			out += tokenBytesString(tok.Bytes)
			col += tok.Width
		}
	}
	return out
}

// Wordwrap wraps s at breakpoints where possible.
func Wordwrap(s string, limit int, breakpoints string) string {
	return WordwrapMode(s, limit, breakpoints, WcWidth, false)
}

// WordwrapMode is Wordwrap with explicit width measurement options.
func WordwrapMode(s string, limit int, breakpoints string, mode WidthMethod, ambiguousWide bool) string {
	wrapped, _ := wordwrapInner(s, limit, breakpoints, mode, ambiguousWide)
	return wrapped
}

func wordwrapInner(s string, limit int, breakpoints string, mode WidthMethod, ambiguousWide bool) (string, bool) {
	if limit == 0 {
		return s, false
	}
	var ascii [128]bool
	wide := make([]rune, 0)
	for _, r := range breakpoints {
		if r < 128 {
			ascii[r] = true
		} else {
			wide = append(wide, r)
		}
	}
	sort.Slice(wide, func(i, j int) bool { return wide[i] < wide[j] })
	isBreak := func(s string) bool {
		for _, r := range s {
			if r < 128 {
				if !ascii[r] {
					return false
				}
			} else {
				i := sort.Search(len(wide), func(i int) bool { return wide[i] >= r })
				if i == len(wide) || wide[i] != r {
					return false
				}
			}
		}
		return true
	}
	out, line, word, space := "", "", "", ""
	lineWidth, wordWidth, spaceWidth := 0, 0, 0
	over := false
	flushWord := func() {
		if word == "" {
			return
		}
		line += space + word
		lineWidth += spaceWidth + wordWidth
		if lineWidth > limit {
			over = true
		}
		word, space = "", ""
		wordWidth, spaceWidth = 0, 0
	}
	tokens := Tokenize([]byte(s), mode, ambiguousWide)
	for {
		tok, ok := tokens.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case TokenEscape:
			esc := tokenBytesString(tok.Bytes)
			if word != "" {
				word += esc
			} else if space != "" {
				space += esc
			} else {
				line += esc
			}
		case TokenControl:
			if tok.Control == '\n' {
				flushWord()
				out += line + "\n"
				line, space = "", ""
				lineWidth, spaceWidth = 0, 0
			} else {
				word += string([]byte{tok.Control})
			}
		case TokenText:
			text := tokenBytesString(tok.Bytes)
			isSpace := text == " " || text == "\t"
			if isSpace {
				if word != "" {
					if lineWidth > 0 && lineWidth+spaceWidth+wordWidth > limit {
						out += line + "\n"
						line, space = "", ""
						lineWidth, spaceWidth = 0, 0
					}
					flushWord()
				}
				space += text
				spaceWidth += tok.Width
			} else if isBreak(text) {
				word += text
				wordWidth += tok.Width
				if lineWidth > 0 && lineWidth+spaceWidth+wordWidth > limit {
					out += line + "\n"
					line, space = "", ""
					lineWidth, spaceWidth = 0, 0
				}
				flushWord()
			} else {
				word += text
				wordWidth += tok.Width
			}
		}
	}
	if word != "" {
		if lineWidth > 0 && lineWidth+spaceWidth+wordWidth > limit {
			out += line + "\n"
			line, space = "", ""
			lineWidth, spaceWidth = 0, 0
		}
		flushWord()
	} else if space != "" {
		line += space
		if lineWidth+spaceWidth > limit {
			over = true
		}
	}
	return out + line, over
}

// Wrap soft-wraps s, then hard-wraps any remaining overlong line.
func Wrap(s string, limit int, breakpoints string) string {
	return WrapMode(s, limit, breakpoints, WcWidth, false)
}

// WrapMode is Wrap with explicit width measurement options.
func WrapMode(s string, limit int, breakpoints string, mode WidthMethod, ambiguousWide bool) string {
	if limit == 0 {
		return s
	}
	wrapped, over := wordwrapInner(s, limit, breakpoints, mode, ambiguousWide)
	if !over {
		return wrapped
	}
	out := ""
	start := 0
	changed := false
	for i := 0; ; i++ {
		if i < len(wrapped) && wrapped[i] != '\n' {
			continue
		}
		line := wrapped[start:i]
		if StringWidth([]byte(line), mode, ambiguousWide) > limit {
			out += HardwrapMode(line, limit, false, mode, ambiguousWide)
			changed = true
		} else {
			out += line
		}
		if i == len(wrapped) {
			break
		}
		out += "\n"
		start = i + 1
	}
	if !changed {
		return wrapped
	}
	return out
}
