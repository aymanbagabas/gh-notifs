package ansi

// Strip returns s with ANSI escape sequences removed.
func Strip(s string) string {
	out := make([]byte, 0, len(s))
	tokens := Tokenize([]byte(s), WcWidth, false)
	for {
		tok, ok := tokens.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case TokenText:
			out = append(out, tok.Bytes...)
		case TokenControl:
			out = append(out, tok.Control)
		}
	}
	return string(out)
}
