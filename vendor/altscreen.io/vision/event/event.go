package event

import (
	stdcolor "image/color"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/term"
)

// Event is any terminal event.
type Event interface{}

// KeyPressEvent is a key-press event.
type KeyPressEvent Key

// KeyRepeatEvent is a key-repeat event.
type KeyRepeatEvent Key

// KeyReleaseEvent is a key-release event.
type KeyReleaseEvent Key

// MouseClickEvent is a mouse-click event.
type MouseClickEvent Mouse

// MouseReleaseEvent is a mouse-release event.
type MouseReleaseEvent Mouse

// MouseWheelEvent is a mouse-wheel event.
type MouseWheelEvent Mouse

// MouseMoveEvent is a mouse-move event.
type MouseMoveEvent Mouse

// ResizeEvent reports a terminal surface size change.
type ResizeEvent term.Winsize

// WindowCellSizeEvent is a reply to a window-cell-size query.
type WindowCellSizeEvent struct {
	// Width is the width in terminal cells.
	Width int
	// Height is the height in terminal cells.
	Height int
}

// WindowPixelSizeEvent is a reply to a window-pixel-size query.
type WindowPixelSizeEvent struct {
	// Width is the width in pixels.
	Width int
	// Height is the height in pixels.
	Height int
}

// CellPixelSizeEvent is a reply to a cell-pixel-size query.
type CellPixelSizeEvent struct {
	// Width is the cell width in pixels.
	Width int
	// Height is the cell height in pixels.
	Height int
}

// FocusInEvent reports focus gained.
type FocusInEvent struct{}

// FocusOutEvent reports focus lost.
type FocusOutEvent struct{}

// PasteStartEvent reports bracketed paste start.
type PasteStartEvent struct{}

// PasteEndEvent reports bracketed paste end.
type PasteEndEvent struct{}

// PasteChunkEvent is a chunk of pasted bytes.
type PasteChunkEvent []byte

// CursorPositionEvent is a standard or extended cursor position report.
type CursorPositionEvent struct {
	// Position is the zero-based cursor position.
	layout.Position
	// Page is the reported one-based page number, or zero if absent.
	Page int
}

// PrimaryDeviceAttributesEvent reports primary device attributes.
type PrimaryDeviceAttributesEvent []OptionalInt

// SecondaryDeviceAttributesEvent reports secondary device attributes.
type SecondaryDeviceAttributesEvent []OptionalInt

// TertiaryDeviceAttributesEvent reports tertiary device attributes.
type TertiaryDeviceAttributesEvent string

// TerminalNameEvent reports the terminal name.
type TerminalNameEvent string

// ModeReportEvent reports a terminal mode setting.
type ModeReportEvent struct {
	// Mode is the reported mode.
	Mode ansi.Mode
	// Setting is the current mode setting.
	Setting ansi.ModeSetting
}

// ModifyOtherKeysEvent reports modifyOtherKeys mode.
type ModifyOtherKeysEvent ModifyOtherKeysMode

// KittyKeyboardEnhancementsEvent reports Kitty keyboard flags.
type KittyKeyboardEnhancementsEvent ansi.KittyKeyboardFlags

// WindowOpEvent reports a window operation reply.
type WindowOpEvent struct {
	// Op is the window operation number.
	Op int
	// Args are the operation arguments.
	Args []OptionalInt
}

// TermcapEvent reports XTGETTCAP capability entries.
type TermcapEvent struct {
	// Recognized reports whether the capabilities were recognized.
	Recognized bool
	// Entries are decoded name and optional value pairs.
	Entries []TermcapEntry
}

// TermcapEntry is a decoded termcap name and optional value.
type TermcapEntry struct {
	// Name is the capability name.
	Name string
	// Value is the capability value.
	Value string
	// HasValue reports whether Value is present.
	HasValue bool
}

// SettingReportEvent is a DECRPSS setting report.
type SettingReportEvent SettingReport

// ForegroundColorEvent reports the default foreground color.
//
// The color is a field rather than the event itself. Naming an interface type
// does not give it a distinct identity in a type switch, so three events
// defined directly over stdcolor.Color would all match the first case and the
// replies would be indistinguishable.
type ForegroundColorEvent struct {
	// Color is the reported color.
	Color stdcolor.Color
}

// BackgroundColorEvent reports the default background color.
type BackgroundColorEvent struct {
	// Color is the reported color.
	Color stdcolor.Color
}

// CursorColorEvent reports the cursor color.
type CursorColorEvent struct {
	// Color is the reported color.
	Color stdcolor.Color
}

// PaletteColorEvent reports an indexed palette color.
type PaletteColorEvent struct {
	// Index is the palette index.
	Index int
	// Color is the reported color.
	Color stdcolor.Color
}

// ColorSchemeEvent reports the terminal color scheme.
type ColorSchemeEvent ColorScheme

// VisibilityEvent reports terminal visibility.
type VisibilityEvent Visibility

// ClipboardEvent reports clipboard content.
type ClipboardEvent struct {
	// Selection is the clipboard selection.
	Selection ClipboardSelection
	// Content is the clipboard content.
	Content string
}

// ProgramStatusEvent reports accepted OSC 7501 state and kind names.
// It replies to [altscreen.io/vision/program.Program.RequestProgramStatus].
type ProgramStatusEvent struct {
	// Capabilities contains fixed defaults for absent fields and preserves
	// explicit empty lists and extension names.
	Capabilities ansi.ProgramStatusCapabilities
}

// KittyGraphicsEvent reports a Kitty graphics response.
type KittyGraphicsEvent struct {
	// Options are response options.
	Options []KeyValue
	// Payload is response payload bytes.
	Payload []byte
}

// MultiEvent is multiple events emitted by a single sequence.
type MultiEvent []Event

// UnknownCsiEvent is an unknown CSI sequence.
type UnknownCsiEvent []byte

// UnknownSs3Event is an unknown SS3 sequence.
type UnknownSs3Event []byte

// UnknownOscEvent is an unknown OSC sequence.
type UnknownOscEvent []byte

// UnknownDcsEvent is an unknown DCS sequence.
type UnknownDcsEvent []byte

// UnknownSosEvent is an unknown SOS sequence.
type UnknownSosEvent []byte

// UnknownPmEvent is an unknown PM sequence.
type UnknownPmEvent []byte

// UnknownApcEvent is an unknown APC sequence.
type UnknownApcEvent []byte

// UnknownEvent is an unrecognized byte sequence.
type UnknownEvent []byte

// OptionalInt is a parameter that may be absent. It is an alias for
// [ansi.OptionalInt], so parameters keep one spelling from the parser through
// to the events they produce.
type OptionalInt = ansi.OptionalInt

// KeyValue is a string key and value pair.
type KeyValue struct {
	// Key is the entry key.
	Key string
	// Value is the entry value.
	Value string
}

// ClipboardSelection identifies an OSC 52 clipboard selection.
type ClipboardSelection struct {
	// Kind is the selection kind.
	Kind ClipboardSelectionKind
	// Other is the unknown selection rune for ClipboardSelectionOther.
	Other rune
}

// ClipboardSelectionKind identifies a clipboard selection kind.
type ClipboardSelectionKind int

const (
	// ClipboardSelectionSystem is the system clipboard.
	ClipboardSelectionSystem ClipboardSelectionKind = iota
	// ClipboardSelectionPrimary is the primary selection.
	ClipboardSelectionPrimary
	// ClipboardSelectionOther is any other selection character.
	ClipboardSelectionOther
)

// ModifyOtherKeysMode is a decoded modifyOtherKeys mode.
type ModifyOtherKeysMode int

const (
	// ModifyOtherKeysDisabled means modifyOtherKeys is disabled.
	ModifyOtherKeysDisabled ModifyOtherKeysMode = iota
	// ModifyOtherKeysMode1 modifies keys without xterm sequences.
	ModifyOtherKeysMode1
	// ModifyOtherKeysMode2 modifies all keys.
	ModifyOtherKeysMode2
)

// ModifyOtherKeysModeFromValue converts a report value to a mode.
func ModifyOtherKeysModeFromValue(v int) ModifyOtherKeysMode {
	switch v {
	case 1:
		return ModifyOtherKeysMode1
	case 2:
		return ModifyOtherKeysMode2
	default:
		return ModifyOtherKeysDisabled
	}
}

// ColorScheme is a reported terminal color scheme.
type ColorScheme int

const (
	// ColorSchemeDark is dark mode.
	ColorSchemeDark ColorScheme = iota
	// ColorSchemeLight is light mode.
	ColorSchemeLight
)

// String returns the color scheme name.
func (s ColorScheme) String() string {
	if s == ColorSchemeLight {
		return "light"
	}
	return "dark"
}

// Visibility is reported terminal visibility.
type Visibility int

const (
	// VisibilityVisible means the terminal may be observable.
	VisibilityVisible Visibility = iota
	// VisibilityHidden means the terminal knows the view is not observable.
	VisibilityHidden
)

// IsVisible reports whether v may be observable.
func (v Visibility) IsVisible() bool { return v == VisibilityVisible }

// String returns the visibility name.
func (v Visibility) String() string {
	if v == VisibilityHidden {
		return "hidden"
	}
	return "visible"
}

// SettingReport is the payload of a DECRPSS reply.
//
// The reply spells the control function out the way a CSI would, without the
// introducer, so "2 q" is the terminal reporting a steady block for DECSCUSR.
//
// Two different things can go unanswered, and they are not the same field.
// Recognized says whether the TERMINAL knew the setting: it is false only for
// the reply that declines the request, which carries nothing at all. Setting
// says whether THIS PACKAGE has a decoded form for what the terminal sent, so a
// reply the terminal answered in full still leaves it nil when the control
// function is one this package does not decode, or when the value is not valid
// for one it does. Raw is always what the terminal sent, whenever it sent
// anything.
type SettingReport struct {
	// Raw is the reported setting as the terminal spelled it, and is empty
	// only when the terminal declined the request. It carries the setting
	// whether or not Setting decodes it, so a caller that knows a control
	// function this package does not can still read the answer.
	Raw string
	// Recognized reports whether the terminal recognized the setting.
	Recognized bool
	// Setting is the decoded setting, or nil when there is none to give: the
	// reply names a control function this package does not decode, or names
	// one it does and carries a value that is not valid for it. Read it with a
	// type switch over the types that implement [Setting].
	Setting Setting
}

// Setting is a decoded DECRPSS setting.
//
// DECRQSS can ask after many control functions, so the set of types
// implementing this grows. A type switch over it must expect a type it does
// not know, and fall back to [SettingReport.Raw], which carries every reply
// whether or not one of these decodes it.
//
// The interface is closed to this package so that adding a setting cannot
// change what an existing one means.
type Setting interface {
	isSetting()
}

// CursorStyleSetting is the cursor style a DECSCUSR report carries.
//
// Ask for it with Program.RequestCursorStyle.
type CursorStyleSetting struct {
	// Style is the reported style. Read it with
	// [altscreen.io/vision/ansi.CursorStyle.Shape] and
	// [altscreen.io/vision/ansi.CursorStyle.Blinking], the same terms
	// Program.SetCursorStyle takes.
	Style ansi.CursorStyle
}

func (CursorStyleSetting) isSetting() {}

// AsKey returns the key payload if ev is any key event.
func AsKey(ev Event) (Key, bool) {
	switch e := ev.(type) {
	case KeyPressEvent:
		return Key(e), true
	case KeyRepeatEvent:
		return Key(e), true
	case KeyReleaseEvent:
		return Key(e), true
	default:
		return Key{}, false
	}
}

// AsMouse returns the mouse payload if ev is any mouse event.
func AsMouse(ev Event) (Mouse, bool) {
	switch e := ev.(type) {
	case MouseClickEvent:
		return Mouse(e), true
	case MouseReleaseEvent:
		return Mouse(e), true
	case MouseWheelEvent:
		return Mouse(e), true
	case MouseMoveEvent:
		return Mouse(e), true
	default:
		return Mouse{}, false
	}
}
