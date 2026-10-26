//go:build unix

package event

import (
	"io"
	"time"

	"altscreen.io/vision/internal/poll"
	"altscreen.io/vision/term"
	"golang.org/x/sys/unix"
)

// Registration slots, in the order the handles are given to the poller.
const (
	slotInput = iota
	slotWake
	slotResize
	slotCount
)

// pollState is what the Unix readiness path needs beyond the backend itself:
// the input to read once it is ready, and the two pipes that make a wake and a
// resize visible to a backend that can only wait on descriptors.
type pollState struct {
	reader io.Reader
	wake   selfPipe
	resize selfPipe
	// resizeDone is closed by the goroutine forwarding resizes as it returns,
	// so teardown can wait for it to stop touching the pipe.
	resizeDone chan struct{}
	// read is reused across reads. The decoder copies what it is handed, and
	// the poller path parses before waiting again, so one buffer serves every
	// read instead of allocating one each time.
	read []byte
}

// selfPipe is a pipe used to make an out-of-band event, a wake or a signal,
// visible to a readiness backend that can only wait on descriptors.
type selfPipe struct{ r, w int }

// newSelfPipe returns a non-blocking, close-on-exec pipe. Non-blocking matters
// on the write side: a waker must never block because an earlier byte is still
// unread, and on the read side so draining can stop at the first empty read.
func newSelfPipe() (selfPipe, error) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		return selfPipe{}, err
	}
	for _, fd := range fds {
		if err := unix.SetNonblock(fd, true); err != nil {
			unix.Close(fds[0])
			unix.Close(fds[1])
			return selfPipe{}, err
		}
		unix.CloseOnExec(fd)
	}
	return selfPipe{r: fds[0], w: fds[1]}, nil
}

// notify writes one byte. A full pipe means a previous notification is still
// unread, which already says what this one would.
func (p selfPipe) notify() error {
	_, err := unix.Write(p.w, []byte{0})
	if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
		return nil
	}
	return err
}

// drain empties the pipe. Several notifications may have arrived, and they all
// mean the same thing.
func (p selfPipe) drain() {
	var buf [64]byte
	for {
		n, err := unix.Read(p.r, buf[:])
		if n <= 0 || err != nil {
			return
		}
	}
}

func (p selfPipe) close() {
	unix.Close(p.r)
	unix.Close(p.w)
}

// startPolling registers the input descriptor, the wake pipe, and the resize
// pipe with the platform readiness backend, so the source waits in one place
// instead of reading ahead on a goroutine.
//
// It reports false when there is nothing to register, which is the case for a
// reader with no descriptor, and leaves the caller on the goroutine path.
func (s *Source) startPolling(r Input) bool {
	f, ok := r.(term.File)
	if !ok {
		return false
	}
	fd := int(f.Fd())
	if fd < 0 {
		return false
	}

	wake, err := newSelfPipe()
	if err != nil {
		return false
	}
	resize, err := newSelfPipe()
	if err != nil {
		wake.close()
		return false
	}

	handles := [slotCount]poll.Handle{
		slotInput:  poll.Handle(fd),
		slotWake:   poll.Handle(wake.r),
		slotResize: poll.Handle(resize.r),
	}
	poller, err := poll.New(handles[:], term.IsTerminal(f))
	if err != nil {
		wake.close()
		resize.close()
		return false
	}

	s.poller = poller
	s.ready = make([]bool, slotCount)
	done := make(chan struct{})
	s.polls = pollState{reader: r, wake: wake, resize: resize, resizeDone: done, read: make([]byte, defaultBufferCapacity)}
	s.wakePipe = &s.polls.wake

	// The signal package cannot write to a pipe from the handler, so a
	// goroutine forwards each delivery to one the backend can wait on.
	go func() {
		defer close(done)
		for range s.resize.C() {
			if resize.notify() != nil {
				return
			}
		}
	}()
	return true
}

// waitPoll waits for one of the registered handles and turns that into the same
// result the goroutine path produces.
func (s *Source) waitPoll(poller poll.Poller, timeout time.Duration) (readResult, bool, bool) {
	n, err := poller.Poll(s.ready, timeout)
	if err != nil {
		return readResult{err: err}, true, false
	}
	if n == 0 {
		return readResult{}, false, false
	}

	// Drain the wake pipe first so the byte cannot wake the next wait too, then
	// report the interruption only when nothing else is ready.
	woke := false
	if s.ready[slotWake] {
		s.polls.wake.drain()
		woke = true
	}
	if s.ready[slotResize] {
		s.polls.resize.drain()
		// A nil payload with no error is how the source is told to re-measure.
		return readResult{}, true, false
	}
	if s.ready[slotInput] {
		nr, err := s.polls.reader.Read(s.polls.read)
		if nr > 0 {
			return readResult{data: s.polls.read[:nr]}, true, false
		}
		if err != nil {
			return readResult{err: err}, true, false
		}
		// A readable descriptor with nothing on it is the hangup the backends
		// report as readiness.
		return readResult{err: io.EOF}, true, false
	}
	return readResult{}, false, woke
}

// closePolling releases the poller and the pipes it watches.
func (s *Source) closePolling() {
	if s.poller == nil {
		return
	}
	// End signal delivery, then wait for the goroutine forwarding resizes to
	// return. Closing the channel only tells it to stop; a delivery it already
	// took is still on its way to the pipe, and closing that pipe first would
	// leave the write to land on whatever reused the descriptor.
	s.resize.stop()
	<-s.polls.resizeDone
	s.poller.Close()
	s.polls.wake.close()
	s.polls.resize.close()
	s.poller = nil
	s.wakePipe = nil
}
