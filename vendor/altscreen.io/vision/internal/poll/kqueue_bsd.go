//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package poll

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

// kqueue waits with kqueue(2), registering each handle under EVFILT_READ for
// level-triggered readiness.
//
// A returned event is readiness whatever its flags, so EV_EOF reaches the read
// path as a readable handle and surfaces there as the underlying error. The
// registration index comes from matching the event's identifier against the
// registered handles rather than from the udata field, whose type varies across
// the BSDs.
type kqueue struct {
	kq      int
	handles []Handle
}

func newKqueue(handles []Handle) (Poller, error) {
	if err := validate(handles); err != nil {
		return nil, err
	}
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	// kqueue(2) does not set close-on-exec on any current BSD.
	unix.CloseOnExec(kq)

	changes := make([]unix.Kevent_t, len(handles))
	for i, h := range handles {
		unix.SetKevent(&changes[i], int(h), unix.EVFILT_READ, unix.EV_ADD)
	}
	if _, err := unix.Kevent(kq, changes, nil, nil); err != nil {
		unix.Close(kq)
		return nil, err
	}
	return &kqueue{kq: kq, handles: append([]Handle(nil), handles...)}, nil
}

func (p *kqueue) Poll(ready []bool, timeout time.Duration) (int, error) {
	if err := checkReadyLen(ready, len(p.handles)); err != nil {
		return 0, err
	}
	clear(ready)

	deadline := deadlineFor(timeout)
	events := make([]unix.Kevent_t, len(p.handles))
	for {
		var ts *unix.Timespec
		if left, forever := remaining(deadline); !forever {
			spec := unix.NsecToTimespec(int64(left))
			ts = &spec
		}
		n, err := unix.Kevent(p.kq, nil, events, ts)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		count := 0
		for _, ev := range events[:n] {
			i := p.indexOf(Handle(ev.Ident))
			if i >= 0 && !ready[i] {
				ready[i] = true
				count++
			}
		}
		return count, nil
	}
}

func (p *kqueue) Close() error { return unix.Close(p.kq) }

// indexOf reports where handle was registered, or -1. The set is the few
// handles an event source watches, so a scan beats carrying an index through
// the udata field, which is a pointer on some BSDs and an integer on others.
func (p *kqueue) indexOf(handle Handle) int {
	for i, h := range p.handles {
		if h == handle {
			return i
		}
	}
	return -1
}
