//go:build linux

package poll

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

// epoll waits with epoll(7), watching each handle for level-triggered read
// readiness. The registration index rides in the event data so a ready event
// maps straight back to the caller's slot.
type epoll struct {
	epfd  int
	count int
}

// New returns a Poller watching handles. Linux always uses epoll, so
// inputIsTerminal does not affect the choice.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	if err := validate(handles); err != nil {
		return nil, err
	}
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	for i, h := range handles {
		// EPOLLERR and EPOLLHUP arrive whether or not they are requested; naming
		// them documents that a hangup counts as readiness and is left for the
		// read path to turn into an error.
		ev := unix.EpollEvent{
			Events: unix.EPOLLIN | unix.EPOLLERR | unix.EPOLLHUP,
			Fd:     int32(i), // the registration index, not the descriptor
		}
		if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, int(h), &ev); err != nil {
			unix.Close(epfd)
			return nil, err
		}
	}
	return &epoll{epfd: epfd, count: len(handles)}, nil
}

func (p *epoll) Poll(ready []bool, timeout time.Duration) (int, error) {
	if err := checkReadyLen(ready, p.count); err != nil {
		return 0, err
	}
	clear(ready)

	deadline := deadlineFor(timeout)
	events := make([]unix.EpollEvent, p.count)
	for {
		msec := -1
		if left, forever := remaining(deadline); !forever {
			msec = epollMillis(left)
		}
		n, err := unix.EpollWait(p.epfd, events, msec)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		count := 0
		for _, ev := range events[:n] {
			i := int(ev.Fd)
			if i >= 0 && i < len(ready) && !ready[i] {
				ready[i] = true
				count++
			}
		}
		return count, nil
	}
}

func (p *epoll) Close() error { return unix.Close(p.epfd) }
