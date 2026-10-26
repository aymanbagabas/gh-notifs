//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !zos

package program

import "errors"

// Suspend is unsupported on platforms without SIGTSTP.
func (p *Program) Suspend() error { return errors.ErrUnsupported }
