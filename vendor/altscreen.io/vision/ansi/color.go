package ansi

import "strconv"

// SetForegroundColor sets the default text foreground color with OSC 10.
func SetForegroundColor(color string) string { return "\x1b]10;" + color + "\x07" }

// SetBackgroundColor sets the default text background color with OSC 11.
func SetBackgroundColor(color string) string { return "\x1b]11;" + color + "\x07" }

// SetCursorColor sets the cursor color with OSC 12.
func SetCursorColor(color string) string { return "\x1b]12;" + color + "\x07" }

// SetPaletteColor sets one indexed palette entry with OSC 4.
func SetPaletteColor(index uint8, color string) string {
	return "\x1b]4;" + strconv.Itoa(int(index)) + ";" + color + "\x07"
}

const (
	RequestForegroundColor = "\x1b]10;?\x07"
	RequestBackgroundColor = "\x1b]11;?\x07"
	RequestCursorColor     = "\x1b]12;?\x07"
)

// RequestPaletteColor requests one indexed palette entry.
func RequestPaletteColor(index uint8) string {
	return "\x1b]4;" + strconv.Itoa(int(index)) + ";?\x07"
}

// ResetPaletteColor resets one indexed palette entry.
func ResetPaletteColor(index uint8) string {
	return "\x1b]104;" + strconv.Itoa(int(index)) + "\x07"
}

const (
	ResetPaletteColors   = "\x1b]104\x07"
	ResetForegroundColor = "\x1b]110\x07"
	ResetBackgroundColor = "\x1b]111\x07"
	ResetCursorColor     = "\x1b]112\x07"
)

// XParseRGB formats 8-bit RGB channels as rgb:RRRR/GGGG/BBBB.
func XParseRGB(r, g, b uint8) string {
	return "rgb:" + dupHex(r) + "/" + dupHex(g) + "/" + dupHex(b)
}

func dupHex(v uint8) string {
	const hexdigits = "0123456789abcdef"
	return string([]byte{hexdigits[v>>4], hexdigits[v&0xf], hexdigits[v>>4], hexdigits[v&0xf]})
}
