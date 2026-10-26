//go:build windows

package term

import "golang.org/x/sys/windows"

// State holds the console modes captured before raw mode was entered, so
// [SetState] can put them back.
type State struct {
	// Input holds the console mode of the input half.
	Input uint32
	// Output holds the console mode of the output half.
	Output uint32
}

// GetState reads the current console modes of both halves of a terminal.
//
// Unlike the Unix build, both handles must be consoles: the console API has
// no partial answer to give.
func GetState(input, output File) (*State, error) {
	var in, out uint32
	if err := windows.GetConsoleMode(windows.Handle(input.Fd()), &in); err != nil {
		return nil, err
	}
	if err := windows.GetConsoleMode(windows.Handle(output.Fd()), &out); err != nil {
		return nil, err
	}
	return &State{Input: in, Output: out}, nil
}

// SetState applies state to both halves of a terminal.
//
// Both handles are attempted even when the first fails, so one bad handle
// cannot strand the other.
func SetState(input, output File, state *State) error {
	inErr := windows.SetConsoleMode(windows.Handle(input.Fd()), state.Input)
	outErr := windows.SetConsoleMode(windows.Handle(output.Fd()), state.Output)
	if inErr != nil {
		return inErr
	}
	return outErr
}

// MakeRaw puts both halves of a terminal into raw mode and returns the console
// modes they had beforehand, for [SetState] to restore.
//
// If either handle fails to switch, the previous modes are put back before
// returning, so no handle is left stranded in raw mode.
func MakeRaw(input, output File) (*State, error) {
	original, err := GetState(input, output)
	if err != nil {
		return nil, err
	}

	// Clearing quick-edit while setting extended-flags is how the console API
	// gives up mouse selection, which would otherwise swallow mouse input.
	rawInput := original.Input &^ (windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_LINE_INPUT |
		windows.ENABLE_QUICK_EDIT_MODE)
	rawInput |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT |
		windows.ENABLE_EXTENDED_FLAGS |
		windows.ENABLE_WINDOW_INPUT

	// Virtual terminal processing requires processed output, so it goes on
	// too.
	rawOutput := original.Output |
		windows.ENABLE_PROCESSED_OUTPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING |
		windows.DISABLE_NEWLINE_AUTO_RETURN

	raw := &State{Input: rawInput, Output: rawOutput}
	if err := SetState(input, output, raw); err != nil {
		_ = SetState(input, output, original)
		return nil, err
	}
	return original, nil
}

// IsTerminal reports whether f is a console handle.
func IsTerminal(f File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
