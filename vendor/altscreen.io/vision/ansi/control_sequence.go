package ansi

// ControlSequence is the body of a control sequence split into the regions
// ECMA-48 section 5.4 names: an optional private parameter byte, the parameter
// bytes, an optional intermediate byte, and the final byte identifying the
// control function.
//
// The introducer is left out, which is what lets one type serve all three
// places the shape appears. A CSI body ends at the final byte, a DCS carries
// its payload after it, and a DECRPSS setting spells the sequence out the same
// way without an introducer at all, so `2 q` is DECSCUSR reporting a steady
// block.
type ControlSequence struct {
	private      byte
	params       Params
	intermediate byte
	final        byte
}

// ParseControlSequence reads a leading control sequence from body and returns
// it with the bytes that follow its final byte.
//
// The tail is empty for a CSI body, carries the payload for a DCS, and is a
// sign of misreading for a setting; a caller that expects nothing to follow
// checks that itself.
//
// The last result is false when body holds no final byte, when a byte falls
// outside the region it appears in, or when more than one intermediate is
// present. Every control function in use carries at most one, so a second means
// this is not a sequence worth recognizing, and answering with either byte
// would be a guess.
func ParseControlSequence(body []byte) (ControlSequence, []byte, bool) {
	var cs ControlSequence
	rest := body
	if len(rest) > 0 && rest[0] >= 0x3c && rest[0] <= 0x3f {
		cs.private = rest[0]
		rest = rest[1:]
	}
	mid := len(rest)
	for i, b := range rest {
		if b < 0x30 || b > 0x3b {
			mid = i
			break
		}
	}
	cs.params = ParamsFromRaw(rest[:mid])
	finalPos := mid
	if mid < len(rest) && rest[mid] >= 0x20 && rest[mid] <= 0x2f {
		cs.intermediate = rest[mid]
		finalPos = mid + 1
	}
	if finalPos >= len(rest) {
		return ControlSequence{}, nil, false
	}
	cs.final = rest[finalPos]
	if cs.final < 0x40 || cs.final > 0x7e {
		return ControlSequence{}, nil, false
	}
	return cs, rest[finalPos+1:], true
}

// Private returns the first parameter byte when it falls in the private-use
// range 03/12 to 03/15 (<, =, >, ?), where it may only appear, and 0 otherwise.
func (c ControlSequence) Private() byte { return c.private }

// Params returns the parameter bytes between the private byte and the
// intermediate.
func (c ControlSequence) Params() Params { return c.params }

// Intermediate returns the intermediate byte, or 0 when there is none.
func (c ControlSequence) Intermediate() byte { return c.intermediate }

// Final returns the byte identifying the control function.
func (c ControlSequence) Final() byte { return c.final }
