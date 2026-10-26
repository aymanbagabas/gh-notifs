//go:build windows

package event

import (
	"io"

	winapi "altscreen.io/vision/internal/windows"
	"golang.org/x/sys/windows"
)

// consoleReadBatch is how many input records one read drains at most. The call
// blocks until at least one record is available and returns whatever else is
// ready, up to this bound.
const consoleReadBatch = 64

// pumpInput copies r into ch. When r wraps a real console handle it reads
// INPUT_RECORDs and serializes them into the shared byte grammar, which is how
// mouse, resize, and focus reports reach the decoder on Windows. When the
// handle is not a console (a pipe, a file, or redirected input) it falls back
// to the plain reader loop so those inputs keep working unchanged.
func pumpInput(r io.Reader, ch chan<- readResult) {
	f, ok := r.(interface{ Fd() uintptr })
	if !ok {
		readInput(r, ch)
		return
	}
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		readInput(r, ch)
		return
	}
	readConsole(handle, mode&windows.ENABLE_VIRTUAL_TERMINAL_INPUT != 0, ch)
}

// readConsole blocks reading console records and forwards their serialized
// bytes over ch, holding the surrogate, mouse-button, and window-size state the
// serializers thread between records. It closes ch when reading stops.
func readConsole(handle windows.Handle, vtInput bool, ch chan<- readResult) {
	defer close(ch)
	var (
		pendingHighSurrogate [2]uint16
		lastMouseButtons     uint32
		size                 lastSize
		records              [consoleReadBatch]winapi.InputRecord
	)
	for {
		var read uint32
		if err := winapi.ReadConsoleInput(handle, &records[0], uint32(len(records)), &read); err != nil {
			ch <- readResult{err: err}
			return
		}
		var buf []byte
		for _, rec := range records[:read] {
			buf = serializeRecord(rec, vtInput, &pendingHighSurrogate, &lastMouseButtons, &size, buf)
		}
		if len(buf) > 0 {
			ch <- readResult{data: buf}
		}
	}
}
