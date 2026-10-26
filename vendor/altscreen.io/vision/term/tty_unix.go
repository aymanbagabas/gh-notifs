//go:build !windows

package term

// The controlling terminal is one device, opened twice.
const (
	ttyInputName  = "/dev/tty"
	ttyOutputName = "/dev/tty"
)
