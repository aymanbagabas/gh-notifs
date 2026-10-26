package ansi

import "strings"

// URXVTExt invokes an OSC 777 extension with ESC ] 777 ; extension [;param...] BEL.
func URXVTExt(extension string, params ...string) string {
	var b strings.Builder
	b.WriteString("\x1b]777;")
	b.WriteString(extension)
	for _, p := range params {
		b.WriteByte(';')
		b.WriteString(p)
	}
	b.WriteByte('\x07')
	return b.String()
}
