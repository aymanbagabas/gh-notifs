// Package windows binds the Windows console input API that
// golang.org/x/sys/windows does not wrap.
//
// x/sys/windows covers the console mode, screen buffer, and read and write
// calls, but not ReadConsoleInputW or the INPUT_RECORD family, which is how a
// console reports mouse, resize, and focus activity. Everything else should
// come from x/sys/windows rather than be redeclared here.
//
// The record types and their constants carry no build tag, so code that turns
// records into something else stays testable on every target. Only the system
// calls are Windows-only.
package windows

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go syscall_windows.go
