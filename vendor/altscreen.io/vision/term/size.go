package term

import "altscreen.io/vision/layout"

// File is what the terminal needs from an open file: its descriptor on Unix,
// or its handle on Windows. [os.File] satisfies it.
type File interface {
	// Fd returns the file descriptor or, on Windows, the handle.
	Fd() uintptr
}

// Winsize holds terminal dimensions in cells and, where the platform reports
// them, pixels.
type Winsize struct {
	// Row is the height in cells.
	Row int
	// Col is the width in cells.
	Col int
	// XPixel is the width in pixels, or 0 when unknown.
	XPixel int
	// YPixel is the height in pixels, or 0 when unknown.
	YPixel int
}

// DefaultWinsize returns the conventional fallback of 80 columns by 24 rows,
// with pixel dimensions unknown.
//
// Use it when [GetWinsize] fails, which happens whenever the output is not a
// terminal.
func DefaultWinsize() Winsize {
	return Winsize{Row: 24, Col: 80}
}

// Size returns the cell dimensions, dropping the pixel ones.
func (w Winsize) Size() layout.Size {
	return layout.Size{Width: w.Col, Height: w.Row}
}
