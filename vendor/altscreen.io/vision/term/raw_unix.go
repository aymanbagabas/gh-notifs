//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package term

import "golang.org/x/sys/unix"

// State holds the terminal attributes captured before raw mode was entered,
// so [SetState] can put them back.
//
// A terminal can be split across two files, and either half may be something
// other than a terminal: output to a pipe with input still on the tty, for
// instance. A half that is not a terminal is recorded as absent and left
// alone on restore.
type State struct {
	// Input holds the attributes of the input half, or nil when that half is
	// not a terminal.
	Input *unix.Termios
	// Output holds the attributes of the output half, or nil when that half
	// is not a terminal.
	Output *unix.Termios
}

// GetState reads the current attributes of both halves of a terminal.
//
// It fails only when neither half is a terminal, reporting the output error.
// When one half is a terminal and the other is not, the attributes of the
// working half are returned and the other is recorded as absent.
func GetState(input, output File) (*State, error) {
	// IoctlGetTermios hands back a non-nil pointer even when it fails, so
	// presence has to be decided by the error rather than the pointer.
	in, inErr := unix.IoctlGetTermios(int(input.Fd()), ioctlReadTermios)
	out, outErr := unix.IoctlGetTermios(int(output.Fd()), ioctlReadTermios)
	if inErr != nil && outErr != nil {
		return nil, outErr
	}

	var s State
	if inErr == nil {
		s.Input = in
	}
	if outErr == nil {
		s.Output = out
	}
	return &s, nil
}

// SetState applies state to both halves of a terminal.
//
// A half recorded as absent is skipped. Both halves are attempted even when
// the first fails, so one bad descriptor cannot strand the other.
func SetState(input, output File, state *State) error {
	inErr := setTermios(input, state.Input)
	outErr := setTermios(output, state.Output)
	if inErr != nil {
		return inErr
	}
	return outErr
}

func setTermios(f File, t *unix.Termios) error {
	if t == nil {
		return nil
	}
	return unix.IoctlSetTermios(int(f.Fd()), ioctlWriteTermios, t)
}

// MakeRaw puts both halves of a terminal into raw mode and returns the
// attributes they had beforehand, for [SetState] to restore.
//
// Raw mode turns off echo, line buffering, signal generation, and output
// post-processing, so bytes arrive exactly as typed and nothing is rewritten
// on the way out.
//
// If either half fails to switch, the previous attributes are put back before
// returning, so no descriptor is left stranded in raw mode.
func MakeRaw(input, output File) (*State, error) {
	original, err := GetState(input, output)
	if err != nil {
		return nil, err
	}

	raw := &State{Input: rawTermios(original.Input), Output: rawTermios(original.Output)}
	if err := SetState(input, output, raw); err != nil {
		// One half may have been switched before the other failed. Put the
		// pre-call attributes back; the half that failed is simply rewritten
		// its own unchanged attributes.
		_ = SetState(input, output, original)
		return nil, err
	}
	return original, nil
}

// rawTermios returns a copy of t with the cooked-mode flags cleared.
func rawTermios(t *unix.Termios) *unix.Termios {
	if t == nil {
		return nil
	}
	raw := *t
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	return &raw
}

// IsTerminal reports whether f is a terminal.
func IsTerminal(f File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlReadTermios)
	return err == nil
}
