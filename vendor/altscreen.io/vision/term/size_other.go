//go:build !unix && !windows

package term

import "errors"

// GetWinsize is unimplemented on this platform and always fails.
func GetWinsize(File) (Winsize, error) {
	return Winsize{}, errors.ErrUnsupported
}
