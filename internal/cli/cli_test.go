package cli

import (
	"context"
	"strings"
	"testing"
)

func TestIntervalValidation(t *testing.T) {
	for _, interval := range []string{"-1s", "invalid"} {
		err := Run(context.Background(), []string{"--interval", interval}, "test")
		if err == nil || !strings.Contains(err.Error(), "interval") {
			t.Fatalf("invalid interval %q was accepted: %v", interval, err)
		}
	}
}
