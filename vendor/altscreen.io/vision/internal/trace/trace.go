//go:build visiontrace

// Package trace tees the wire bytes to a file so a session can be replayed
// and read after the fact.
//
// Two destinations, each enabled by setting its variable to a file path:
//
//   - VISION_OUTPUT_TRACE receives one entry per output flush, holding the
//     whole staged buffer in flush order: frames, control sequences, and any
//     raw writes a caller staged through the screen.
//   - VISION_INPUT_TRACE receives one entry per decoder parse.
//
// Entries append, and control bytes are escaped so the file stays readable
// without splitting a multibyte glyph.
//
// The package is behind the visiontrace build tag, so an ordinary build
// compiles the no-op stubs and pays nothing:
//
//	go build -tags visiontrace ./...
package trace

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// The destination is read once: a trace follows the process, and re-reading
// the environment per flush would cost more than the write it guards.
var (
	outputPath = sync.OnceValue(func() string { return os.Getenv("VISION_OUTPUT_TRACE") })
	inputPath  = sync.OnceValue(func() string { return os.Getenv("VISION_INPUT_TRACE") })

	outputIndex atomic.Uint64
	inputIndex  atomic.Uint64
)

// Output records bytes on their way to the terminal.
func Output(b []byte) {
	if len(b) == 0 {
		return
	}
	if path := outputPath(); path != "" {
		appendEntry(path, "flush", outputIndex.Add(1)-1, b)
	}
}

// Input records bytes on their way from the terminal to the decoder.
func Input(b []byte) {
	if len(b) == 0 {
		return
	}
	if path := inputPath(); path != "" {
		appendEntry(path, "input", inputIndex.Add(1)-1, b)
	}
}

// appendEntry writes one labelled record. The file is reopened per entry so a
// trace survives a crash, which is the case it most often has to explain.
func appendEntry(path, label string, index uint64, b []byte) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	fmt.Fprintf(f, "--- %s %d (%d bytes) ---\n%s\n", label, index, len(b), escape(b))
}

// escape renders b as a Go quoted string with the quotes removed. Printable
// text passes through as itself, multibyte runes included, so the trace reads
// as the terminal sees it, and everything else is escaped so no byte is lost.
//
// ESC comes out as \x1b rather than the \e that terminal traces often use.
// Rewriting it would have to know which backslashes strconv already escaped:
// a stream containing the literal text \x1b quotes to \\x1b, and a blind
// replacement would turn that into \\e and lose the original bytes.
func escape(b []byte) string {
	q := strconv.Quote(string(b))
	return q[1 : len(q)-1]
}
