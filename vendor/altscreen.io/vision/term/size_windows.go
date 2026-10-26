//go:build windows

package term

import "golang.org/x/sys/windows"

// GetWinsize returns the size of the console window attached to f.
//
// Pixel dimensions are unavailable on this platform and come back as 0. The
// size reported is the visible window, not the console screen buffer, which is
// often taller.
//
// It returns an error when f is not a console output handle.
func GetWinsize(f File) (Winsize, error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(f.Fd()), &info); err != nil {
		return Winsize{}, err
	}
	col := int(info.Window.Right) - int(info.Window.Left) + 1
	row := int(info.Window.Bottom) - int(info.Window.Top) + 1
	return Winsize{Row: max(row, 0), Col: max(col, 0)}, nil
}
