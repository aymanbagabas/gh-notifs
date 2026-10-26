package event

import "strings"

// MouseButton is a mouse button or wheel direction.
type MouseButton int

const (
	// MouseLeft is the primary mouse button.
	MouseLeft MouseButton = iota
	// MouseMiddle is the middle mouse button.
	MouseMiddle
	// MouseRight is the secondary mouse button.
	MouseRight
	// MouseWheelUp is vertical wheel up.
	MouseWheelUp
	// MouseWheelDown is vertical wheel down.
	MouseWheelDown
	// MouseWheelLeft is horizontal wheel left.
	MouseWheelLeft
	// MouseWheelRight is horizontal wheel right.
	MouseWheelRight
	// MouseButton4 is extra mouse button 4.
	MouseButton4
	// MouseButton5 is extra mouse button 5.
	MouseButton5
	// MouseButton6 is extra mouse button 6.
	MouseButton6
	// MouseButton7 is extra mouse button 7.
	MouseButton7
	// MouseButton8 is extra mouse button 8.
	MouseButton8
	// MouseButton9 is extra mouse button 9.
	MouseButton9
	// MouseButton10 is extra mouse button 10.
	MouseButton10
	// MouseButton11 is extra mouse button 11.
	MouseButton11
	// MouseNone means no button was reported.
	MouseNone
)

// Mouse is a mouse-event payload with position, button, and modifiers.
type Mouse struct {
	// X is the horizontal coordinate.
	X int
	// Y is the vertical coordinate.
	Y int
	// Button is the button, wheel direction, or no-button marker.
	Button MouseButton
	// Mod is the set of keyboard modifiers active during the mouse event.
	Mod KeyMod
}

// NewMouse constructs a mouse payload.
func NewMouse(x, y int, button MouseButton, modifiers KeyMod) Mouse {
	return Mouse{X: x, Y: y, Button: button, Mod: modifiers}
}

// MousePixelToCell converts an SGR-Pixel mouse payload to cell coordinates.
func MousePixelToCell(m Mouse, pixelWidth, pixelHeight, cols, rows int) Mouse {
	var x, y int
	if pixelWidth > 0 {
		x = m.X * cols / pixelWidth
	}
	if pixelHeight > 0 {
		y = m.Y * rows / pixelHeight
	}
	return NewMouse(x, y, m.Button, m.Mod)
}

// mouseButtonNames names each button after its constant, so a printed event
// leads back to the identifier that matches it.
var mouseButtonNames = map[MouseButton]string{
	MouseLeft: "left", MouseMiddle: "middle", MouseRight: "right",
	MouseWheelUp: "wheelup", MouseWheelDown: "wheeldown",
	MouseWheelLeft: "wheelleft", MouseWheelRight: "wheelright",
	MouseButton4: "button4", MouseButton5: "button5", MouseButton6: "button6",
	MouseButton7: "button7", MouseButton8: "button8", MouseButton9: "button9",
	MouseButton10: "button10", MouseButton11: "button11",
	MouseNone: "none",
}

// String returns the button's name, such as "left" or "wheelup".
func (b MouseButton) String() string {
	if name, ok := mouseButtonNames[b]; ok {
		return name
	}
	return "unknown"
}

// String returns the event's name, such as "left" or "ctrl+wheelup", using the
// same modifier order as [Key.String].
//
// The button is always named, so a report with no button reads "none" rather
// than as the empty string.
func (m Mouse) String() string {
	var b strings.Builder
	for _, mod := range []struct {
		flag KeyMod
		name string
	}{
		{KeyModCtrl, "ctrl+"},
		{KeyModAlt, "alt+"},
		{KeyModShift, "shift+"},
		{KeyModSuper, "super+"},
		{KeyModHyper, "hyper+"},
		{KeyModMeta, "meta+"},
	} {
		if m.Mod.Contains(mod.flag) {
			b.WriteString(mod.name)
		}
	}
	b.WriteString(m.Button.String())
	return b.String()
}
