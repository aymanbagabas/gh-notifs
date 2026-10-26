package ansi

// C1 control bytes (0x80..=0x9f).
//
// C1 controls are the 8-bit single-byte forms of terminal controls such as
// DCS, CSI, ST, OSC, PM, and APC. Each has a 7-bit fallback spelling of ESC
// followed by byte-0x40: CSI is 0x9b, and the 7-bit introducer is ESC [.

const (
	// PAD is the padding Character C1 control byte 0x80.
	PAD = 0x80
	// HOP is the high Octet Preset C1 control byte 0x81.
	HOP = 0x81
	// BPH is the break Permitted Here C1 control byte 0x82.
	BPH = 0x82
	// NBH is the no Break Here C1 control byte 0x83.
	NBH = 0x83
	// IND is the index C1 control byte 0x84; 7-bit spelling is ESC D.
	IND = 0x84
	// NEL is the next Line C1 control byte 0x85; 7-bit spelling is ESC E.
	NEL = 0x85
	// SSA is the start of Selected Area C1 control byte 0x86.
	SSA = 0x86
	// ESA is the end of Selected Area C1 control byte 0x87.
	ESA = 0x87
	// HTS is the horizontal Tab Set C1 control byte 0x88; 7-bit spelling is ESC H.
	HTS = 0x88
	// HTJ is the horizontal Tab with Justify C1 control byte 0x89.
	HTJ = 0x89
	// VTS is the vertical Tab Set C1 control byte 0x8a.
	VTS = 0x8A
	// PLD is the partial Line Down C1 control byte 0x8b.
	PLD = 0x8B
	// PLU is the partial Line Up C1 control byte 0x8c.
	PLU = 0x8C
	// RI is the reverse Index C1 control byte 0x8d; 7-bit spelling is ESC M.
	RI = 0x8D
	// SS2 is the single Shift 2 C1 control byte 0x8e; 7-bit spelling is ESC N.
	SS2 = 0x8E
	// SS3 is the single Shift 3 C1 control byte 0x8f; 7-bit spelling is ESC O.
	SS3 = 0x8F
	// DCS is the device Control String introducer byte 0x90; 7-bit spelling is ESC P and terminator is [ST].
	DCS = 0x90
	// PU1 is the private Use 1 C1 control byte 0x91.
	PU1 = 0x91
	// PU2 is the private Use 2 C1 control byte 0x92.
	PU2 = 0x92
	// STS is the set Transmit State C1 control byte 0x93.
	STS = 0x93
	// CCH is the cancel Character C1 control byte 0x94.
	CCH = 0x94
	// MW is the message Waiting C1 control byte 0x95.
	MW = 0x95
	// SPA is the start of Protected Area C1 control byte 0x96.
	SPA = 0x96
	// EPA is the end of Protected Area C1 control byte 0x97.
	EPA = 0x97
	// SOS is the start of String introducer byte 0x98; 7-bit spelling is ESC X.
	SOS = 0x98
	// SGCI is the single Graphic Character Introducer C1 control byte 0x99.
	SGCI = 0x99
	// SCI is the single Character Introducer C1 control byte 0x9a.
	SCI = 0x9A
	// CSI is the control Sequence Introducer byte 0x9b; 7-bit spelling is ESC [.
	CSI = 0x9B
	// ST is the string Terminator byte 0x9c; 7-bit spelling is ESC \.
	ST = 0x9C
	// OSC is the operating System Command introducer byte 0x9d; 7-bit spelling is ESC ].
	OSC = 0x9D
	// PM is the privacy Message introducer byte 0x9e; 7-bit spelling is ESC ^.
	PM = 0x9E
	// APC is the application Program Command introducer byte 0x9f; 7-bit spelling is ESC _.
	APC = 0x9F
)
