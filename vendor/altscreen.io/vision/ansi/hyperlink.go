package ansi

import (
	"bytes"
	"unicode/utf8"
)

// HyperlinkReset resets the current OSC 8 hyperlink: ESC ] 8 ; ; ESC \.
const HyperlinkReset = "\x1b]8;;\x1b\\"

// Hyperlink begins an OSC 8 hyperlink with ESC ] 8 ; params ; url ESC \.
func Hyperlink(url, params string) string {
	return "\x1b]8;" + params + ";" + url + "\x1b\\"
}

// ParseHyperlink parses an OSC 8 body into params and URL.
func ParseHyperlink(body []byte) (params, url string, ok bool) {
	rest, found := bytes.CutPrefix(body, []byte("8;"))
	if !found {
		return "", "", false
	}
	paramsBytes, urlBytes, found := bytes.Cut(rest, []byte(";"))
	if !found || !utf8.Valid(paramsBytes) || !utf8.Valid(urlBytes) {
		return "", "", false
	}
	return string(paramsBytes), string(urlBytes), true
}
