package ansi

import "strings"

// XTGETTCAP requests terminal capability values with DCS + q hex-caps ST.
func XTGETTCAP(caps ...string) string {
	if len(caps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\x1bP+q")
	for i, cap := range caps {
		if i > 0 {
			b.WriteByte(';')
		}
		for j := 0; j < len(cap); j++ {
			b.WriteByte(upperHex[cap[j]>>4])
			b.WriteByte(upperHex[cap[j]&0x0f])
		}
	}
	b.WriteString("\x1b\\")
	return b.String()
}

const upperHex = "0123456789ABCDEF"
