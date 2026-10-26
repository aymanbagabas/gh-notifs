//go:build windows

package poll

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// maxWait is the handle limit of WaitForMultipleObjects. x/sys/windows does not
// define MAXIMUM_WAIT_OBJECTS.
const maxWait = 64

// waitPoller waits with WaitForMultipleObjects, which names only the first
// signaled handle. The rest are probed with zero-timeout single waits so one
// call reports everything ready, matching the per-index contract the other
// backends provide.
type waitPoller struct {
	handles []windows.Handle
}

// New returns a Poller watching handles. Windows waits on the handles
// themselves, so inputIsTerminal does not affect the choice.
func New(handles []Handle, inputIsTerminal bool) (Poller, error) {
	if err := validate(handles); err != nil {
		return nil, err
	}
	if len(handles) > maxWait {
		return nil, fmt.Errorf("poll: %d handles exceeds the %d WaitForMultipleObjects supports", len(handles), maxWait)
	}
	hs := make([]windows.Handle, len(handles))
	for i, h := range handles {
		hs[i] = windows.Handle(h)
	}
	return &waitPoller{handles: hs}, nil
}

func (p *waitPoller) Poll(ready []bool, timeout time.Duration) (int, error) {
	if err := checkReadyLen(ready, len(p.handles)); err != nil {
		return 0, err
	}
	clear(ready)

	deadline := deadlineFor(timeout)
	for {
		msec := uint32(windows.INFINITE)
		if left, forever := remaining(deadline); !forever {
			msec = windowsMillis(left)
		}
		ev, err := windows.WaitForMultipleObjects(p.handles, false, msec)
		switch {
		case err != nil:
			return 0, err
		case ev == uint32(windows.WAIT_TIMEOUT):
			return 0, nil
		case ev >= windows.WAIT_OBJECT_0+uint32(len(p.handles)):
			return 0, fmt.Errorf("poll: wait returned unexpected status %d", ev)
		}
		first := int(ev - windows.WAIT_OBJECT_0)
		ready[first] = true
		count := 1
		for i, h := range p.handles {
			if i == first {
				continue
			}
			if got, err := windows.WaitForSingleObject(h, 0); err == nil && got == windows.WAIT_OBJECT_0 {
				ready[i] = true
				count++
			}
		}
		return count, nil
	}
}

func (p *waitPoller) Close() error { return nil }
