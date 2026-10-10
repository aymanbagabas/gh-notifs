//go:build unix

package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"altscreen.io/vision/term"
	"golang.org/x/sys/unix"
)

func TestRunRestoresTerminalAfterPanic(t *testing.T) {
	terminal, err := term.Stdio()
	if errors.Is(err, term.ErrNotTerminal) {
		t.Skip("requires terminal input and output; run the compiled test in a PTY")
	}
	if err != nil {
		t.Fatal(err)
	}
	before, err := terminal.GetState()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := terminal.SetState(before); err != nil {
			t.Errorf("restore test terminal: %v", err)
		}
	})

	// A nil client triggers a panic after the terminal enters raw mode.
	err = Run(context.Background(), nil, Options{NoColor: true})
	if err == nil || !strings.Contains(err.Error(), "inbox panic:") ||
		!strings.Contains(err.Error(), "runtime/debug.Stack") {
		t.Fatalf("expected panic details and stack trace, got %v", err)
	}
	after, err := terminal.GetState()
	if err != nil {
		t.Fatal(err)
	}
	// The kernel can set PENDIN when canonical mode resumes.
	for _, state := range []*term.State{before, after} {
		state.Input.Lflag &^= unix.PENDIN
		state.Output.Lflag &^= unix.PENDIN
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Run did not restore the original terminal attributes:\nbefore: %+v / %+v\nafter: %+v / %+v",
			before.Input, before.Output, after.Input, after.Output)
	}
}
