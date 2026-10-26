// Package segmenter implements a state machine engine for Unicode text
// segmentation (UAX #29 and UAX #14). It is shared by the grapheme, word,
// sentence, and line break packages.
//
// The engine is data-driven: each segmenter type supplies a [RuleBreakData]
// containing property lookup tables and a pre-built break state table.
// The table is an N×N matrix (left-property × right-property → action),
// where actions are break, keep, no-match (rewind), or enter-combined-state.
//
// Combined states come in two flavours:
//   - Index states (0–119) enter a combined state. Marker movement
//     is controlled by [RuleBreakData.LastCodepointProperty]: the marker only
//     advances when the previous state index ≤ LastCodepointProperty.
//   - Intermediate states (120–252) always advance the rewind point,
//     regardless of LastCodepointProperty. Encoded as property index + 120.
package segmenter

// BreakState is the element type of a break state table cell.
// Encoding matches ICU4X: Index 0–119, Intermediate 120–252,
// Break 253, NoMatch 254, Keep 255.
type BreakState = uint8

const (
	Break   BreakState = 253
	NoMatch BreakState = 254
	Keep    BreakState = 255

	intermediateOffset BreakState = 120
)

// isIntermediate reports whether state is an Intermediate combined state.
func isIntermediate(s uint8) bool {
	return s >= intermediateOffset && s < Break
}

// isCombinedState reports whether state is any combined state (Index or Intermediate).
func isCombinedState(s uint8) bool {
	return s < Break
}

// stateIndex extracts the combined-state property index from a combined
// state (either Index or Intermediate).
func stateIndex(s uint8) uint8 {
	if s >= intermediateOffset {
		return s - intermediateOffset
	}
	return s
}

// IndexState returns the BreakState encoding for an Index combined state.
func IndexState(prop uint8) BreakState { return prop }

// IntermediateState returns the BreakState encoding for an Intermediate combined state.
func IntermediateState(prop uint8) BreakState { return prop + intermediateOffset }

// IsIndex reports whether state is an Index combined state (not Intermediate).
func IsIndex(s uint8) bool {
	return s < intermediateOffset
}

// IsIntermediate reports whether state is an Intermediate combined state.
func IsIntermediate(s uint8) bool {
	return isIntermediate(s)
}

// IndexValue extracts the property index from a combined state.
func IndexValue(s uint8) uint8 {
	return stateIndex(s)
}

// RuleBreakData holds the generated tables for one segmenter type.
// This is the runtime data structure populated by generated tables.go files.
type RuleBreakData struct {
	PropertyLookup        func([]byte) (uint8, int)
	BreakStateTable       []uint8
	PropertyCount         uint8
	LastCodepointProperty uint8
	SOTProperty           uint8
	EOTProperty           uint8
	// ComplexProp is the SA (South-East Asian) property index, used to
	// identify codepoints that require dictionary-based segmentation.
	// Not yet consumed at runtime; reserved for future complex-script support.
	ComplexProp uint8
}

// Segmenter iterates over segments in text.
type Segmenter struct {
	data           *RuleBreakData
	input          []byte
	start          int
	end            int
	boundaryProp   uint8
	overrideLookup func(uint8, rune) uint8
}

// New returns a Segmenter that iterates over segments in input
// according to data.
func New(data *RuleBreakData, input []byte) *Segmenter {
	return &Segmenter{data: data, input: input}
}

// SetOverrideLookup sets an optional property override function.
// The override receives the base property (from the trie) and the decoded
// rune, and returns the effective property. To leave a codepoint unchanged,
// return the base property as-is.
func (s *Segmenter) SetOverrideLookup(fn func(uint8, rune) uint8) {
	s.overrideLookup = fn
}

// lookupProperty returns the break property and byte size for the codepoint
// at the start of input. If an override is set, it is called with the base
// property and decoded rune to produce the effective property.
func (s *Segmenter) lookupProperty(input []byte) (uint8, int) {
	prop, sz := s.data.PropertyLookup(input)
	if s.overrideLookup != nil {
		prop = s.overrideLookup(prop, decodeRune(input, sz))
	}
	return prop, sz
}

// Next advances to the next segment.
func (s *Segmenter) Next() bool {
	if s.end >= len(s.input) {
		return false
	}

	s.start = s.end

	var leftProp uint8
	if s.end == 0 {
		leftProp = s.data.SOTProperty
		rightProp, size := s.lookupProperty(s.input[s.end:])
		state := s.data.BreakStateTable[int(leftProp)*int(s.data.PropertyCount)+int(rightProp)]
		s.end += size
		switch state {
		case Break:
			s.boundaryProp = leftProp
			return true
		case Keep:
			leftProp = rightProp
		default:
			if isCombinedState(state) {
				leftProp = stateIndex(state)
			} else {
				leftProp = rightProp
			}
		}
	} else {
		var size int
		leftProp, size = s.lookupProperty(s.input[s.end:])
		s.end += size
	}

	marker := s.end
	markerLeftProp := leftProp

	for s.end < len(s.input) {
		rightProp, size := s.lookupProperty(s.input[s.end:])
		state := s.data.BreakStateTable[int(leftProp)*int(s.data.PropertyCount)+int(rightProp)]

		switch state {
		case Break:
			if s.end == s.start {
				s.end += size
			}
			s.boundaryProp = leftProp
			return true

		case Keep:
			leftProp = rightProp
			s.end += size
			marker = s.end
			markerLeftProp = rightProp

		case NoMatch:
			s.end = marker
			s.boundaryProp = markerLeftProp
			if s.end == s.start {
				_, sz := s.lookupProperty(s.input[s.end:])
				s.end += sz
			}
			return true

		default:
			idx := stateIndex(state)
			if isIntermediate(state) {
				marker = s.end + size
				if leftProp <= s.data.LastCodepointProperty {
					markerLeftProp = idx
				}
			} else {
				if leftProp <= s.data.LastCodepointProperty {
					marker = s.end
					markerLeftProp = idx
				}
			}
			leftProp = idx
			s.end += size
		}
	}

	eotState := s.data.BreakStateTable[int(leftProp)*int(s.data.PropertyCount)+int(s.data.EOTProperty)]
	if eotState == NoMatch {
		s.boundaryProp = markerLeftProp
		s.end = marker
		if s.end == s.start {
			s.end = len(s.input)
		}
	} else {
		s.boundaryProp = leftProp
	}
	return true
}

// Bytes returns the current segment as a byte slice.
func (s *Segmenter) Bytes() []byte {
	return s.input[s.start:s.end]
}

// Text returns the current segment as a string.
func (s *Segmenter) Text() string {
	return string(s.input[s.start:s.end])
}

// Position returns the byte offsets [start, end) of the current segment.
func (s *Segmenter) Position() (start, end int) {
	return s.start, s.end
}

// BoundaryProperty returns the property index of the left side at the break point.
func (s *Segmenter) BoundaryProperty() uint8 {
	return s.boundaryProp
}

// End returns the end position of the last segment.
func (s *Segmenter) End() int { return s.end }

// SetEnd sets the end position.
func (s *Segmenter) SetEnd(pos int) { s.end = pos }

// SetStart sets the start position of the current segment.
func (s *Segmenter) SetStart(pos int) { s.start = pos }

// Input returns the input byte slice.
func (s *Segmenter) Input() []byte { return s.input }

// FastForward sets the current segment without running the state machine.
func (s *Segmenter) FastForward(end int, prop uint8) {
	s.start = s.end
	s.end = end
	s.boundaryProp = prop
}

// decodeRune extracts the rune from input given its UTF-8 byte length sz.
// This avoids importing unicode/utf8 and re-scanning leading bytes that
// PropertyLookup already consumed.
func decodeRune(b []byte, sz int) rune {
	switch sz {
	case 1:
		return rune(b[0])
	case 2:
		return rune(b[0]&0x1F)<<6 | rune(b[1]&0x3F)
	case 3:
		return rune(b[0]&0x0F)<<12 | rune(b[1]&0x3F)<<6 | rune(b[2]&0x3F)
	case 4:
		return rune(b[0]&0x07)<<18 | rune(b[1]&0x3F)<<12 | rune(b[2]&0x3F)<<6 | rune(b[3]&0x3F)
	default:
		return '\uFFFD'
	}
}
