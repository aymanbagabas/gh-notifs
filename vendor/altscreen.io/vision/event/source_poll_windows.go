//go:build windows

package event

import (
	"io"
	"time"

	"altscreen.io/vision/internal/poll"
	winapi "altscreen.io/vision/internal/windows"
	"golang.org/x/sys/windows"
)

// Registration slots, in the order the handles are given to the poller. A
// console reports resizes as records on the input handle, so unlike Unix there
// is no separate resize slot.
const (
	slotInput = iota
	slotWake
	slotCount
)

// pollState is what the Windows readiness path needs beyond the backend: the
// console to read once it signals, the event a waker signals, and the state the
// record serializers thread between calls.
type pollState struct {
	console windows.Handle
	vtInput bool
	wake    selfPipe

	pendingHighSurrogate [2]uint16
	lastMouseButtons     uint32
	size                 lastSize
}

// selfPipe is a manual-reset event, which is what a Windows readiness backend
// can wait on. It carries the name the Unix side uses so the waker needs no
// platform branch of its own.
type selfPipe struct{ h windows.Handle }

// newSelfPipe creates a manual-reset event, unsignaled. Manual reset means a
// wake stays visible until it is drained, so it cannot be missed between the
// signal and the wait.
func newSelfPipe() (selfPipe, error) {
	h, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return selfPipe{}, err
	}
	return selfPipe{h: h}, nil
}

func (p selfPipe) notify() error {
	if p.h == 0 {
		return nil
	}
	return windows.SetEvent(p.h)
}

func (p selfPipe) drain() {
	if p.h != 0 {
		windows.ResetEvent(p.h)
	}
}

func (p selfPipe) close() {
	if p.h != 0 {
		windows.CloseHandle(p.h)
	}
}

// startPolling registers the console input handle and a wake event with
// WaitForMultipleObjects, so the source waits rather than blocking a goroutine
// in a read it cannot cancel.
//
// It reports false for anything that is not a console, leaving pipes, files,
// and redirected input on the goroutine reader.
func (s *Source) startPolling(r Input) bool {
	f, ok := r.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	console := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(console, &mode); err != nil {
		return false
	}

	wake, err := newSelfPipe()
	if err != nil {
		return false
	}
	handles := [slotCount]poll.Handle{
		slotInput: poll.Handle(console),
		slotWake:  poll.Handle(wake.h),
	}
	poller, err := poll.New(handles[:], true)
	if err != nil {
		wake.close()
		return false
	}

	s.poller = poller
	s.ready = make([]bool, slotCount)
	s.polls = pollState{
		console: console,
		vtInput: mode&windows.ENABLE_VIRTUAL_TERMINAL_INPUT != 0,
		wake:    wake,
	}
	s.wakePipe = &s.polls.wake
	return true
}

// waitPoll waits for the console or the wake event and turns console records
// into the same bytes the goroutine path produces.
func (s *Source) waitPoll(poller poll.Poller, timeout time.Duration) (readResult, bool, bool) {
	n, err := poller.Poll(s.ready, timeout)
	if err != nil {
		return readResult{err: err}, true, false
	}
	if n == 0 {
		return readResult{}, false, false
	}

	woke := false
	if s.ready[slotWake] {
		s.polls.wake.drain()
		woke = true
	}
	if s.ready[slotInput] {
		var records [consoleReadBatch]winapi.InputRecord
		var read uint32
		if err := winapi.ReadConsoleInput(s.polls.console, &records[0], uint32(len(records)), &read); err != nil {
			return readResult{err: err}, true, false
		}
		var buf []byte
		for _, rec := range records[:read] {
			buf = serializeRecord(rec, s.polls.vtInput, &s.polls.pendingHighSurrogate, &s.polls.lastMouseButtons, &s.polls.size, buf)
		}
		if len(buf) > 0 {
			return readResult{data: buf}, true, false
		}
		// Records that carry nothing, a menu event or a key that produces no
		// bytes, are not a reason to report readiness upward.
		return readResult{}, false, woke
	}
	return readResult{}, false, woke
}

// closePolling releases the poller and the wake event.
func (s *Source) closePolling() {
	if s.poller == nil {
		return
	}
	s.poller.Close()
	s.polls.wake.close()
	s.poller = nil
	s.wakePipe = nil
}

var _ io.Reader = Input(nil)
