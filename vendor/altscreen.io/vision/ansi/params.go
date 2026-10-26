package ansi

import "strconv"

// Params is a borrowed view of a CSI or DCS parameter body.
type Params struct {
	raw []byte
}

// EmptyParams is an empty parameter list.
var EmptyParams = Params{}

// ParamsFromRaw wraps raw parameter bytes between the introducer and final byte.
func ParamsFromRaw(raw []byte) Params { return Params{raw: raw} }

// Raw returns the raw parameter bytes exactly as they appeared on the wire.
func (p Params) Raw() []byte { return p.raw }

// IsEmpty reports whether the parameter body has no groups.
func (p Params) IsEmpty() bool { return len(p.raw) == 0 }

// Len returns the number of semicolon-separated groups.
func (p Params) Len() int {
	if len(p.raw) == 0 {
		return 0
	}
	n := 1
	for _, b := range p.raw {
		if b == ';' {
			n++
		}
	}
	return n
}

// Groups returns the semicolon-separated groups in p.
func (p Params) Groups() []Group {
	if len(p.raw) == 0 {
		return nil
	}
	groups := make([]Group, 0, p.Len())
	start := 0
	for i, b := range p.raw {
		if b == ';' {
			groups = append(groups, Group{raw: p.raw[start:i]})
			start = i + 1
		}
	}
	return append(groups, Group{raw: p.raw[start:]})
}

// Group returns the group at idx.
func (p Params) Group(idx int) (Group, bool) {
	if idx < 0 || len(p.raw) == 0 {
		return Group{}, false
	}
	start := 0
	for i := 0; ; i++ {
		end := len(p.raw)
		for j := start; j < len(p.raw); j++ {
			if p.raw[j] == ';' {
				end = j
				break
			}
		}
		if i == idx {
			return Group{raw: p.raw[start:end]}, true
		}
		if end == len(p.raw) {
			return Group{}, false
		}
		start = end + 1
	}
}

// Get returns the first sub-parameter of the group at idx.
func (p Params) Get(idx int) (int, bool) {
	g, ok := p.Group(idx)
	if !ok {
		return 0, false
	}
	return g.First()
}

// GetOr returns the first sub-parameter at idx, or def when absent or empty.
func (p Params) GetOr(idx, def int) int {
	if v, ok := p.Get(idx); ok {
		return v
	}
	return def
}

// SliceFrom returns a Params view starting at group start.
func (p Params) SliceFrom(start int) Params {
	if start <= 0 {
		return p
	}
	cur := p.raw
	for skipped := 0; skipped < start; skipped++ {
		pos := -1
		for i, b := range cur {
			if b == ';' {
				pos = i
				break
			}
		}
		if pos < 0 {
			return EmptyParams
		}
		cur = cur[pos+1:]
	}
	return Params{raw: cur}
}

// String returns the flat parameter list in Rust Debug style.
func (p Params) String() string {
	groups := p.Groups()
	out := "["
	for i, g := range groups {
		if i > 0 {
			out += ", "
		}
		if v, ok := g.First(); ok {
			out += "Some(" + strconv.Itoa(v) + ")"
		} else {
			out += "None"
		}
	}
	return out + "]"
}

// Group is one semicolon-separated group within a Params body.
type Group struct {
	raw []byte
}

// EmptyGroup is a group with one empty sub-parameter slot.
var EmptyGroup = Group{}

// GroupFromRaw wraps raw bytes as a single group.
func GroupFromRaw(raw []byte) Group { return Group{raw: raw} }

// Raw returns the raw bytes between the surrounding semicolon separators.
func (g Group) Raw() []byte { return g.raw }

// IsEmpty reports whether the group's raw body is empty.
func (g Group) IsEmpty() bool { return len(g.raw) == 0 }

// Len returns the number of colon-separated sub-parameter slots.
func (g Group) Len() int {
	if len(g.raw) == 0 {
		return 1
	}
	n := 1
	for _, b := range g.raw {
		if b == ':' {
			n++
		}
	}
	return n
}

// Values returns the colon-separated sub-parameters. A slot that was empty or
// invalid comes back with Ok false.
func (g Group) Values() []OptionalInt {
	vals := make([]OptionalInt, 0, g.Len())
	start := 0
	for i, b := range g.raw {
		if b == ':' {
			vals = append(vals, optionalParam(g.raw[start:i]))
			start = i + 1
		}
	}
	return append(vals, optionalParam(g.raw[start:]))
}

// First returns the first sub-parameter of the group.
func (g Group) First() (int, bool) { return g.Nth(0) }

// Nth returns the n-th sub-parameter of the group.
func (g Group) Nth(i int) (int, bool) {
	if i < 0 {
		return 0, false
	}
	start := 0
	for n := 0; ; n++ {
		end := len(g.raw)
		for j := start; j < len(g.raw); j++ {
			if g.raw[j] == ':' {
				end = j
				break
			}
		}
		if n == i {
			v := parseParam(g.raw[start:end])
			if v == nil {
				return 0, false
			}
			return *v, true
		}
		if end == len(g.raw) {
			return 0, false
		}
		start = end + 1
	}
}

func parseParam(bytes []byte) *int {
	if len(bytes) == 0 {
		return nil
	}
	var acc uint64
	for _, b := range bytes {
		if b < '0' || b > '9' {
			return nil
		}
		acc = acc*10 + uint64(b-'0')
		if acc > 1<<32-1 {
			return nil
		}
	}
	v := int(acc)
	return &v
}

// OptionalInt is a parameter that may be absent.
//
// Terminal parameter lists allow empty slots, which mean "use the default"
// rather than zero, so a plain int cannot represent them.
type OptionalInt struct {
	// Value is the integer value, meaningful only when Ok is true.
	Value int
	// Ok reports whether the parameter was present.
	Ok bool
}

// optionalParam parses one parameter slot, reporting absence rather than a
// zero value for an empty or invalid slot.
func optionalParam(raw []byte) OptionalInt {
	if v := parseParam(raw); v != nil {
		return OptionalInt{Value: *v, Ok: true}
	}
	return OptionalInt{}
}
