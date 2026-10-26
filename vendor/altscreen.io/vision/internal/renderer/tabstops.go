package renderer

const (
	DefaultTabInterval = 8
	wordBits           = 64
)

// TabStops stores horizontal tab stops and O(1) neighbor tables.
type TabStops struct {
	stops               []uint64
	width, interval     int
	next, nextRaw, prev []int
}

func NewTabStops(width, interval int) *TabStops {
	if interval < 1 {
		interval = 1
	}
	t := &TabStops{stops: make([]uint64, (max(width, 0)+wordBits-1)/wordBits), width: max(width, 0), interval: interval}
	t.initDefault(0, t.width)
	t.rebuild()
	return t
}

func DefaultTabStops(width int) *TabStops { return NewTabStops(width, DefaultTabInterval) }
func (t *TabStops) Width() int            { return t.width }

func (t *TabStops) Resize(width int) {
	width = max(width, 0)
	old := t.width
	if width == old {
		return
	}
	t.stops = resizeWords(t.stops, (width+wordBits-1)/wordBits)
	if width < old {
		t.clearPast(width)
	}
	t.width = width
	if width > old {
		t.initDefault(old, width)
	}
	t.rebuild()
}

func (t *TabStops) IsStop(x int) bool {
	if x < 0 || x >= t.width {
		return false
	}
	return t.bit(x)
}

func (t *TabStops) Next(x int) int {
	if t.width == 0 {
		return x
	}
	idx := min(max(x, 0), len(t.next)-1)
	return t.next[idx]
}

func (t *TabStops) NextStop(x int) int {
	if x >= t.width {
		return (x/t.interval + 1) * t.interval
	}
	if x < 0 {
		x = 0
	}
	return t.nextRaw[x]
}

func (t *TabStops) Prev(x int) int {
	if x <= 0 || len(t.prev) == 0 {
		return 0
	}
	return t.prev[min(x, len(t.prev)-1)]
}

func (t *TabStops) Set(x int) {
	if x < 0 || x >= t.width {
		return
	}
	t.stops[x/wordBits] |= 1 << (x % wordBits)
	t.rebuild()
}

func (t *TabStops) Reset(x int) {
	if x < 0 || x >= t.width {
		return
	}
	t.stops[x/wordBits] &^= 1 << (x % wordBits)
	t.rebuild()
}

func (t *TabStops) Clear() {
	for i := range t.stops {
		t.stops[i] = 0
	}
	t.rebuild()
}

func (t *TabStops) initDefault(from, to int) {
	for x := max(from, 0); x < to; x++ {
		if x%t.interval == 0 {
			t.stops[x/wordBits] |= 1 << (x % wordBits)
		}
	}
}

func (t *TabStops) clearPast(width int) {
	if width >= len(t.stops)*wordBits {
		return
	}
	idx, bit := width/wordBits, width%wordBits
	if idx < len(t.stops) {
		t.stops[idx] &= (uint64(1) << bit) - 1
		for i := idx + 1; i < len(t.stops); i++ {
			t.stops[i] = 0
		}
	}
}

func (t *TabStops) rebuild() {
	w := t.width
	t.next, t.nextRaw, t.prev = make([]int, w), make([]int, w), make([]int, w)
	if w == 0 {
		return
	}
	last := w - 1
	nearest := last
	nearestReal := -1
	for x := w - 1; x >= 0; x-- {
		t.next[x] = nearest
		if nearestReal >= 0 {
			t.nextRaw[x] = nearestReal
		} else {
			t.nextRaw[x] = (x/t.interval + 1) * t.interval
		}
		if t.bit(x) {
			nearest, nearestReal = x, x
		}
	}
	nearest = 0
	for x := 0; x < w; x++ {
		t.prev[x] = nearest
		if t.bit(x) {
			nearest = x
		}
	}
}

func (t *TabStops) bit(x int) bool { return (t.stops[x/wordBits]>>(x%wordBits))&1 != 0 }

func resizeWords(in []uint64, n int) []uint64 {
	out := make([]uint64, n)
	copy(out, in)
	return out
}
