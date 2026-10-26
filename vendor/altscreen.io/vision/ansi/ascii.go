package ansi

// Printable ASCII boundaries, used to classify bytes while scanning.

const (
	// SP is the space byte 0x20, the first printable ASCII code point and the separator often used inside CSI parameter syntax as an intermediate byte.
	SP = 0x20
	// DEL is the delete control byte 0x7f, the last 7-bit ASCII byte. It is treated as a standalone control byte by the tokenizer.
	DEL = 0x7F
)
