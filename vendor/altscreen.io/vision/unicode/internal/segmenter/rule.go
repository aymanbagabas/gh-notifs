package segmenter

// Rule is the interface for all rule types that contribute to a break state table.
// Each rule knows how to emit its cells into a table builder.
type Rule interface {
	Apply(b *TableBuilder)
}

// SimpleRule defines a break or keep decision for (left, right) property pairs.
// nil Left or Right means "any property" (wildcard).
type SimpleRule struct {
	Left  []uint8
	Right []uint8
	Break bool
}

func (r SimpleRule) Apply(b *TableBuilder) {
	lefts := r.Left
	rights := r.Right
	if lefts == nil {
		lefts = b.allProps()
	}
	if rights == nil {
		rights = b.allProps()
	}
	var state BreakState
	if r.Break {
		state = Break
	} else {
		state = Keep
	}
	for _, l := range lefts {
		for _, ri := range rights {
			b.Set(l, ri, state)
		}
	}
}

// IgnoreRule generates combined states for absorption patterns like
// "X (Extend | Format)*" where the ignored properties are transparent.
type IgnoreRule struct {
	Props   []uint8
	Ignored []uint8
	Target  func(base, ignored uint8) uint8
	Interm  bool
}

func (r IgnoreRule) Apply(b *TableBuilder) {
	for _, base := range r.Props {
		for _, ign := range r.Ignored {
			target := r.Target(base, ign)
			if r.Interm {
				b.ForceSet(base, ign, IntermediateState(target))
			} else {
				b.ForceSet(base, ign, IndexState(target))
			}
		}
	}
}

// IntermOverride controls per-step intermediate encoding.
type IntermOverride int8

const (
	IntermDefault IntermOverride = iota
	IntermTrue
	IntermFalse
)

// ChainStep is one transition in a chain sequence.
type ChainStep struct {
	Props  []uint8
	State  uint8
	Interm IntermOverride
}

// ChainRule defines a multi-step lookahead pattern.
type ChainRule struct {
	Entry    []uint8
	Steps    []ChainStep
	SelfLoop []uint8
	Interm   bool
}

func (r ChainRule) Apply(b *TableBuilder) {
	for i, step := range r.Steps {
		var lefts []uint8
		if i == 0 {
			lefts = r.Entry
		} else {
			lefts = []uint8{r.Steps[i-1].State}
		}

		interm := r.Interm
		switch step.Interm {
		case IntermTrue:
			interm = true
		case IntermFalse:
			interm = false
		}

		for _, l := range lefts {
			for _, right := range step.Props {
				if interm {
					b.ForceSet(l, right, IntermediateState(step.State))
				} else {
					b.ForceSet(l, right, IndexState(step.State))
				}
			}
		}

		for _, sl := range r.SelfLoop {
			if interm {
				b.ForceSet(step.State, sl, IntermediateState(step.State))
			} else {
				b.ForceSet(step.State, sl, IndexState(step.State))
			}
		}
	}
}

// OverrideRule overwrites specific cells in already-built combined state rows.
type OverrideRule struct {
	States    []uint8
	Overrides map[uint8]uint8
	WipeValue uint8
}

func (r OverrideRule) Apply(b *TableBuilder) {
	for _, s := range r.States {
		for i := 0; i < b.stride; i++ {
			b.ForceSet(s, uint8(i), r.WipeValue)
		}
		for right, val := range r.Overrides {
			b.ForceSet(s, right, val)
		}
	}
}
