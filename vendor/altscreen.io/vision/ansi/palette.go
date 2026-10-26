package ansi

// ResetPalette resets the Linux-console palette: ESC ] R BEL.
const ResetPalette = "\x1b]R\x07"

// SetPalette sets a 16-color palette entry with OSC P: ESC ] P index rrggbb BEL.
func SetPalette(index, r, g, b int) string {
	if index < 0 || index > 15 {
		return ""
	}
	return "\x1b]P" + string(hexLower[index]) + hexByte(r) + hexByte(g) + hexByte(b) + "\x07"
}

const hexLower = "0123456789abcdef"

func hexByte(v int) string {
	b := byte(v)
	return string([]byte{hexLower[b>>4], hexLower[b&0x0f]})
}
