//go:build !unix && !windows

package event

import (
	"time"

	"altscreen.io/vision/internal/poll"
)

// selfPipe has no descriptors to watch off Unix, where the source stays on the
// goroutine reader.
type selfPipe struct{}

func (selfPipe) notify() error { return nil }
func (selfPipe) drain()        {}
func (selfPipe) close()        {}

// pollState is empty on a target with no readiness backend.
type pollState struct{}

// startPolling reports that there is no readiness backend to register with, so
// the source reads on a goroutine.
func (s *Source) startPolling(r Input) bool { return false }

// waitPoll is unreachable while startPolling reports false.
func (s *Source) waitPoll(poller poll.Poller, timeout time.Duration) (readResult, bool, bool) {
	return readResult{}, false, false
}

func (s *Source) closePolling() {}
