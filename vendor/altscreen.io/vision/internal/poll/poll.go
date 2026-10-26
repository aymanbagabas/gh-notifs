// Package poll waits for read readiness on a small, fixed set of handles.
//
// An event source waits on terminal input, a wake handle, and on Unix a resize
// handle. Each backend registers those handles once and reports readiness back
// through a caller-owned slice indexed by registration order:
//
//	handles -> New -> Poller.Poll -> ready[i]
//
// The backend is chosen per target: epoll on Linux, kqueue on the BSDs,
// WaitForMultipleObjects on Windows, and select(2) elsewhere. Darwin uses
// select for a terminal input handle because kqueue wakes repeatedly on a tty
// character device, and reaches for kqueue otherwise. Unix targets with no
// native backend fall back to select rather than poll(2), because poll does not
// report readiness for tty character devices everywhere, notably on illumos and
// Solaris.
//
// Every backend reports a hangup or error on a watched handle as readiness, so
// the caller's read path surfaces the underlying EOF or error, and recomputes
// the remaining time from an absolute deadline when a wait is interrupted.
//
// A Poller does not own the handles it watches. The caller must keep them open
// for the Poller's lifetime.
package poll

import (
	"errors"
	"time"
)

// Handle is a watched handle: a file descriptor on Unix, a HANDLE on Windows.
// It is the width of [os.File.Fd] so a caller can register one without a
// conversion.
type Handle uintptr

// ErrNoHandles is returned by New when given no handles to watch.
var ErrNoHandles = errors.New("poll: at least one handle is required")

// ErrReadyLen is returned by Poll when the readiness slice is not the length of
// the registered handle set, which would put readiness out of step with the
// handles it reports on.
var ErrReadyLen = errors.New("poll: readiness length does not match the registered handles")

// Poller waits for read readiness on the handles registered when it was
// constructed. The set is fixed for the Poller's lifetime.
//
// Poll writes readiness into ready, which stays owned by the caller: a slice
// shares its backing array, so the elements the caller passed in are the ones
// that get set, in the order the handles were registered. Every element is
// reset first, so a stale value from an earlier call cannot leak through. ready
// must be exactly as long as the registered handle set, and Poll returns
// [ErrReadyLen] otherwise rather than reporting against the wrong handle.
//
// Poll returns how many handles became ready, so an all-false slice and a zero
// count mean the wait timed out. A negative timeout blocks until a handle is
// ready, and a zero timeout polls without blocking. Poll is safe to call from
// multiple goroutines.
//
// Reusing one slice across calls is the intended pattern, and is why readiness
// is reported this way rather than returned: waiting for input should not
// allocate.
type Poller interface {
	Poll(ready []bool, timeout time.Duration) (int, error)
	Close() error
}

// validate rejects an empty handle set, which no backend can wait on.
func validate(handles []Handle) error {
	if len(handles) == 0 {
		return ErrNoHandles
	}
	return nil
}

// checkReadyLen keeps per-index reporting in step with the registered handles.
func checkReadyLen(ready []bool, registered int) error {
	if len(ready) != registered {
		return ErrReadyLen
	}
	return nil
}

// deadlineFor turns a timeout into an absolute deadline, returning the zero
// time for a negative timeout, which means block until a handle is ready. A
// wait interrupted by a signal resumes against the deadline rather than
// restarting the original timeout.
func deadlineFor(timeout time.Duration) time.Time {
	if timeout < 0 {
		return time.Time{}
	}
	return time.Now().Add(timeout)
}

// remaining reports the time left until deadline. It reports forever for the
// zero deadline, and zero once the deadline has passed, so a caller polls
// without blocking rather than waiting again.
func remaining(deadline time.Time) (d time.Duration, forever bool) {
	if deadline.IsZero() {
		return 0, true
	}
	left := time.Until(deadline)
	if left < 0 {
		left = 0
	}
	return left, false
}
