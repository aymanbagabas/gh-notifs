//go:build windows

package term

// The console has separate names for its input and output halves.
const (
	ttyInputName  = "CONIN$"
	ttyOutputName = "CONOUT$"
)
