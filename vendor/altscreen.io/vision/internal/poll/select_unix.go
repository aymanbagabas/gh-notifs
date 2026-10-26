//go:build unix

package poll

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fdSetCapacity is how many descriptors a unix.FdSet holds, derived from the
// type so it stays right where the word size differs: the bit array is 32-bit
// words on Darwin and 64-bit words on Linux. unix.FD_SETSIZE is not defined on
// every target, so it cannot be used here.
const fdSetCapacity = len(unix.FdSet{}.Bits) * int(unsafe.Sizeof(unix.FdSet{}.Bits[0])) * 8

// selectPoller waits with select(2). It is the portable fallback and the
// Darwin backend for terminal input, where kqueue wakes repeatedly on a tty
// character device.
//
// select reports only read readiness; a hangup arrives as readable and becomes
// an error on the read path. It also consumes the descriptor set and the
// timeout it is given, so both are rebuilt for every wait, which keeps Poll
// free of shared mutable state and safe to call concurrently.
type selectPoller struct {
	handles []Handle
	nfd     int
}

func newSelect(handles []Handle) (Poller, error) {
	if err := validate(handles); err != nil {
		return nil, err
	}
	nfd := 0
	for _, h := range handles {
		fd := int(h)
		// A descriptor at or past the set's capacity cannot be represented, and
		// setting it would run off the end of the bit array.
		if fd < 0 || fd >= fdSetCapacity {
			return nil, fmt.Errorf("poll: descriptor %d outside the %d select(2) supports", fd, fdSetCapacity)
		}
		if fd+1 > nfd {
			nfd = fd + 1
		}
	}
	return &selectPoller{handles: append([]Handle(nil), handles...), nfd: nfd}, nil
}

func (p *selectPoller) Poll(ready []bool, timeout time.Duration) (int, error) {
	if err := checkReadyLen(ready, len(p.handles)); err != nil {
		return 0, err
	}
	clear(ready)

	deadline := deadlineFor(timeout)
	for {
		var read unix.FdSet
		for _, h := range p.handles {
			read.Set(int(h))
		}
		var tv *unix.Timeval
		if left, forever := remaining(deadline); !forever {
			spec := unix.NsecToTimeval(int64(left))
			tv = &spec
		}
		if _, err := unix.Select(p.nfd, &read, nil, nil, tv); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		count := 0
		for i, h := range p.handles {
			if read.IsSet(int(h)) {
				ready[i] = true
				count++
			}
		}
		return count, nil
	}
}

func (p *selectPoller) Close() error { return nil }
