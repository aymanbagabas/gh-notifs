package ansi

import "encoding/base64"

// SystemClipboard is the OSC 52 selector byte c for the system clipboard.
const SystemClipboard = 'c'

// PrimaryClipboard is the OSC 52 selector byte p for the primary selection.
const PrimaryClipboard = 'p'

// SetClipboard sets a clipboard or selection with OSC 52: ESC ] 52 ; pc ; base64-data BEL.
func SetClipboard(pc byte, data []byte) string {
	return "\x1b]52;" + string(pc) + ";" + base64Encode(data) + "\x07"
}

// RequestClipboard requests clipboard contents with OSC 52: ESC ] 52 ; pc ; ? BEL.
func RequestClipboard(pc byte) string {
	return "\x1b]52;" + string(pc) + ";?\x07"
}

// ClearClipboard clears a clipboard or selection with OSC 52: ESC ] 52 ; pc ; BEL.
func ClearClipboard(pc byte) string {
	return "\x1b]52;" + string(pc) + ";\x07"
}

func base64Encode(data []byte) string { return base64.StdEncoding.EncodeToString(data) }
