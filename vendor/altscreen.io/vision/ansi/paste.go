package ansi

// BracketedPasteStart is the start delimiter for bracketed paste: ESC [ 200 ~.
const BracketedPasteStart = "\x1b[200~"

// BracketedPasteEnd is the end delimiter for bracketed paste: ESC [ 201 ~.
const BracketedPasteEnd = "\x1b[201~"
