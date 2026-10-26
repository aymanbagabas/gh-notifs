package segmenter

// TableBuilder accumulates rules and produces a flat break state table.
// It uses first-write-wins semantics: Set only writes to cells that have
// not been written yet, so higher-priority rules (applied first) take
// precedence. ForceSet always writes, used by ChainRule, IgnoreRule,
// and OverrideRule which manage their own priority.
type TableBuilder struct {
	stride  int
	table   []uint8
	written []bool
}

// Set writes a value into the table at (left, right) using first-write-wins
// semantics: if the cell has already been written, the call is a no-op.
// This lets higher-priority rules (applied first) take precedence.
func (b *TableBuilder) Set(left, right, value uint8) {
	i := int(left)*b.stride + int(right)
	if b.written[i] {
		return
	}
	b.table[i] = value
	b.written[i] = true
}

// ForceSet writes a value unconditionally, ignoring first-write-wins.
// Used by ChainRule, IgnoreRule, and OverrideRule.
func (b *TableBuilder) ForceSet(left, right, value uint8) {
	i := int(left)*b.stride + int(right)
	b.table[i] = value
	b.written[i] = true
}

// Get reads the current value at (left, right).
func (b *TableBuilder) Get(left, right uint8) uint8 {
	return b.table[int(left)*b.stride+int(right)]
}

func (b *TableBuilder) allProps() []uint8 {
	p := make([]uint8, b.stride)
	for i := range p {
		p[i] = uint8(i)
	}
	return p
}

// Build creates a break state table from a sequence of rules.
func Build(rules []Rule, stride, sot, eot, lastCP uint8) *BreakTable {
	n := int(stride)
	b := &TableBuilder{
		stride:  n,
		table:   make([]uint8, n*n),
		written: make([]bool, n*n),
	}
	for _, r := range rules {
		r.Apply(b)
	}
	for i, w := range b.written {
		if !w {
			b.table[i] = Break
		}
	}
	return &BreakTable{
		Table:  b.table,
		Stride: stride,
		LastCP: lastCP,
		SOT:    sot,
		EOT:    eot,
	}
}

// BreakTable holds the generated tables for one segmenter type.
type BreakTable struct {
	Table  []uint8
	Stride uint8
	LastCP uint8
	SOT    uint8
	EOT    uint8
}
