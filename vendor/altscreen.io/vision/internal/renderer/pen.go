package renderer

import (
	"io"

	"altscreen.io/vision/buffer"
	"altscreen.io/vision/style"
)

func (r *Renderer) updatePen(w io.Writer, cell *buffer.Cell) error {
	target := style.Empty
	if cell != nil {
		target = cell.Style
	}
	if r.cur.style.Equal(target) {
		return nil
	}
	from := r.colorProfile.ConvertStyle(r.cur.style)
	to := r.colorProfile.ConvertStyle(target)
	var wrote bool
	r.emitBuf, wrote = style.AppendDiff(r.emitBuf[:0], from, to)
	if wrote {
		if err := r.writeEmit(w); err != nil {
			return err
		}
	}
	r.cur.setStyle(target)
	return nil
}

func (r *Renderer) resetPen(w io.Writer) error { return r.updatePen(w, nil) }

// writeEmit sends whatever the caller staged in emitBuf to w, writing nothing
// when the buffer came out empty, which is what a zero-count movement does.
func (r *Renderer) writeEmit(w io.Writer) error {
	if len(r.emitBuf) == 0 {
		return nil
	}
	_, err := w.Write(r.emitBuf)
	r.emitBuf = r.emitBuf[:0]
	return err
}
