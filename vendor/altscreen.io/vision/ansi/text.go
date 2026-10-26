package ansi

import (
	"iter"
	"unicode/utf8"

	"altscreen.io/vision/unicode/graphemeaw"
)

// TokenKind is the class of an ANSI-aware text token.
type TokenKind uint8

const (
	// TokenText is one whole UTF-8 grapheme cluster and its display width.
	TokenText TokenKind = iota
	// TokenEscape is an ANSI escape or control string with zero display width.
	TokenEscape
	// TokenControl is one standalone control or malformed byte with zero width.
	TokenControl
)

// Token is one item produced by Tokenize.
type Token struct {
	Kind    TokenKind
	Bytes   []byte
	Width   int
	Control byte
}

// StringWidth returns the display width of bytes, ignoring ANSI escapes.
func StringWidth(bytes []byte, mode WidthMethod, ambiguousWide bool) int {
	total := 0
	tokens := Tokenize(bytes, mode, ambiguousWide)
	for {
		tok, ok := tokens.Next()
		if !ok {
			break
		}
		if tok.Kind == TokenText {
			total += tok.Width
		}
	}
	return total
}

// Tokenize splits bytes into text, escape, and control tokens.
func Tokenize(bytes []byte, mode WidthMethod, ambiguousWide bool) *Tokenizer {
	return &Tokenizer{bytes: bytes, mode: mode, ambiguousWide: ambiguousWide}
}

// Tokenizer iterates over ANSI-aware tokens.
type Tokenizer struct {
	bytes         []byte
	pos           int
	mode          WidthMethod
	ambiguousWide bool
	scanEnd       int
	run           string
	scanned       int
	validated     int
}

// Next returns the next token.
func (t *Tokenizer) Next() (Token, bool) {
	if t.pos >= len(t.bytes) {
		return Token{}, false
	}
	b := t.bytes[t.pos]
	// Printable ASCII followed by ASCII or EOF is a complete cluster.
	if b >= 0x20 && b < 0x7f && (t.pos+1 == len(t.bytes) || t.bytes[t.pos+1] < 0x80) {
		start := t.pos
		t.pos++
		if t.run != "" {
			t.run = t.run[1:]
		}
		return Token{Kind: TokenText, Bytes: t.bytes[start:t.pos], Width: 1}, true
	}
	if b == 0x1b || isC1Introducer(b) {
		start := t.pos
		end := scanSequence(t.bytes, start)
		t.pos = end
		return Token{Kind: TokenEscape, Bytes: t.bytes[start:end]}, true
	}
	// A cached run needs no further control checks or text scans.
	if t.run == "" {
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.pos++
			return Token{Kind: TokenControl, Control: b}, true
		}
		if t.pos >= t.scanEnd {
			end := t.pos
			for end < len(t.bytes) {
				bb := t.bytes[end]
				if bb == 0x1b || bb < 0x20 || bb == 0x7f || (bb >= 0x80 && bb <= 0x9f) {
					break
				}
				n := utf8CharAt(t.bytes, end)
				if n == 0 {
					break
				}
				end += n
			}
			if end > t.pos+1 {
				t.scanned += end - t.pos
			} else {
				t.scanned++
			}
			t.scanEnd = end
			t.validate(end)
		}
		if t.run == "" {
			t.pos++
			return Token{Kind: TokenControl, Control: b}, true
		}
	}
	opts := graphemeaw.Options{AmbiguousWide: t.ambiguousWide}
	seg := opts.NewSegmenter([]byte(t.run))
	if !seg.Next() {
		return Token{}, false
	}
	g := seg.Bytes()
	start := t.pos
	t.pos += len(g)
	t.run = t.run[len(g):]
	// Each measure walks the cluster from the start, so only ask for the one
	// the mode wants.
	var w int
	if t.mode == GraphemeWidth {
		w = seg.Width()
	} else {
		w = seg.WcWidth()
	}
	return Token{Kind: TokenText, Bytes: t.bytes[start:t.pos], Width: w}, true
}

func (t *Tokenizer) validate(to int) {
	from := t.pos
	t.validated += to - from
	if utf8.Valid(t.bytes[from:to]) {
		t.run = string(t.bytes[from:to])
	} else {
		t.run = ""
	}
}

// All returns an iterator over the remaining tokens. It advances the
// Tokenizer, so a partially consumed one yields only what is left.
func (t *Tokenizer) All() iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for {
			tok, ok := t.Next()
			if !ok || !yield(tok) {
				return
			}
		}
	}
}

func utf8CharAt(bytes []byte, i int) int {
	n := utf8CharLen(bytes[i])
	if n == 0 || i+n > len(bytes) {
		return 0
	}
	if n > 1 && !utf8.Valid(bytes[i:i+n]) {
		return 0
	}
	return n
}

func utf8CharLen(b byte) int {
	switch {
	case b <= 0x7f:
		return 1
	case b >= 0xc2 && b <= 0xdf:
		return 2
	case b >= 0xe0 && b <= 0xef:
		return 3
	case b >= 0xf0 && b <= 0xf4:
		return 4
	default:
		return 0
	}
}

func isC1Introducer(b byte) bool {
	switch b {
	case 0x90, 0x98, 0x9b, 0x9d, 0x9e, 0x9f:
		return true
	}
	return false
}

func scanSequence(bytes []byte, start int) int {
	if bytes[start] == 0x1b {
		i := start + 1
		if i >= len(bytes) {
			return len(bytes)
		}
		switch bytes[i] {
		case '[':
			return scanCSI(bytes, i+1)
		case ']':
			return scanString(bytes, i+1, true)
		case 'P', 'X', '^', '_':
			return scanString(bytes, i+1, false)
		default:
			return scanEscIntermediate(bytes, i)
		}
	}
	switch bytes[start] {
	case 0x9b:
		return scanCSI(bytes, start+1)
	case 0x9d:
		return scanString(bytes, start+1, true)
	case 0x90, 0x98, 0x9e, 0x9f:
		return scanString(bytes, start+1, false)
	default:
		return start + 1
	}
}

func scanCSI(bytes []byte, from int) int {
	for i := from; i < len(bytes); i++ {
		if bytes[i] >= 0x40 && bytes[i] <= 0x7e {
			return i + 1
		}
	}
	return len(bytes)
}

func scanString(bytes []byte, from int, belEnds bool) int {
	for i := from; i < len(bytes); {
		b := bytes[i]
		if b == 0x9c || (b == 0x07 && belEnds) {
			return i + 1
		}
		if b == 0x1b && i+1 < len(bytes) && bytes[i+1] == '\\' {
			return i + 2
		}
		if b == 0x1b {
			return i
		}
		if n := utf8CharAt(bytes, i); n > 0 {
			i += n
		} else {
			i++
		}
	}
	return len(bytes)
}

func scanEscIntermediate(bytes []byte, at int) int {
	i := at
	for i < len(bytes) && bytes[i] >= 0x20 && bytes[i] <= 0x2f {
		i++
	}
	if i < len(bytes) {
		if n := utf8CharAt(bytes, i); n > 0 {
			return i + n
		}
		return i + 1
	}
	return len(bytes)
}

// All calls below are same-package helpers for the text utilities.
func tokenBytesString(b []byte) string { return string(b) }
