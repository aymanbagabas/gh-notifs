//go:build !windows

package event

import "io"

// pumpInput copies r into ch. Off Windows the terminal delivers a byte stream,
// so this is the plain reader loop with no console-record translation.
func pumpInput(r io.Reader, ch chan<- readResult) {
	readInput(r, ch)
}
