package ansi

import "strconv"

// SetModifyOtherKeys1 enables modifyOtherKeys resource 4 at value 1: ESC [ > 4 ; 1 m.
const SetModifyOtherKeys1 = "\x1b[>4;1m"

// SetModifyOtherKeys2 enables modifyOtherKeys resource 4 at value 2: ESC [ > 4 ; 2 m.
const SetModifyOtherKeys2 = "\x1b[>4;2m"

// ResetModifyOtherKeys resets modifyOtherKeys resource 4: ESC [ > 4 m.
const ResetModifyOtherKeys = "\x1b[>4m"

// QueryModifyOtherKeys queries modifyOtherKeys resource 4: ESC [ ? 4 m.
const QueryModifyOtherKeys = "\x1b[?4m"

// SetKeyModifierOptions sets or resets a key-modifier resource with ESC [ > resource [ ; value ] m.
func SetKeyModifierOptions(resource int, value *int) string {
	out := "\x1b[>" + strconv.Itoa(resource)
	if value != nil {
		out += ";" + strconv.Itoa(*value)
	}
	return out + "m"
}

// QueryKeyModifierOptions queries a key-modifier resource with ESC [ ? resource m.
func QueryKeyModifierOptions(resource int) string {
	return "\x1b[?" + strconv.Itoa(resource) + "m"
}
