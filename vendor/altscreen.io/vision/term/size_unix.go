//go:build unix

package term

import "golang.org/x/sys/unix"

// GetWinsize returns the size of the terminal attached to f.
//
// It issues a TIOCGWINSZ ioctl and reports the kernel's row, column, and pixel
// fields. Pixel dimensions are 0 when the terminal does not report them.
//
// It returns an error when f is not a terminal.
func GetWinsize(f File) (Winsize, error) {
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return Winsize{}, err
	}
	return Winsize{
		Row:    int(ws.Row),
		Col:    int(ws.Col),
		XPixel: int(ws.Xpixel),
		YPixel: int(ws.Ypixel),
	}, nil
}
