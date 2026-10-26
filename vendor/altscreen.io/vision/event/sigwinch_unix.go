//go:build unix

package event

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

type resizeSource struct{ ch chan os.Signal }

func newResizeSource() resizeSource {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGWINCH)
	return resizeSource{ch: ch}
}

func (r resizeSource) C() <-chan os.Signal { return r.ch }

// stop ends signal delivery. It runs before the resize pipe closes, so the
// goroutine forwarding to that pipe cannot write to a reused descriptor.
func (r resizeSource) stop() { signal.Stop(r.ch); close(r.ch) }
