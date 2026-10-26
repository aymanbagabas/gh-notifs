// Package graphemeaw segments text into Unicode grapheme clusters and reports
// how many terminal cells each cluster occupies.
//
// It applies the same UAX #29 grapheme cluster boundaries as the grapheme
// package, and additionally carries the East Asian Width property of UAX #11
// and a derived monospace width in the same trie entry. Terminal rendering
// needs a cluster's boundary and its width together, so answering both from
// one table keeps measurement off a second lookup path.
package graphemeaw

import (
	"bytes"
	"unicode/utf8"

	"altscreen.io/vision/unicode/internal/segmenter"
)

var trie = newGraphemeawTrie(0)

// breakProperty returns just the cluster break class, which is what the
// segmentation state machine indexes on. The width bits ride along in the
// upper half of the same entry.
func breakProperty(s []byte) (uint8, int) {
	v, sz := trie.lookup(s)
	return uint8(v & classMask), sz
}

var ruleData = segmenter.RuleBreakData{
	PropertyLookup:        breakProperty,
	BreakStateTable:       breakTable[:],
	PropertyCount:         stride,
	LastCodepointProperty: lastCP,
	SOTProperty:           sot,
	EOTProperty:           eot,
}

// Variation selectors that override a pictograph's default presentation.
var (
	textVariation  = []byte("\ufe0e") // VS15, request text presentation
	emojiVariation = []byte("\ufe0f") // VS16, request emoji presentation
)

// Regional indicator symbols, which pair up into flags.
const (
	firstRegionalIndicator = 0x1F1E6
	lastRegionalIndicator  = 0x1F1FF
)

// Options configures grapheme cluster segmentation and measurement.
type Options struct {
	// AmbiguousWide reports code points whose East Asian Width is Ambiguous
	// as two cells rather than one. Terminals configured for a CJK locale
	// generally want this. It applies to both [Segmenter.Width] and
	// [Segmenter.WcWidth].
	//
	// The zero value reports ambiguous code points as one cell.
	AmbiguousWide bool
}

// Segmenter iterates over the grapheme clusters in a byte slice, reporting the
// terminal width of each. The usage pattern is:
//
//	seg := graphemeaw.NewSegmenter(input)
//	for seg.Next() {
//	    fmt.Println(seg.Bytes(), seg.Width())
//	}
type Segmenter struct {
	s             *segmenter.Segmenter
	ambiguousWide bool
}

// NewSegmenter returns a Segmenter that iterates over the grapheme clusters
// in the given input using default options.
func NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{s: segmenter.New(&ruleData, input)}
}

// NewSegmenter returns a Segmenter that iterates over the grapheme clusters
// in the given input, configured by o.
func (o *Options) NewSegmenter(input []byte) *Segmenter {
	return &Segmenter{
		s:             segmenter.New(&ruleData, input),
		ambiguousWide: o.AmbiguousWide,
	}
}

// Next advances to the next grapheme cluster. It returns false when the
// end of input has been reached.
func (g *Segmenter) Next() bool {
	input := g.s.Input()
	pos := g.s.End()

	// ASCII fast path: non-CR/LF ASCII bytes are each their own grapheme
	// cluster (property Other). Emit one directly, skipping the state machine.
	// The next byte must also be ASCII to avoid splitting a base+combining-mark
	// sequence (a non-ASCII follower could be Extend/ZWJ/SpacingMark).
	if pos < len(input) {
		b := input[pos]
		if b < 0x80 && b != '\r' && b != '\n' {
			if pos+1 >= len(input) || input[pos+1] < 0x80 {
				nprop := Other
				if b < 0x20 {
					nprop = Control
				}
				g.s.FastForward(pos+1, uint8(nprop))
				return true
			}
		}
	}
	return g.s.Next()
}

// Bytes returns the current grapheme cluster as a byte slice.
func (g *Segmenter) Bytes() []byte { return g.s.Bytes() }

// Text returns the current grapheme cluster as a string.
func (g *Segmenter) Text() string { return g.s.Text() }

// Position returns the byte offsets [start, end) of the current grapheme
// cluster.
func (g *Segmenter) Position() (start, end int) { return g.s.Position() }

// Width returns the number of terminal cells the current grapheme cluster
// occupies, measuring the cluster as a whole.
//
// The measurement is cluster-aware where that changes terminal layout: it
// honours text and emoji variation selectors, treats a regional indicator as a
// flag, and otherwise takes the width of the base code point, since combining
// marks, joiners, and variation selectors in the tail add no cells.
func (g *Segmenter) Width() int {
	return graphemeWidth(g.s.Bytes(), g.ambiguousWide)
}

// WcWidth returns the number of terminal cells the current grapheme cluster
// occupies, as the sum of its code-point widths, as in wcswidth(3).
//
// It is deliberately cluster-blind: variation selectors, joiners, and emoji
// presentation rules do not change the result. Each printable code point adds
// its own width. A joined family emoji can therefore occupy eight columns.
func (g *Segmenter) WcWidth() int {
	cluster := g.s.Bytes()
	total := 0
	for len(cluster) > 0 {
		width, size := wcWidth(cluster, g.ambiguousWide)
		total += width
		cluster = cluster[size:]
	}
	return total
}

// wcWidth returns the first code point's cell width and decoded byte size.
func wcWidth(cluster []byte, ambiguousWide bool) (width, size int) {
	if len(cluster) == 0 {
		return 0, 0
	}
	entry, sz := trie.lookup(cluster)
	r, size := utf8.DecodeRune(cluster)
	// C0 controls, DEL, and C1 controls occupy no cells.
	if sz == 0 || r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return 0, size
	}
	return Properties{entry: entry}.Width(ambiguousWide), size
}

// graphemeWidth measures a cluster as a whole. An empty cluster is zero cells.
func graphemeWidth(cluster []byte, ambiguousWide bool) int {
	if len(cluster) == 0 {
		return 0
	}
	entry, sz := trie.lookup(cluster)
	if sz == 0 {
		return 0
	}
	r, _ := utf8.DecodeRune(cluster)
	p := Properties{entry: entry}

	// Printable ASCII, by far the common case.
	if r >= 0x20 && r < 0x7f {
		return 1
	}
	// C0 controls, DEL, and C1 controls occupy no cells.
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return 0
	}
	// A regional indicator forms a flag. Terminals render one two cells wide
	// even when it is not followed by its pair.
	if r >= firstRegionalIndicator && r <= lastRegionalIndicator {
		return 2
	}
	if p.IsDefaultIgnorable() {
		return 0
	}
	if p.Class() == Extended_Pictographic {
		// An explicit variation selector wins over the default presentation.
		if bytes.Contains(cluster, emojiVariation) {
			return 2
		}
		if bytes.Contains(cluster, textVariation) {
			return 1
		}
		if p.IsEmojiPresentation() {
			return 2
		}
		return 1
	}
	return p.Width(ambiguousWide)
}
