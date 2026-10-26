package text

import (
	"io"
	"strings"

	"altscreen.io/vision/buffer"
	"altscreen.io/vision/color"
	"altscreen.io/vision/style"
)

// Write writes s to w as terminal text and escape sequences, converting every
// cell's colors to p first.
//
// One row is written per line, separated by CRLF with no trailing newline
// after the final row. Write assumes w starts in the default style; it does
// not emit a reset before the first row, so pre-existing SGR state or a
// hyperlink on w can bleed into it. Each row ends in the default style, so
// any open SGR state or hyperlink is closed at the row's end. Trailing blank
// cells are dropped only when they carry no style; a styled blank cell is
// retained.
func Write(w io.Writer, s buffer.Surface, p color.Profile) (int, error) {
	bounds := s.Bounds()
	def := style.Empty
	total := 0
	write := func(str string) error {
		n, err := io.WriteString(w, str)
		total += n
		return err
	}
	writeDiff := func(from, to style.Style) error {
		n, err := style.WriteDiff(w, from, to)
		total += n
		return err
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if y > bounds.Min.Y {
			if err := write("\r\n"); err != nil {
				return total, err
			}
		}
		last := -1
		for x := bounds.Max.X - 1; x >= bounds.Min.X; x-- {
			cell := s.CellAt(x, y)
			if cell != nil && (!cell.IsBlank() || !style.Convert(cell.Style, p).IsEmpty()) {
				last = x
				break
			}
		}
		if last < 0 {
			continue
		}
		pen := def
		for x := bounds.Min.X; x <= last; x++ {
			cell := s.CellAt(x, y)
			if cell == nil || cell.IsContinuation() {
				continue
			}
			to := style.Convert(cell.Style, p)
			if err := writeDiff(pen, to); err != nil {
				return total, err
			}
			pen = to
			content := cell.Content()
			if content == "" {
				content = strings.Repeat(" ", cell.Width())
			}
			if err := write(content); err != nil {
				return total, err
			}
		}
		if err := writeDiff(pen, def); err != nil {
			return total, err
		}
	}
	return total, nil
}

// String returns s as terminal text and escape sequences, converting every
// cell's colors to p first. It is [Write] against a string builder.
func String(s buffer.Surface, p color.Profile) string {
	var b strings.Builder
	_, _ = Write(&b, s, p)
	return b.String()
}
