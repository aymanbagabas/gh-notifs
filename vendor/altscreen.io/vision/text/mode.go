// Package text paints and encodes styled text on terminal cell surfaces.
package text

import (
	"iter"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/unicode/graphemeaw"
)

// GraphemeCells iterates the grapheme clusters of s with the cell width of
// each, measured by mode.
//
// Each cluster is a slice of s rather than a fresh string, so painting a line
// costs no allocation per cell. Callers must not assume the cluster outlives s,
// which is the usual rule for a string slice.
func GraphemeCells(s string, mode ansi.WidthMethod, eawWide bool) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		seg := (&graphemeaw.Options{AmbiguousWide: eawWide}).NewSegmenter([]byte(s))
		for seg.Next() {
			var w int
			// Each measure walks the cluster from the start, so only ask for
			// the one the mode wants.
			if mode == ansi.GraphemeWidth {
				w = seg.Width()
			} else {
				w = seg.WcWidth()
			}
			// The segmenter reports offsets into the bytes it was handed, which
			// are a copy of s, so they index s just as well. Slicing s shares
			// its memory where Text would allocate a string per cluster.
			start, end := seg.Position()
			if !yield(s[start:end], w) {
				return
			}
		}
	}
}

func stringWidth(s string, mode ansi.WidthMethod, eawWide bool) int {
	total := 0
	for _, w := range GraphemeCells(s, mode, eawWide) {
		total += w
	}
	return total
}
