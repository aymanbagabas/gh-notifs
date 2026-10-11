//go:build unix

package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"altscreen.io/vision/color"
	"altscreen.io/vision/program"
	"altscreen.io/vision/term"
	"altscreen.io/vision/text"
	"golang.org/x/sys/unix"
)

func TestRunUpdatesAgesWhileIdle(t *testing.T) {
	p, err := program.Stdio()
	if errors.Is(err, term.ErrNotTerminal) {
		t.Skip("requires terminal input and output; run the compiled test in a PTY")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Finish(); err != nil {
			t.Errorf("restore terminal: %v", err)
		}
	})

	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{"seconds", 0, "Refreshed 2s ago"},
		{"minutes", 59 * time.Second, "Refreshed 1m ago"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := testApp()
				a.program = p
				a.result.CheckedAt = time.Now().Add(-tc.elapsed)
				a.result.Items[0].UpdatedAt = time.Now().Add(-59 * time.Second)
				a.cursor = 1
				a.selected["2"] = true
				checkedAt := a.result.CheckedAt
				ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
				defer cancel()

				// No input or worker messages arrive during this interval.
				if err := a.run(ctx, nil); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("idle loop exit: %v", err)
				}
				output := text.String(p.Screen(), color.Disabled)
				for _, want := range []string{tc.want, "1m ago"} {
					if !strings.Contains(output, want) {
						t.Errorf("idle display lacks %q:\n%s", want, output)
					}
				}
				if a.result.CheckedAt != checkedAt || a.cursor != 1 || !a.selected["2"] || a.loading {
					t.Fatal("display tick changed refresh or selection state")
				}
			})
		})
	}
}

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
