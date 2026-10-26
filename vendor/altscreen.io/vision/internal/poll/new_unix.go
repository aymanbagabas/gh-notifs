//go:build unix && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package poll

// New returns a Poller watching handles. Unix targets with no native readiness
// backend fall back to select(2) rather than poll(2), because poll does not
// report readiness for tty character devices on illumos and Solaris.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	return newSelect(handles)
}
