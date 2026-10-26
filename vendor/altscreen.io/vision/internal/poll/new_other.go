//go:build !unix && !windows

package poll

import "errors"

// New reports that this target has no readiness backend, which leaves js/wasm
// and plan9 to the caller's fallback path.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	return nil, errors.ErrUnsupported
}
