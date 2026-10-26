//go:build darwin

package poll

// New returns a Poller watching handles. Darwin uses select(2) when the input
// is a terminal, because kqueue wakes repeatedly on a tty character device with
// nothing to read, and kqueue otherwise.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	if inputIsTerminal {
		return newSelect(handles)
	}
	return newKqueue(handles)
}
