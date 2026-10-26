package ansi

// Truncate shortens s to at most length display columns, appending tail if needed.
func Truncate(s string, length int, tail string) string {
	return TruncateMode(s, length, tail, WcWidth, false)
}

// TruncateMode is Truncate with explicit width measurement options.
func TruncateMode(s string, length int, tail string, mode WidthMethod, ambiguousWide bool) string {
	if length == 0 {
		return ""
	}
	if StringWidth([]byte(s), mode, ambiguousWide) <= length {
		return s
	}
	tailWidth := StringWidth([]byte(tail), mode, ambiguousWide)
	budget := length - tailWidth
	if budget < 0 {
		budget = 0
	}
	out := ""
	used := 0
	tailInserted := false
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
			if !tailInserted {
				out += string([]byte{tok.Control})
			}
		case TokenText:
			if used+tok.Width > budget {
				if !tailInserted {
					out += tail
					tailInserted = true
				}
				continue
			}
			out += tokenBytesString(tok.Bytes)
			used += tok.Width
		}
	}
	if !tailInserted {
		out += tail
	}
	return out
}

// TruncateLeft shortens s from the left until length display columns remain.
func TruncateLeft(s string, length int, prefix string) string {
	return TruncateLeftMode(s, length, prefix, WcWidth, false)
}

// TruncateLeftMode is TruncateLeft with explicit width measurement options.
func TruncateLeftMode(s string, length int, prefix string, mode WidthMethod, ambiguousWide bool) string {
	total := StringWidth([]byte(s), mode, ambiguousWide)
	if total <= length {
		return s
	}
	prefixWidth := StringWidth([]byte(prefix), mode, ambiguousWide)
	drop := total - (length - prefixWidth)
	if drop < 0 {
		drop = 0
	}
	headEscapes := ""
	out := ""
	dropped := 0
	dropping := true
	tokens := Tokenize([]byte(s), mode, ambiguousWide)
	for {
		tok, ok := tokens.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case TokenEscape:
			if dropping {
				headEscapes += tokenBytesString(tok.Bytes)
			} else {
				out += tokenBytesString(tok.Bytes)
			}
		case TokenControl:
			if !dropping {
				out += string([]byte{tok.Control})
			}
		case TokenText:
			if dropping {
				dropped += tok.Width
				if dropped >= drop {
					dropping = false
				}
			} else {
				out += tokenBytesString(tok.Bytes)
			}
		}
	}
	return headEscapes + prefix + out
}

// Cut removes left display columns from the start and right columns from the end of s.
func Cut(s string, left, right int) string {
	return CutMode(s, left, right, WcWidth, false)
}

// CutMode is Cut with explicit width measurement options.
func CutMode(s string, left, right int, mode WidthMethod, ambiguousWide bool) string {
	if left == 0 && right == 0 {
		return s
	}
	total := StringWidth([]byte(s), mode, ambiguousWide)
	if left >= total {
		return ""
	}
	keepEnd := total - right
	if keepEnd < 0 {
		keepEnd = 0
	}
	if keepEnd <= left {
		return ""
	}
	target := keepEnd - left
	out := ""
	col := 0
	emitted := 0
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
			if col >= left && emitted < target {
				out += string([]byte{tok.Control})
			}
		case TokenText:
			if col >= left && emitted+tok.Width <= target {
				out += tokenBytesString(tok.Bytes)
				emitted += tok.Width
			}
			col += tok.Width
		}
	}
	return out
}
