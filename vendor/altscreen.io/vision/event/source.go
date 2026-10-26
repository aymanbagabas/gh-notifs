package event

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"altscreen.io/vision/internal/poll"
	"altscreen.io/vision/internal/trace"
	"altscreen.io/vision/term"
)

const defaultBufferCapacity = 4096

// DEFAULT_ESC_TIMEOUT is the default time allowed for an ambiguous escape prefix.
const DEFAULT_ESC_TIMEOUT = 50 * time.Millisecond

// DEFAULT_PASTE_IDLE_TIMEOUT is the default idle time before an unterminated paste is closed.
const DEFAULT_PASTE_IDLE_TIMEOUT = 2 * time.Second

type readResult struct {
	data []byte
	err  error
}

// Input is a readable terminal input source.
type Input interface{ io.Reader }

// Source reads terminal input, decodes it into events, and queues the results.
type Source struct {
	mu     sync.Mutex
	waitMu sync.Mutex

	inputFile term.File
	reads     <-chan readResult
	wakeCh    chan struct{}
	// wakePipe is set when a readiness backend is waiting on descriptors,
	// which a channel send would not reach.
	wakePipe *selfPipe
	notify   chan struct{}
	resize   resizeSource

	// Set when a readiness backend is driving the source instead of a reader
	// goroutine. What else that needs is platform specific, so it lives in
	// pollState rather than here.
	poller poll.Poller
	ready  []bool
	polls  pollState

	decoder *decoder
	queue   []Event
	readErr error

	escTimeout      time.Duration
	escDeadline     time.Time
	pasteIdle       time.Duration
	pasteIdleActive bool
	pasteDeadline   time.Time
	handleResize    bool
	lastSize        *term.Winsize
}

// NewSource constructs a Source over r.
func NewSource(r Input) (*Source, error) {
	ch := make(chan readResult, 16)

	s := &Source{
		reads:           ch,
		wakeCh:          make(chan struct{}, 1),
		notify:          make(chan struct{}, 1),
		resize:          newResizeSource(),
		decoder:         newDecoder(0),
		queue:           make([]Event, 0, 16),
		escTimeout:      DEFAULT_ESC_TIMEOUT,
		pasteIdle:       DEFAULT_PASTE_IDLE_TIMEOUT,
		pasteIdleActive: true,
		handleResize:    true,
	}
	if f, ok := r.(term.File); ok {
		s.inputFile = f
		if ws, err := term.GetWinsize(f); err == nil {
			s.lastSize = &ws
		}
	}
	// A reader with no descriptor cannot be registered with a backend, and one
	// that can is read directly rather than ahead of time.
	if !s.startPolling(r) {
		go pumpInput(r, ch)
	}
	return s, nil
}

// WithDecoderFlags chooses how the ambiguous legacy keys are read, returning
// the source so it can be chained after [NewSource].
//
// LF defaults to Ctrl+J; [DecoderFlagLFIsEnter] selects Enter instead.
// See [DecoderFlags] for all mappings.
func (s *Source) WithDecoderFlags(flags DecoderFlags) *Source {
	s.SetDecoderFlags(flags)
	return s
}

// SetDecoderFlags chooses how the ambiguous legacy keys are read, on a source
// already in use.
//
// Buffered input is left alone: a flag decides how a byte is named, not how it
// is framed, so a sequence half read keeps being read.
func (s *Source) SetDecoderFlags(flags DecoderFlags) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decoder.flags = flags
}

// DecoderFlags reports how the ambiguous legacy keys are being read.
func (s *Source) DecoderFlags() DecoderFlags {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decoder.flags
}

// BackspaceMode returns the decoder's Backarrow mode. Reset (false) is the default.
func (s *Source) BackspaceMode() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decoder.backspaceMode()
}

// SetBackspaceMode selects Backarrow mode for future legacy key decoding.
// It does not interpret reports or change events already in the queue.
func (s *Source) SetBackspaceMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decoder.setBackspaceMode(enabled)
}

func readInput(r io.Reader, ch chan<- readResult) {
	defer close(ch)
	buf := make([]byte, defaultBufferCapacity)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b := append([]byte(nil), buf[:n]...)
			ch <- readResult{data: b}
		}
		if err != nil {
			ch <- readResult{err: err}
			return
		}
	}
}

// EscTimeout returns the escape-prefix timeout.
func (s *Source) EscTimeout() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.escTimeout
}

// WithEscTimeout sets the escape-prefix timeout and returns s.
func (s *Source) WithEscTimeout(timeout time.Duration) *Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.escTimeout = timeout
	return s
}

// WithPasteIdleTimeout sets the bracketed-paste idle timeout and returns s.
// A negative timeout disables the idle safety net.
func (s *Source) WithPasteIdleTimeout(timeout time.Duration) *Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pasteIdle = timeout
	s.pasteIdleActive = timeout >= 0
	return s
}

// Wake interrupts a blocked poll or read on s.
func (s *Source) Wake() error {
	if s.wakePipe != nil {
		return s.wakePipe.notify()
	}
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
	return nil
}

// HandleResize reports whether out-of-band resize delivery is enabled.
func (s *Source) HandleResize() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handleResize
}

// SetHandleResize controls out-of-band resize delivery.
func (s *Source) SetHandleResize(enable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handleResize = enable
}

// Poll waits up to timeout for at least one event to become available.
// A negative timeout blocks indefinitely; zero performs a non-blocking pass.
// Millisecond-based backends round waits up and can return just after the deadline.
func (s *Source) Poll(timeout time.Duration) (bool, error) {
	s.waitMu.Lock()
	defer s.waitMu.Unlock()

	start := time.Now()
	for {
		s.mu.Lock()
		s.expireElapsedLocked(time.Now())
		if len(s.queue) > 0 {
			s.mu.Unlock()
			return true, nil
		}
		if s.readErr != nil {
			err := s.readErr
			s.mu.Unlock()
			return false, err
		}
		wait := s.effectiveTimeoutLocked(timeout, start, time.Now())
		// Read the backend while the lock is held. Close can nil it from
		// another goroutine, and waiting happens with the lock released.
		poller := s.poller
		s.mu.Unlock()

		res, ok, woke := s.wait(poller, wait)

		s.mu.Lock()
		if ok {
			s.handleReadResultLocked(res)
		} else {
			s.expireElapsedLocked(time.Now())
		}
		ready := len(s.queue) > 0
		err := s.readErr
		s.mu.Unlock()

		if ready {
			return true, nil
		}
		if woke {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if timeout >= 0 && time.Since(start) >= timeout {
			return false, nil
		}
	}
}

// wait blocks for input, a wake, or the timeout. It takes the readiness
// backend rather than reading the field, because the caller holds the lock when
// it looks and does not while it waits.
func (s *Source) wait(poller poll.Poller, timeout time.Duration) (readResult, bool, bool) {
	if poller != nil {
		return s.waitPoll(poller, timeout)
	}
	if timeout == 0 {
		select {
		case res, ok := <-s.reads:
			if !ok {
				return readResult{err: io.EOF}, true, false
			}
			return res, true, false
		case <-s.wakeCh:
			return readResult{}, false, true
		case <-s.notify:
			return readResult{data: nil}, true, false
		case <-s.resize.C():
			return readResult{data: nil}, true, false
		default:
			return readResult{}, false, false
		}
	}
	if timeout < 0 {
		select {
		case res, ok := <-s.reads:
			if !ok {
				return readResult{err: io.EOF}, true, false
			}
			return res, true, false
		case <-s.wakeCh:
			return readResult{}, false, true
		case <-s.notify:
			return readResult{data: nil}, true, false
		case <-s.resize.C():
			return readResult{data: nil}, true, false
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res, ok := <-s.reads:
		if !ok {
			return readResult{err: io.EOF}, true, false
		}
		return res, true, false
	case <-s.wakeCh:
		return readResult{}, false, true
	case <-s.notify:
		return readResult{data: nil}, true, false
	case <-s.resize.C():
		return readResult{data: nil}, true, false
	case <-timer.C:
		return readResult{}, false, false
	}
}

func (s *Source) effectiveTimeoutLocked(timeout time.Duration, start, now time.Time) time.Duration {
	wait := timeout
	if timeout > 0 {
		left := timeout - now.Sub(start)
		if left <= 0 {
			return 0
		}
		wait = left
	}
	if deadline, ok := s.nextDeadlineLocked(); ok {
		internal := time.Until(deadline)
		if internal < 0 {
			internal = 0
		}
		if timeout < 0 || internal < wait {
			wait = internal
		}
	}
	return wait
}

func (s *Source) nextDeadlineLocked() (time.Time, bool) {
	if s.decoder.inPasteMode() {
		return s.pasteDeadline, !s.pasteDeadline.IsZero()
	}
	return s.escDeadline, !s.escDeadline.IsZero()
}

func (s *Source) handleReadResultLocked(res readResult) {
	if res.data == nil && res.err == nil {
		s.handleResizeLocked()
		return
	}
	s.expireElapsedLocked(time.Now())
	if len(res.data) > 0 {
		trace.Input(res.data)
		s.queue = append(s.queue, s.decoder.parse(res.data)...)
		s.armDeadlinesLocked(time.Now())
	}
	if res.err != nil {
		if errors.Is(res.err, io.EOF) {
			if s.decoder.inPasteMode() {
				s.expirePasteLocked()
			} else {
				s.expirePartialLocked()
			}
		}
		s.readErr = res.err
	}
}

func (s *Source) handleResizeLocked() {
	if !s.handleResize || s.inputFile == nil {
		return
	}
	ws, err := term.GetWinsize(s.inputFile)
	if err != nil {
		return
	}
	if s.lastSize != nil && *s.lastSize == ws {
		return
	}
	s.lastSize = &ws
	s.queue = append(s.queue, ResizeEvent(ws))
}

func (s *Source) armDeadlinesLocked(now time.Time) {
	if s.decoder.inPasteMode() {
		s.escDeadline = time.Time{}
		if s.pasteIdleActive {
			s.pasteDeadline = now.Add(s.pasteIdle)
		} else {
			s.pasteDeadline = time.Time{}
		}
		return
	}
	s.pasteDeadline = time.Time{}
	if s.decoder.hasPending() {
		if s.escDeadline.IsZero() {
			s.escDeadline = now.Add(s.escTimeout)
		}
	} else {
		s.escDeadline = time.Time{}
	}
}

func (s *Source) expireElapsedLocked(now time.Time) {
	if s.decoder.inPasteMode() {
		if !s.pasteDeadline.IsZero() && !s.pasteDeadline.After(now) {
			s.expirePasteLocked()
		}
		return
	}
	if !s.escDeadline.IsZero() && !s.escDeadline.After(now) {
		s.expirePartialLocked()
	}
}

func (s *Source) expirePartialLocked() {
	s.escDeadline = time.Time{}
	s.queue = append(s.queue, s.decoder.drain()...)
	s.armDeadlinesLocked(time.Now())
}

func (s *Source) expirePasteLocked() {
	s.pasteDeadline = time.Time{}
	if !s.decoder.inPasteMode() {
		return
	}
	s.queue = append(s.queue, s.decoder.parse(nil)...)
	if len(s.decoder.buf) > 0 {
		s.queue = append(s.queue, PasteChunkEvent(append([]byte(nil), s.decoder.buf...)))
		s.decoder.buf = s.decoder.buf[:0]
	}
	if ev := s.decoder.endPaste(); ev != nil {
		s.queue = append(s.queue, ev)
	}
}

func (s *Source) notifyReady() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// TryRead returns a queued event without performing I/O.
func (s *Source) TryRead() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil, false
	}
	ev := s.queue[0]
	s.queue = s.queue[1:]
	return ev, true
}

// Read blocks until the next event is available.
func (s *Source) Read() (Event, error) {
	for {
		if ev, ok := s.TryRead(); ok {
			return ev, nil
		}
		ok, err := s.Poll(-1)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, io.ErrNoProgress
		}
	}
}

// Unread returns ev to the front of the queue.
// It panics if ev is nil.
func (s *Source) Unread(ev Event) {
	if ev == nil {
		panic("event: cannot unread a nil event")
	}
	s.mu.Lock()
	s.queue = append([]Event{ev}, s.queue...)
	s.mu.Unlock()
	s.notifyReady()
}

// EndPaste force-closes a bracketed paste, if one is open.
func (s *Source) EndPaste() {
	s.mu.Lock()
	s.expirePasteLocked()
	s.mu.Unlock()
	s.notifyReady()
}

// Close releases the readiness backend and the descriptors it watches. It does
// not close the input, which the Source does not own. A Source that fell back
// to reading on a goroutine has nothing to release.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closePolling()
	return nil
}

// Events streams events until ctx is done or input reaches EOF.
func (s *Source) Events(ctx context.Context) <-chan Event {
	ch := make(chan Event)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ev, err := s.Read()
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch
}
