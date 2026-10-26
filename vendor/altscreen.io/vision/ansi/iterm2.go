package ansi

// Iterm2 sends a generic OSC 1337 message with ESC ] 1337 ; payload BEL.
func Iterm2(payload string) string { return "\x1b]1337;" + payload + "\x07" }
