//go:build !visiontrace

// Package trace tees the wire bytes to a file so a session can be replayed and
// read after the fact. It sits behind the visiontrace build tag; without that
// tag these stubs compile away, so the call sites cost nothing.
//
// See the tagged build of this package for the variables it reads and the
// format it writes.
package trace

// Output records bytes on their way to the terminal. It does nothing unless
// the visiontrace build tag is set.
func Output([]byte) {}

// Input records bytes on their way from the terminal to the decoder. It does
// nothing unless the visiontrace build tag is set.
func Input([]byte) {}
