package ansi

import "strconv"

// Mode is an ANSI or DEC private terminal mode.
type Mode struct {
	dec bool
	n   int
}

// AnsiMode returns a standard ANSI mode number.
func AnsiMode(n int) Mode { return Mode{n: n} }

// DecMode returns a DEC private mode number.
func DecMode(n int) Mode { return Mode{dec: true, n: n} }

// Number returns the mode number without its ANSI/DEC private prefix.
func (m Mode) Number() int { return m.n }

// IsDEC reports whether the mode is a DEC private mode.
func (m Mode) IsDEC() bool { return m.dec }

// Well-known DEC private modes.
var (
	CursorKeysMode          = DecMode(1)
	ANSIVT52Mode            = DecMode(2)
	Column132Mode           = DecMode(3)
	SmoothScrollMode        = DecMode(4)
	ReverseVideoMode        = DecMode(5)
	OriginMode              = DecMode(6)
	AutoWrapMode            = DecMode(7)
	AutoRepeatMode          = DecMode(8)
	MouseX10Mode            = DecMode(9)
	PrintFormFeedMode       = DecMode(18)
	PrintExtentMode         = DecMode(19)
	CursorVisibleMode       = DecMode(25)
	NoClearColumnMode       = DecMode(40)
	NumericKeypadMode       = DecMode(66)
	BackarrowKeyMode        = DecMode(67)
	LeftRightMarginMode     = DecMode(69)
	AltScreenLegacyMode     = DecMode(47)
	MouseNormalMode         = DecMode(1000)
	MouseHighlightMode      = DecMode(1001)
	MouseButtonMode         = DecMode(1002)
	MouseAnyMode            = DecMode(1003)
	FocusMode               = DecMode(1004)
	MouseUTF8Mode           = DecMode(1005)
	MouseSGRMode            = DecMode(1006)
	MouseURXVTMode          = DecMode(1015)
	MouseSGRPixelMode       = DecMode(1016)
	AltScreenMode           = DecMode(1047)
	SaveCursorMode          = DecMode(1048)
	AltScreenSaveCursorMode = DecMode(1049)
	BracketedPasteMode      = DecMode(2004)
	SynchronizedOutputMode  = DecMode(2026)
	UnicodeCoreMode         = DecMode(2027)
	LightDarkMode           = DecMode(2031)
	VisibilityReportsMode   = DecMode(2033)
	InBandResizeMode        = DecMode(2048)
	Win32InputMode          = DecMode(9001)
)

// Well-known ANSI modes.
var (
	KeyboardActionMode  = AnsiMode(2)
	InsertMode          = AnsiMode(4)
	BidiSupportMode     = AnsiMode(8)
	SendReceiveMode     = AnsiMode(12)
	LineFeedNewLineMode = AnsiMode(20)
)

// ModeSetting is a decoded mode-report status value.
type ModeSetting uint8

const (
	ModeNotRecognized ModeSetting = iota
	ModeSet
	ModeReset
	ModePermanentlySet
	ModePermanentlyReset
)

// ModeSettingFromValue decodes the numeric report status value.
func ModeSettingFromValue(v int) ModeSetting {
	switch v {
	case 1:
		return ModeSet
	case 2:
		return ModeReset
	case 3:
		return ModePermanentlySet
	case 4:
		return ModePermanentlyReset
	default:
		return ModeNotRecognized
	}
}

// Value returns the numeric status value used on the wire.
func (s ModeSetting) Value() int { return int(s) }

// IsSet reports whether the mode is set or permanently set.
func (s ModeSetting) IsSet() bool { return s == ModeSet || s == ModePermanentlySet }

// IsReset reports whether the mode is reset or permanently reset.
func (s ModeSetting) IsReset() bool { return s == ModeReset || s == ModePermanentlyReset }

// IsRecognized reports whether the terminal recognized the mode.
func (s ModeSetting) IsRecognized() bool { return s != ModeNotRecognized }

// IsPermanent reports whether the mode cannot be toggled.
func (s ModeSetting) IsPermanent() bool { return s == ModePermanentlySet || s == ModePermanentlyReset }

// IsAvailable reports whether the mode can be used by the host.
func (s ModeSetting) IsAvailable() bool {
	return s == ModeSet || s == ModeReset || s == ModePermanentlySet
}

func (s ModeSetting) String() string {
	switch s {
	case ModeSet:
		return "set"
	case ModeReset:
		return "reset"
	case ModePermanentlySet:
		return "permanently set"
	case ModePermanentlyReset:
		return "permanently reset"
	default:
		return "not recognized"
	}
}

// SetMode sets one or more modes, splitting DEC and ANSI modes because their prefixes differ.
func SetMode(modes ...Mode) string { return modeSeq('h', modes) }

// ResetMode resets one or more modes, splitting DEC and ANSI modes because their prefixes differ.
func ResetMode(modes ...Mode) string { return modeSeq('l', modes) }

// Set returns this mode's set sequence.
func (m Mode) Set() string { return SetMode(m) }

// Reset returns this mode's reset sequence.
func (m Mode) Reset() string { return ResetMode(m) }

// Request returns this mode's state request sequence.
func (m Mode) Request() string { return RequestMode(m) }

func modeSeq(final byte, modes []Mode) string {
	if len(modes) == 0 {
		return ""
	}
	var buf seqBuf
	return string(appendModeSeq(buf[:0], final, modes))
}

// RequestMode requests a mode's current state.
func RequestMode(m Mode) string {
	if m.dec {
		return "\x1b[?" + strconv.Itoa(m.n) + "$p"
	}
	return "\x1b[" + strconv.Itoa(m.n) + "$p"
}

// ReportMode writes a mode-report response.
func ReportMode(m Mode, setting ModeSetting) string {
	if m.dec {
		return "\x1b[?" + strconv.Itoa(m.n) + ";" + strconv.Itoa(setting.Value()) + "$y"
	}
	return "\x1b[" + strconv.Itoa(m.n) + ";" + strconv.Itoa(setting.Value()) + "$y"
}
