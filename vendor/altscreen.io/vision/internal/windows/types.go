package windows

import "encoding/binary"

// Input record event types.
// https://learn.microsoft.com/en-us/windows/console/input-record-str
//
//nolint:revive
const (
	KEY_EVENT                = 0x0001
	MOUSE_EVENT              = 0x0002
	WINDOW_BUFFER_SIZE_EVENT = 0x0004
	MENU_EVENT               = 0x0008
	FOCUS_EVENT              = 0x0010
)

// Control key states.
// https://learn.microsoft.com/en-us/windows/console/key-event-record-str
//
//nolint:revive
const (
	RIGHT_ALT_PRESSED  = 0x0001
	LEFT_ALT_PRESSED   = 0x0002
	RIGHT_CTRL_PRESSED = 0x0004
	LEFT_CTRL_PRESSED  = 0x0008
	SHIFT_PRESSED      = 0x0010
	NUMLOCK_ON         = 0x0020
	SCROLLLOCK_ON      = 0x0040
	CAPSLOCK_ON        = 0x0080
	ENHANCED_KEY       = 0x0100
)

// Mouse button states.
// https://learn.microsoft.com/en-us/windows/console/mouse-event-record-str
//
//nolint:revive
const (
	FROM_LEFT_1ST_BUTTON_PRESSED = 0x0001
	RIGHTMOST_BUTTON_PRESSED     = 0x0002
	FROM_LEFT_2ND_BUTTON_PRESSED = 0x0004
	FROM_LEFT_3RD_BUTTON_PRESSED = 0x0008
	FROM_LEFT_4TH_BUTTON_PRESSED = 0x0010
)

// Mouse event flags.
// https://learn.microsoft.com/en-us/windows/console/mouse-event-record-str
//
//nolint:revive
const (
	MOUSE_MOVED    = 0x0001
	DOUBLE_CLICK   = 0x0002
	MOUSE_WHEELED  = 0x0004
	MOUSE_HWHEELED = 0x0008
)

// Coord is a COORD, a console cell position or size.
// https://learn.microsoft.com/en-us/windows/console/coord-str
//
// It mirrors windows.Coord but carries no build tag, so records stay usable on
// every target.
type Coord struct {
	X int16
	Y int16
}

// InputRecord is an INPUT_RECORD, one entry from a console input buffer.
// https://learn.microsoft.com/en-us/windows/console/input-record-str
//
// EventType selects which of the accessor methods reads Event correctly.
type InputRecord struct {
	// EventType is one of the KEY_EVENT constants.
	EventType uint16

	// Padding of the 16-bit EventType to a whole 32-bit dword.
	_ [2]byte

	// Event is the union payload, read by the accessor for EventType.
	Event [16]byte
}

// KeyEventRecord is a KEY_EVENT_RECORD, one key press or release.
// https://learn.microsoft.com/en-us/windows/console/key-event-record-str
type KeyEventRecord struct {
	// KeyDown reports whether the key was pressed rather than released.
	KeyDown bool

	// RepeatCount is how many times a held key repeated.
	RepeatCount uint16

	// VirtualKeyCode identifies the key independently of the device.
	VirtualKeyCode uint16

	// VirtualScanCode is the value the keyboard hardware produced.
	VirtualScanCode uint16

	// Char is the UTF-16 code unit for the key, which is zero for keys that
	// produce no character and one half of a surrogate pair outside the basic
	// multilingual plane.
	Char uint16

	// ControlKeyState holds the modifier and lock key states.
	ControlKeyState uint32
}

// MouseEventRecord is a MOUSE_EVENT_RECORD, one mouse action.
// https://learn.microsoft.com/en-us/windows/console/mouse-event-record-str
type MouseEventRecord struct {
	// MousePosition is the cell the pointer is over.
	MousePosition Coord

	// ButtonState holds which buttons are down, and the wheel delta in its high
	// word for a wheel event.
	ButtonState uint32

	// ControlKeyState holds the modifier and lock key states.
	ControlKeyState uint32

	// EventFlags distinguishes a click from a move, a wheel, or a double click.
	EventFlags uint32
}

// WindowBufferSizeRecord is a WINDOW_BUFFER_SIZE_RECORD, reporting a new
// console screen buffer size.
// https://learn.microsoft.com/en-us/windows/console/window-buffer-size-record-str
type WindowBufferSizeRecord struct {
	// Size is the new buffer size in cells.
	Size Coord
}

// FocusEventRecord is a FOCUS_EVENT_RECORD, reporting a focus change.
// https://learn.microsoft.com/en-us/windows/console/focus-event-record-str
type FocusEventRecord struct {
	// SetFocus reports whether the window gained focus.
	SetFocus bool
}

// MenuEventRecord is a MENU_EVENT_RECORD. Microsoft documents these as
// reserved and they should be ignored.
// https://learn.microsoft.com/en-us/windows/console/menu-event-record-str
type MenuEventRecord struct {
	// CommandID identifies the menu command.
	CommandID uint32
}

// KeyEvent reads the record as a KEY_EVENT_RECORD.
func (ir InputRecord) KeyEvent() KeyEventRecord {
	return KeyEventRecord{
		KeyDown:         binary.LittleEndian.Uint32(ir.Event[0:4]) > 0,
		RepeatCount:     binary.LittleEndian.Uint16(ir.Event[4:6]),
		VirtualKeyCode:  binary.LittleEndian.Uint16(ir.Event[6:8]),
		VirtualScanCode: binary.LittleEndian.Uint16(ir.Event[8:10]),
		Char:            binary.LittleEndian.Uint16(ir.Event[10:12]),
		ControlKeyState: binary.LittleEndian.Uint32(ir.Event[12:16]),
	}
}

// MouseEvent reads the record as a MOUSE_EVENT_RECORD.
func (ir InputRecord) MouseEvent() MouseEventRecord {
	return MouseEventRecord{
		MousePosition: Coord{
			X: int16(binary.LittleEndian.Uint16(ir.Event[0:2])),
			Y: int16(binary.LittleEndian.Uint16(ir.Event[2:4])),
		},
		ButtonState:     binary.LittleEndian.Uint32(ir.Event[4:8]),
		ControlKeyState: binary.LittleEndian.Uint32(ir.Event[8:12]),
		EventFlags:      binary.LittleEndian.Uint32(ir.Event[12:16]),
	}
}

// WindowBufferSizeEvent reads the record as a WINDOW_BUFFER_SIZE_RECORD.
func (ir InputRecord) WindowBufferSizeEvent() WindowBufferSizeRecord {
	return WindowBufferSizeRecord{
		Size: Coord{
			X: int16(binary.LittleEndian.Uint16(ir.Event[0:2])),
			Y: int16(binary.LittleEndian.Uint16(ir.Event[2:4])),
		},
	}
}

// FocusEvent reads the record as a FOCUS_EVENT_RECORD.
func (ir InputRecord) FocusEvent() FocusEventRecord {
	return FocusEventRecord{SetFocus: ir.Event[0] > 0}
}

// MenuEvent reads the record as a MENU_EVENT_RECORD.
func (ir InputRecord) MenuEvent() MenuEventRecord {
	return MenuEventRecord{CommandID: binary.LittleEndian.Uint32(ir.Event[0:4])}
}
