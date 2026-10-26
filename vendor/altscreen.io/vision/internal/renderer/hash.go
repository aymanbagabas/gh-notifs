package renderer

import (
	"hash/maphash"

	"altscreen.io/vision/buffer"
)

// hashLine hashes a line's contents so scroll detection can compare rows
// cheaply. Only the content is hashed, never the style: a row that scrolled
// keeps its glyphs, and the pen is re-emitted wherever it lands.
//
// The scratch buffer and the seed live on the Renderer so hashing a frame
// allocates nothing. The seed is per-Renderer, which is enough because hashes
// are only ever compared against others from the same Renderer.
func (r *Renderer) hashLine(line []buffer.Cell) uint64 {
	r.hashBuf = r.hashBuf[:0]
	for i := range line {
		r.hashBuf = append(r.hashBuf, line[i].Content()...)
		r.hashBuf = append(r.hashBuf, 0)
	}
	return maphash.Bytes(r.hashSeed, r.hashBuf)
}
