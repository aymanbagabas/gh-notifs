package term

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotTerminal reports that a half of a terminal cannot be one, because it
// yields no descriptor or handle to size or to put into raw mode.
var ErrNotTerminal = errors.New("term: not a terminal")

// Terminal pairs terminal input, output, environment, and saved raw state.
type Terminal struct {
	input      io.Reader
	output     io.Writer
	inputFile  File
	outputFile File
	env        Environ
	saved      *State
}

// Stdio returns a terminal over the inherited standard input and output.
func Stdio() (*Terminal, error) { return New(os.Stdin, os.Stdout, ProcessEnv{}) }

// Open opens the controlling terminal.
func Open() (*Terminal, error) {
	input, output, err := OpenTTY()
	if err != nil {
		return nil, err
	}
	return New(input, output, ProcessEnv{})
}

// New builds a terminal from an input half, an output half, and an
// environment. A nil environment reads the live process environment.
//
// Both halves must be terminals: they must satisfy [File], meaning they yield
// a descriptor or handle, and must pass [IsTerminal]. Anything else cannot be
// sized or put into raw mode, so it is rejected with [ErrNotTerminal] here
// rather than failing at every later call.
//
// Rendering to a pipe, a file, or a buffer does not go through this type.
// [altscreen.io/vision/screen.New] takes a plain [io.Writer] and
// manages no session.
func New(input io.Reader, output io.Writer, env Environ) (*Terminal, error) {
	if env == nil {
		env = ProcessEnv{}
	}
	inputFile, ok := input.(File)
	if !ok || !IsTerminal(inputFile) {
		return nil, fmt.Errorf("input: %w", ErrNotTerminal)
	}
	outputFile, ok := output.(File)
	if !ok || !IsTerminal(outputFile) {
		return nil, fmt.Errorf("output: %w", ErrNotTerminal)
	}
	return &Terminal{
		input:      input,
		output:     output,
		inputFile:  inputFile,
		outputFile: outputFile,
		env:        env,
	}, nil
}

// Env returns the terminal environment.
func (t *Terminal) Env() Environ { return t.env }

// GetEnv looks up an environment variable.
func (t *Terminal) GetEnv(key string) (string, bool) { return t.env.LookupEnv(key) }

// HasEnv reports whether an environment variable is present and non-empty.
func (t *Terminal) HasEnv(key string) bool { return HasEnv(t.env, key) }

// Input returns the terminal input half.
func (t *Terminal) Input() io.Reader { return t.input }

// Output returns the terminal output half.
func (t *Terminal) Output() io.Writer { return t.output }

// Read reads from the terminal input half.
func (t *Terminal) Read(p []byte) (int, error) { return t.input.Read(p) }

// Write writes to the terminal output half.
func (t *Terminal) Write(p []byte) (int, error) { return t.output.Write(p) }

// Flush flushes the output half when it supports flushing.
func (t *Terminal) Flush() error {
	if f, ok := t.output.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// MakeRaw puts the terminal into raw mode and saves its previous state.
func (t *Terminal) MakeRaw() (*State, error) {
	state, err := MakeRaw(t.inputFile, t.outputFile)
	if err != nil {
		return nil, err
	}
	t.saved = state
	return state, nil
}

// Restore restores the state saved by MakeRaw.
func (t *Terminal) Restore() error {
	if t.saved == nil {
		return nil
	}
	if err := SetState(t.inputFile, t.outputFile, t.saved); err != nil {
		return err
	}
	t.saved = nil
	return nil
}

// GetState reads the current terminal state.
func (t *Terminal) GetState() (*State, error) {
	return GetState(t.inputFile, t.outputFile)
}

// SetState applies a terminal state.
func (t *Terminal) SetState(state *State) error {
	return SetState(t.inputFile, t.outputFile, state)
}

// IsTerminal reports whether the input and output halves are attached to a
// terminal. Both can be false: holding a descriptor is not the same as being
// a terminal.
func (t *Terminal) IsTerminal() (bool, bool) {
	return IsTerminal(t.inputFile), IsTerminal(t.outputFile)
}

// GetWindowSize returns the terminal window size.
//
// The output half is asked first, since that is what the size describes, and
// the input half is a fallback for output redirected away from the terminal.
// The output error is the one reported when neither answers.
func (t *Terminal) GetWindowSize() (Winsize, error) {
	ws, err := GetWinsize(t.outputFile)
	if err == nil {
		return ws, nil
	}
	if in, inErr := GetWinsize(t.inputFile); inErr == nil {
		return in, nil
	}
	return Winsize{}, err
}

var _ io.ReadWriter = (*Terminal)(nil)
