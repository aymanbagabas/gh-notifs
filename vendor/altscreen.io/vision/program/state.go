package program

import (
	stdcolor "image/color"
	"slices"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/event"
)

type state struct {
	cursorStyle       ansi.CursorStyle
	mouse             *MouseTracking
	bracketedPaste    bool
	focusEvents       bool
	colorScheme       bool
	visibilityReports bool
	inBandResize      bool
	windowTitle       string
	hasWindowTitle    bool
	iconName          string
	hasIconName       bool
	foregroundColor   stdcolor.Color
	backgroundColor   stdcolor.Color
	cursorColor       stdcolor.Color
	palette           map[int]stdcolor.Color
	modifyOtherKeys   event.ModifyOtherKeysMode
	pointerShape      string
	hasPointerShape   bool
	progress          *Progress
	kittyKeyboard     ansi.KittyKeyboardFlags
	altScreen         bool
	cursorVisible     bool
	graphemeClusters  bool
	// backspaceMode holds the application's selection, not the latest report.
	backspaceMode bool
	chosen        map[ansi.Mode]bool
}

func defaultState() state {
	return state{cursorVisible: true, palette: map[int]stdcolor.Color{}, chosen: map[ansi.Mode]bool{}}
}

// Capabilities holds terminal replies observed by a Program.
type Capabilities struct {
	modes                     map[ansi.Mode]ansi.ModeSetting
	primaryDeviceAttributes   []event.OptionalInt
	secondaryDeviceAttributes []event.OptionalInt
	tertiaryDeviceAttributes  string
	hasTertiary               bool
	kittyKeyboard             *ansi.KittyKeyboardFlags
	modifyOtherKeys           *event.ModifyOtherKeysMode
	terminalName              string
	hasTerminalName           bool
	termcap                   map[string]*string
	foregroundColor           stdcolor.Color
	backgroundColor           stdcolor.Color
	cursorColor               stdcolor.Color
	palette                   map[int]stdcolor.Color
	colorScheme               *event.ColorScheme
	kittyGraphics             bool
	programStatus             *ansi.ProgramStatusCapabilities
}

// NewCapabilities returns an empty capability set.
func NewCapabilities() Capabilities {
	return Capabilities{modes: map[ansi.Mode]ansi.ModeSetting{}, termcap: map[string]*string{}, palette: map[int]stdcolor.Color{}}
}

func (c *Capabilities) init() {
	if c.modes == nil {
		*c = NewCapabilities()
	}
}

// Mode returns the reported setting for mode.
func (c Capabilities) Mode(mode ansi.Mode) (ansi.ModeSetting, bool) {
	v, ok := c.modes[mode]
	return v, ok
}

// Supports reports whether mode was reported as available.
func (c Capabilities) Supports(mode ansi.Mode) bool {
	v, ok := c.Mode(mode)
	return ok && v.IsAvailable()
}

// Modes returns every mode report recorded so far.
func (c Capabilities) Modes() map[ansi.Mode]ansi.ModeSetting { return c.modes }

// PrimaryDeviceAttributes returns the raw primary DA attributes.
func (c Capabilities) PrimaryDeviceAttributes() ([]event.OptionalInt, bool) {
	if c.primaryDeviceAttributes == nil {
		return nil, false
	}
	return c.primaryDeviceAttributes, true
}

// SecondaryDeviceAttributes returns the raw secondary DA attributes.
func (c Capabilities) SecondaryDeviceAttributes() ([]event.OptionalInt, bool) {
	if c.secondaryDeviceAttributes == nil {
		return nil, false
	}
	return c.secondaryDeviceAttributes, true
}

// TertiaryDeviceAttributes returns the terminal unit ID.
func (c Capabilities) TertiaryDeviceAttributes() (string, bool) {
	return c.tertiaryDeviceAttributes, c.hasTertiary
}

// KittyGraphics reports whether a Kitty graphics response was observed.
func (c Capabilities) KittyGraphics() bool { return c.kittyGraphics }

// ProgramStatus returns an independent copy of the latest OSC 7501 capabilities.
// The Boolean reports whether a reply was observed, even if its lists are empty.
// False means no reply was observed, not a definitive rejection.
func (c Capabilities) ProgramStatus() (ansi.ProgramStatusCapabilities, bool) {
	if c.programStatus == nil {
		return ansi.ProgramStatusCapabilities{}, false
	}
	return ansi.ProgramStatusCapabilities{
		States: slices.Clone(c.programStatus.States),
		Kinds:  slices.Clone(c.programStatus.Kinds),
	}, true
}

// KittyKeyboard returns the reported Kitty keyboard flags.
func (c Capabilities) KittyKeyboard() (ansi.KittyKeyboardFlags, bool) {
	if c.kittyKeyboard == nil {
		return 0, false
	}
	return *c.kittyKeyboard, true
}

// ModifyOtherKeys returns the reported modifyOtherKeys mode.
func (c Capabilities) ModifyOtherKeys() (event.ModifyOtherKeysMode, bool) {
	if c.modifyOtherKeys == nil {
		return 0, false
	}
	return *c.modifyOtherKeys, true
}

// TerminalName returns the self-reported terminal name.
func (c Capabilities) TerminalName() (string, bool) { return c.terminalName, c.hasTerminalName }

// Termcap returns the reported termcap value.
func (c Capabilities) Termcap(name string) (string, bool) {
	v, ok := c.termcap[name]
	if !ok || v == nil {
		return "", false
	}
	return *v, true
}

// SupportsTermcap reports whether name was reported as supported.
func (c Capabilities) SupportsTermcap(name string) bool { _, ok := c.Termcap(name); return ok }

// TermcapReports returns every termcap report recorded so far.
func (c Capabilities) TermcapReports() map[string]*string { return c.termcap }

// ForegroundColor returns the reported default foreground color.
func (c Capabilities) ForegroundColor() (stdcolor.Color, bool) {
	return c.foregroundColor, c.foregroundColor != nil
}

// BackgroundColor returns the reported default background color.
func (c Capabilities) BackgroundColor() (stdcolor.Color, bool) {
	return c.backgroundColor, c.backgroundColor != nil
}

// CursorColor returns the reported cursor color.
func (c Capabilities) CursorColor() (stdcolor.Color, bool) {
	return c.cursorColor, c.cursorColor != nil
}

// PaletteColor returns the reported palette color at index.
func (c Capabilities) PaletteColor(index int) (stdcolor.Color, bool) {
	v, ok := c.palette[index]
	return v, ok
}

// Palette returns every palette color report recorded so far.
func (c Capabilities) Palette() map[int]stdcolor.Color { return c.palette }

// ColorScheme returns the latest reported color scheme.
func (c Capabilities) ColorScheme() (event.ColorScheme, bool) {
	if c.colorScheme == nil {
		return 0, false
	}
	return *c.colorScheme, true
}
