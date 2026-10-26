package ansi

// SelectCharset designates a character set with ESC gset charset.
func SelectCharset(gset, charset byte) string { return string([]byte{0x1b, gset, charset}) }

// LS1R locks G1 into GR: ESC ~.
const LS1R = "\x1b~"

// LS2 locks G2 into GL: ESC n.
const LS2 = "\x1bn"

// LS2R locks G2 into GR: ESC }.
const LS2R = "\x1b}"

// LS3 locks G3 into GL: ESC o.
const LS3 = "\x1bo"

// LS3R locks G3 into GR: ESC |.
const LS3R = "\x1b|"
