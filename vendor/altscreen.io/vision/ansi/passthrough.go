package ansi

import "strings"

// ScreenPassthrough wraps seq in one or more DCS passthrough strings for screen-style forwarding.
func ScreenPassthrough(seq string, limit int) string {
	var b strings.Builder
	b.WriteString("\x1bP")
	if limit > 0 {
		for i := 0; i < len(seq); i += limit {
			end := i + limit
			if end > len(seq) {
				end = len(seq)
			}
			b.WriteString(seq[i:end])
			if end < len(seq) {
				b.WriteString("\x1b\\\x1bP")
			}
		}
	} else {
		b.WriteString(seq)
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// TmuxPassthrough wraps seq in a tmux passthrough frame, doubling literal ESC bytes inside seq.
func TmuxPassthrough(seq string) string {
	var b strings.Builder
	b.WriteString("\x1bPtmux;")
	for i := 0; i < len(seq); i++ {
		if seq[i] == ESC {
			b.WriteByte(ESC)
		}
		b.WriteByte(seq[i])
	}
	b.WriteString("\x1b\\")
	return b.String()
}
