package ansi

// C0 control bytes (0x00..=0x1f).
//
// These are the 7-bit single-byte control codes shared with ASCII.

const (
	// NUL is the null control byte 0x00; used as a padding or string terminator in some byte protocols.
	NUL = 0x00
	// SOH is the start of Heading control byte 0x01.
	SOH = 0x01
	// STX is the start of Text control byte 0x02.
	STX = 0x02
	// ETX is the end of Text control byte 0x03.
	ETX = 0x03
	// EOT is the end of Transmission control byte 0x04.
	EOT = 0x04
	// ENQ is the enquiry control byte 0x05; historically asks for terminal identification.
	ENQ = 0x05
	// ACK is the acknowledge control byte 0x06.
	ACK = 0x06
	// BEL is the bell control byte 0x07; also terminates many OSC strings emitted by this crate.
	BEL = 0x07
	// BS is the backspace control byte 0x08; moves one column left in ordinary terminal output.
	BS = 0x08
	// HT is the horizontal Tab control byte 0x09; advances to the next tab stop.
	HT = 0x09
	// LF is the line Feed control byte 0x0a; moves down one line, with carriage behavior depending on terminal mode.
	LF = 0x0A
	// VT is the vertical Tab control byte 0x0b.
	VT = 0x0B
	// FF is the form Feed control byte 0x0c.
	FF = 0x0C
	// CR is the carriage Return control byte 0x0d; moves to the start of the current line.
	CR = 0x0D
	// SO is the shift Out control byte 0x0e; invokes G1 into GL until shifted back.
	SO = 0x0E
	// SI is the shift In control byte 0x0f; invokes G0 into GL.
	SI = 0x0F
	// DLE is the data Link Escape control byte 0x10.
	DLE = 0x10
	// DC1 is the device Control 1 byte 0x11; commonly XON in software flow control.
	DC1 = 0x11
	// DC2 is the device Control 2 byte 0x12.
	DC2 = 0x12
	// DC3 is the device Control 3 byte 0x13; commonly XOFF in software flow control.
	DC3 = 0x13
	// DC4 is the device Control 4 byte 0x14.
	DC4 = 0x14
	// NAK is the negative Acknowledge control byte 0x15.
	NAK = 0x15
	// SYN is the synchronous Idle control byte 0x16.
	SYN = 0x16
	// ETB is the end of Transmission Block control byte 0x17.
	ETB = 0x17
	// CAN is the cancel control byte 0x18; often aborts an in-progress escape sequence.
	CAN = 0x18
	// EM is the end of Medium control byte 0x19.
	EM = 0x19
	// SUB is the substitute control byte 0x1a; may also abort an in-progress escape sequence.
	SUB = 0x1A
	// ESC is the escape control byte 0x1b; introduces 7-bit escape sequences such as CSI (ESC [) and OSC (ESC ]).
	ESC = 0x1B
	// FS is the file Separator control byte 0x1c.
	FS = 0x1C
	// GS is the group Separator control byte 0x1d.
	GS = 0x1D
	// RS is the record Separator control byte 0x1e.
	RS = 0x1E
	// US is the unit Separator control byte 0x1f.
	US = 0x1F
)
