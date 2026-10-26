package event

import (
	stdcolor "image/color"

	"altscreen.io/vision/ansi"
)

// The key and mouse events are defined types over [Key] and [Mouse], so they
// do not inherit their methods. Without these, every consumer has to convert
// before it can name or match a key, which is the common case:
//
//	if Key(ev).Matches("ctrl+c") { ... }   // before
//	if ev.Matches("ctrl+c")      { ... }   // with these
//
// Only the reading methods are forwarded. Anything that mutates stays on [Key]
// and [Mouse], where the caller has already said which one it means.

// Key returns the key this event carries.
func (e KeyPressEvent) Key() Key { return Key(e) }

// String returns the key's name, such as "ctrl+c" or "enter".
func (e KeyPressEvent) String() string { return Key(e).String() }

// Matches reports whether the key matches a binding pattern such as "ctrl+c".
func (e KeyPressEvent) Matches(pattern string) bool { return Key(e).Matches(pattern) }

// MatchesAny reports whether the key matches any of the binding patterns.
func (e KeyPressEvent) MatchesAny(patterns ...string) bool { return Key(e).MatchesAny(patterns...) }

// Key returns the key this event carries.
func (e KeyRepeatEvent) Key() Key { return Key(e) }

// String returns the key's name.
func (e KeyRepeatEvent) String() string { return Key(e).String() }

// Matches reports whether the key matches a binding pattern.
func (e KeyRepeatEvent) Matches(pattern string) bool { return Key(e).Matches(pattern) }

// MatchesAny reports whether the key matches any of the binding patterns.
func (e KeyRepeatEvent) MatchesAny(patterns ...string) bool { return Key(e).MatchesAny(patterns...) }

// Key returns the key this event carries.
func (e KeyReleaseEvent) Key() Key { return Key(e) }

// String returns the key's name.
func (e KeyReleaseEvent) String() string { return Key(e).String() }

// Matches reports whether the key matches a binding pattern.
func (e KeyReleaseEvent) Matches(pattern string) bool { return Key(e).Matches(pattern) }

// MatchesAny reports whether the key matches any of the binding patterns.
func (e KeyReleaseEvent) MatchesAny(patterns ...string) bool { return Key(e).MatchesAny(patterns...) }

// Mouse returns the mouse state this event carries.
func (e MouseClickEvent) Mouse() Mouse { return Mouse(e) }

// Mouse returns the mouse state this event carries.
func (e MouseReleaseEvent) Mouse() Mouse { return Mouse(e) }

// Mouse returns the mouse state this event carries.
func (e MouseWheelEvent) Mouse() Mouse { return Mouse(e) }

// Mouse returns the mouse state this event carries.
func (e MouseMoveEvent) Mouse() Mouse { return Mouse(e) }

// Keystroke returns the key as a keystroke, such as "ctrl+c". It is what
// [Key.String] returns; the name exists because matching code reads better
// saying which form it wants, and because it is the spelling terminal
// libraries have settled on.
func (k Key) Keystroke() string { return k.String() }

// Keystroke returns the key as a keystroke, such as "ctrl+c".
func (e KeyPressEvent) Keystroke() string { return Key(e).Keystroke() }

// Keystroke returns the key as a keystroke, such as "ctrl+c".
func (e KeyRepeatEvent) Keystroke() string { return Key(e).Keystroke() }

// Keystroke returns the key as a keystroke, such as "ctrl+c".
func (e KeyReleaseEvent) Keystroke() string { return Key(e).Keystroke() }

// Clipboard returns which selection the content came from.
func (e ClipboardEvent) Clipboard() ClipboardSelection { return e.Selection }

// IsDark reports whether the reported color is dark. An absent color counts as
// dark, since a terminal that will not say is far more often dark than light.
func (e ForegroundColorEvent) IsDark() bool { return isDarkColor(e.Color) }

// IsDark reports whether the reported color is dark.
func (e BackgroundColorEvent) IsDark() bool { return isDarkColor(e.Color) }

// IsDark reports whether the reported color is dark.
func (e CursorColorEvent) IsDark() bool { return isDarkColor(e.Color) }

// isDarkColor reports whether a color's HSL lightness is below half. Lightness
// is (max+min)/2 over the normalized channels, so comparing it against 0.5 is
// the same as comparing max+min against 1, and the hue and saturation the full
// conversion would compute are never read.
func isDarkColor(c stdcolor.Color) bool {
	if c == nil {
		return true
	}
	r, g, b, _ := c.RGBA()
	r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
	return max(r8, max(g8, b8))+min(r8, min(g8, b8)) < 0xff
}

// SupportsKeyDisambiguation reports whether the terminal distinguishes keys
// that would otherwise share an escape sequence.
func (e KittyKeyboardEnhancementsEvent) SupportsKeyDisambiguation() bool {
	return e.Contains(ansi.KittyDisambiguateEscapeCodes)
}

// SupportsEventTypes reports whether the terminal reports key release and
// repeat, not only press.
func (e KittyKeyboardEnhancementsEvent) SupportsEventTypes() bool {
	return e.Contains(ansi.KittyReportEventTypes)
}

// SupportsAlternateKeys reports whether the terminal reports the shifted and
// base layout keys alongside the key itself.
func (e KittyKeyboardEnhancementsEvent) SupportsAlternateKeys() bool {
	return e.Contains(ansi.KittyReportAlternateKeys)
}

// SupportsAllKeysAsEscapeCodes reports whether the terminal reports every key
// as an escape sequence, including those that would otherwise be plain text.
func (e KittyKeyboardEnhancementsEvent) SupportsAllKeysAsEscapeCodes() bool {
	return e.Contains(ansi.KittyReportAllKeysAsEscape)
}

// SupportsAssociatedText reports whether the terminal reports the text a key
// press produces alongside the key.
func (e KittyKeyboardEnhancementsEvent) SupportsAssociatedText() bool {
	return e.Contains(ansi.KittyReportAssociatedText)
}

// Contains reports whether all of the given flags are set.
func (e KittyKeyboardEnhancementsEvent) Contains(f ansi.KittyKeyboardFlags) bool {
	return ansi.KittyKeyboardFlags(e)&f == f
}
