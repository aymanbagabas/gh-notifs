//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !zos && !windows

package term

import "errors"

// State holds terminal attributes. This platform has none to hold.
type State struct{}

// GetState is unimplemented on this platform and always fails.
func GetState(_, _ File) (*State, error) {
	return nil, errors.ErrUnsupported
}

// SetState is unimplemented on this platform and always fails.
func SetState(_, _ File, _ *State) error {
	return errors.ErrUnsupported
}

// MakeRaw is unimplemented on this platform and always fails.
func MakeRaw(_, _ File) (*State, error) {
	return nil, errors.ErrUnsupported
}

// IsTerminal reports whether f is a terminal. It is always false here.
func IsTerminal(File) bool { return false }
