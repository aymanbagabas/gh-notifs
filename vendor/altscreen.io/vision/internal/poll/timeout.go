package poll

import (
	"math"
	"time"
)

// milliseconds rounds a finite wait up without overflowing a Duration.
func milliseconds(d time.Duration) time.Duration {
	ms := d / time.Millisecond
	if d%time.Millisecond != 0 {
		ms++
	}
	return ms
}

func epollMillis(d time.Duration) int {
	return int(min(milliseconds(d), math.MaxInt32))
}

func windowsMillis(d time.Duration) uint32 {
	// The maximum uint32 value is INFINITE, not a finite timeout.
	return uint32(min(milliseconds(d), math.MaxUint32-1))
}
