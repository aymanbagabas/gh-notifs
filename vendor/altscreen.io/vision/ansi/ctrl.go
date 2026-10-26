package ansi

// Terminal-wide controls: reset, device attributes, and version.
//
// None of these need a mode enabled. RIS is destructive: it asks the terminal
// to discard modes, colors, tabs, and character sets.
const (
	// RIS resets the terminal to its initial state: ESC c. This is a hard
	// reset, not an SGR reset or a screen clear.
	RIS = "\x1bc"

	// RequestPrimaryDeviceAttributes asks what the terminal is: ESC [ c.
	// The reply is a DA1 report listing the features it claims.
	RequestPrimaryDeviceAttributes = "\x1b[c"
	// RequestPrimaryDA is a shorthand for
	// [RequestPrimaryDeviceAttributes].
	RequestPrimaryDA = RequestPrimaryDeviceAttributes

	// RequestSecondaryDeviceAttributes asks for the terminal's identity and
	// firmware version: ESC [ > c.
	RequestSecondaryDeviceAttributes = "\x1b[>c"
	// RequestSecondaryDA is a shorthand for
	// [RequestSecondaryDeviceAttributes].
	RequestSecondaryDA = RequestSecondaryDeviceAttributes

	// RequestTertiaryDeviceAttributes asks for the terminal's unit ID:
	// ESC [ = c.
	RequestTertiaryDeviceAttributes = "\x1b[=c"
	// RequestTertiaryDA is a shorthand for
	// [RequestTertiaryDeviceAttributes].
	RequestTertiaryDA = RequestTertiaryDeviceAttributes

	// RequestXTVersion asks for the terminal's name and version: ESC [ > q.
	// The reply is a DCS carrying a string such as "XTerm(380)".
	RequestXTVersion = "\x1b[>q"
)
