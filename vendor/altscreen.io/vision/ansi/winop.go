package ansi

import (
	"strconv"
	"strings"
)

const (
	WindowOpResizeWindow        = 4
	WindowOpRequestWindowSize   = 14
	WindowOpRequestCellSize     = 16
	WindowOpRequestTextAreaSize = 18
)

const (
	RequestWindowPixelSize = "\x1b[14t"
	RequestCellPixelSize   = "\x1b[16t"
)

// WindowOp encodes an XTWINOPS sequence. Operation 0 emits nothing.
func WindowOp(p int, ps ...int) string {
	if p == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(p))
	for _, v := range ps {
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(v))
	}
	b.WriteByte('t')
	return b.String()
}
