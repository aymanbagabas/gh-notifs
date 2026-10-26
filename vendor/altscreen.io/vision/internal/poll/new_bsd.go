//go:build freebsd || netbsd || openbsd || dragonfly

package poll

// New returns a Poller watching handles. The BSDs use kqueue whatever the input
// is; only Darwin needs the terminal exception.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	return newKqueue(handles)
}
