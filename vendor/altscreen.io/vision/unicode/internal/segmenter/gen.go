package segmenter

import (
	"fmt"

	"altscreen.io/vision/unicode/internal/gen"
)

// WriteBreakTable writes the breakTable array and a RuleBreakData struct
// literal to w, using the given variable name and trie type name.
func WriteBreakTable(w *gen.CodeWriter, bt *BreakTable) {
	n := int(bt.Stride)
	w.WriteComment(
		`breakTable is the break state table.
	breakTable[left*stride + right] encodes the action for (left, right).
	See segmenter.BreakState for the action encoding.`)
	fmt.Fprintf(w, "var breakTable = [...]uint8{")
	for i, v := range bt.Table {
		if i%n == 0 {
			fmt.Fprintf(w, "\n\t")
		}
		fmt.Fprintf(w, "%d, ", v)
	}
	fmt.Fprintf(w, "\n}\n\n")
}
