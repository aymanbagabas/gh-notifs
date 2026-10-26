package ansi

import (
	"strconv"
	"strings"
)

// Sixel frames a Sixel payload as ESC P p1 ; p2 [ ; p3 ] q payload ESC \.
func Sixel(p1, p2, p3 int, payload string) string {
	var b strings.Builder
	b.WriteString("\x1bP")
	if p1 >= 0 {
		b.WriteString(strconv.Itoa(p1))
	}
	b.WriteByte(';')
	if p2 >= 0 {
		b.WriteString(strconv.Itoa(p2))
	}
	if p3 > 0 {
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(p3))
	}
	b.WriteByte('q')
	b.WriteString(payload)
	b.WriteString("\x1b\\")
	return b.String()
}

// KittyGraphics frames a graphics payload as ESC _ G options [ ; payload ] ESC \.
func KittyGraphics(options []string, payload string) string {
	out := "\x1b_G" + strings.Join(options, ",")
	if payload != "" {
		out += ";" + payload
	}
	return out + "\x1b\\"
}
