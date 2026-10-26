package windows

import "golang.org/x/sys/windows"

// NewLazySystemDLL is an alias for windows.NewLazySystemDLL, which the
// generated bindings resolve their procedures through.
var NewLazySystemDLL = windows.NewLazySystemDLL

// Handle is an alias for windows.Handle, so callers need only one import to
// pass a console handle here.
type Handle = windows.Handle

//sys	ReadConsoleInput(console Handle, buf *InputRecord, toread uint32, read *uint32) (err error) = kernel32.ReadConsoleInputW
//sys	PeekConsoleInput(console Handle, buf *InputRecord, toread uint32, read *uint32) (err error) = kernel32.PeekConsoleInputW
//sys	WriteConsoleInput(console Handle, buf *InputRecord, towrite uint32, written *uint32) (err error) = kernel32.WriteConsoleInputW
