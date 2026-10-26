package event

// DecoderFlags chooses how the ambiguous legacy keys are read.
//
// A terminal using its legacy encoding sends one control byte for a key and
// for the Ctrl combination that collides with it, and which of the two a
// reader meant is not in the bytes:
//
//	0x09      tab or ctrl+i
//	0x0a      ctrl+j or enter
//	0x08      ctrl+h, backspace, or ctrl+backspace
//	0x0d      enter or ctrl+m
//	0x00      ctrl+space or ctrl+@
//	0x1b      escape or ctrl+[ , when it stands alone
//	0x7f      backspace or delete
//	CSI 1 ~   home or the VT220 find
//	CSI 4 ~   end or the VT220 select
//
// With Backarrow mode reset, the zero value reads BS as Ctrl+H, LF as Ctrl+J,
// and DEL as Backspace. With Backarrow mode set, BS becomes Backspace and DEL
// becomes Ctrl+Backspace. An ESC prefix adds Alt to the selected key.
// An application that wants another interpretation uses
// [Source.WithDecoderFlags] when it builds its own source or
// [altscreen.io/vision/program.Options] when it takes a program's.
//
// A flag decides how a byte is named, not how it is framed, so changing one on
// a source already in use leaves buffered input alone.
type DecoderFlags uint16

const (
	// DecoderFlagCtrlAt reports 0x00 as ctrl+@ instead of ctrl+space.
	DecoderFlagCtrlAt DecoderFlags = 1 << iota
	// DecoderFlagCtrlI reports 0x09 as ctrl+i instead of tab.
	DecoderFlagCtrlI
	// DecoderFlagCtrlM reports 0x0d as ctrl+m instead of enter.
	DecoderFlagCtrlM
	// DecoderFlagCtrlOpenBracket reports a lone 0x1b as ctrl+[ instead of
	// escape, wherever that byte resolves as a key of its own: a source
	// reaching its escape deadline, whether the byte stood alone or headed a
	// sequence that never finished, and the inner one of a run of them, which
	// reads as ctrl+alt+[.
	DecoderFlagCtrlOpenBracket
	// DecoderFlagDELIsDelete reports 0x7f as Delete in either Backarrow mode.
	DecoderFlagDELIsDelete
	// DecoderFlagFindKey reports CSI 1 ~ as the VT220 find key instead of
	// home.
	DecoderFlagFindKey
	// DecoderFlagSelectKey reports CSI 4 ~ as the VT220 select key instead of
	// end.
	DecoderFlagSelectKey
	// DecoderFlagLFIsEnter reports LF as Enter instead of Ctrl+J.
	// CR retains its independent interpretation through DecoderFlagCtrlM.
	DecoderFlagLFIsEnter
	// DecoderFlagCtrlH reports BS as Ctrl+H in either Backarrow mode.
	// It takes precedence over DecoderFlagCtrlBackspace.
	DecoderFlagCtrlH
	// DecoderFlagCtrlBackspace reports BS as Ctrl+Backspace when Backarrow mode is reset.
	// When the mode is set, BS remains Backspace and DEL becomes Ctrl+Backspace.
	DecoderFlagCtrlBackspace
)

// Contains reports whether every flag in g is set in f.
func (f DecoderFlags) Contains(g DecoderFlags) bool { return f&g == g }

// Set returns f with the flags in g set.
func (f DecoderFlags) Set(g DecoderFlags) DecoderFlags { return f | g }

// Clear returns f with the flags in g cleared.
func (f DecoderFlags) Clear(g DecoderFlags) DecoderFlags { return f &^ g }
