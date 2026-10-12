//go:build unix

package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"altscreen.io/vision/ansi"
	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/program"
	"altscreen.io/vision/term"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
	"golang.org/x/sys/unix"
)

type bellTestOutput struct {
	*os.File
	bytes.Buffer
	err error
}

func (w *bellTestOutput) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func TestRefreshBell(t *testing.T) {
	output := &bellTestOutput{File: os.Stdout}
	terminal, err := term.New(os.Stdin, output, nil)
	if errors.Is(err, term.ErrNotTerminal) {
		t.Skip("requires terminal input and output; run the compiled test in a PTY")
	}
	if err != nil {
		t.Fatal(err)
	}
	p, err := program.New(terminal)
	if err != nil {
		t.Fatal(err)
	}
	a := testApp()
	a.program = p
	a.autoRefresh, a.bell = true, true
	now := time.Now()
	for _, tc := range []struct {
		name      string
		ids       []string
		automatic bool
		enabled   bool
		err       error
		bells     int
	}{
		{"startup", []string{"1"}, false, true, nil, 0},
		{"batch", []string{"2", "3"}, true, true, nil, 1},
		{"unchanged", []string{"2", "3"}, true, true, nil, 1},
		{"failure", []string{"4"}, true, true, errors.New("offline"), 1},
		{"retry", []string{"4"}, true, true, nil, 2},
		{"pagination", []string{"5"}, false, true, nil, 2},
		{"already observed", []string{"5"}, true, true, nil, 2},
		{"disabled", []string{"6"}, true, false, nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.autoFetching, a.bell = tc.automatic, tc.enabled
			a.notifyRefresh = tc.automatic
			result := notifs.Result{}
			for _, id := range tc.ids {
				result.Items = append(result.Items, notifs.Thread{ID: id, Unread: true,
					UpdatedAt: now.Add(time.Duration(id[0]) * time.Second)})
			}
			a.receive(context.Background(), message{generation: a.generation, final: true, result: &result, err: tc.err})
			if got := strings.Count(output.String(), "\a"); got != tc.bells {
				t.Fatalf("bell count=%d, want %d", got, tc.bells)
			}
		})
	}
	a.bell, a.notifyRefresh = true, true
	a.autoRefresh, a.autoFetching = false, false
	a.key(context.Background(), event.Key{Code: 'r', Mod: event.KeyModShift})
	a.key(context.Background(), event.Key{Code: 'r', Mod: event.KeyModShift})
	result := notifs.Result{Items: []notifs.Thread{{ID: "7", Unread: true, UpdatedAt: now.Add(time.Hour)}}}
	a.receive(context.Background(), message{generation: a.generation, final: true, result: &result})
	if got := strings.Count(output.String(), "\a"); got != 3 {
		t.Fatalf("manual refresh with automatic refresh disabled: bell count=%d, want 3", got)
	}
	a.receive(context.Background(), message{generation: a.generation, final: true, result: &result})
	if got := strings.Count(output.String(), "\a"); got != 3 {
		t.Fatalf("unchanged manual refresh repeated the bell: %d", got)
	}
	output.err = errors.New("terminal write failed")
	a.key(context.Background(), event.Key{Code: event.KeyEscape})
	result.Items[0].UpdatedAt = now.Add(2 * time.Hour)
	a.receive(context.Background(), message{generation: a.generation, final: true, result: &result})
	if !errors.Is(a.err, output.err) {
		t.Fatalf("bell failure was not reported: %v", a.err)
	}
}

func TestTerminalBackgroundFollowsTheme(t *testing.T) {
	for _, noColor := range []bool{false, true} {
		output := &bellTestOutput{File: os.Stdout}
		terminal, err := term.New(os.Stdin, output, nil)
		if errors.Is(err, term.ErrNotTerminal) {
			t.Skip("requires terminal input and output; run the compiled test in a PTY")
		}
		if err != nil {
			t.Fatal(err)
		}
		p, err := program.New(terminal)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Finish() })
		a := testApp()
		a.program, a.options.NoColor = p, noColor
		events := make(chan event.Event, 6)
		events <- event.BackgroundColorEvent{Color: theme.Colors(true).Background}
		events <- event.ColorSchemeEvent(event.ColorSchemeDark)
		// A delayed background reply must not override the explicit scheme.
		events <- event.BackgroundColorEvent{Color: theme.Colors(true).Background}
		events <- event.ColorSchemeEvent(event.ColorSchemeLight)
		events <- event.ColorSchemeEvent(event.ColorSchemeLight)
		close(events)
		if err := a.run(context.Background(), events); err != nil {
			t.Fatal(err)
		}
		if !a.light {
			t.Fatal("last color scheme was not applied")
		}
		for _, light := range []bool{false, true} {
			bg := theme.Colors(light).Background
			sequence := ansi.SetBackgroundColor(ansi.XParseRGB(bg.R, bg.G, bg.B))
			want := 2
			if noColor {
				want = 0
			}
			if got := strings.Count(output.String(), sequence); got != want {
				t.Fatalf("noColor=%t light=%t: background writes=%d, want %d", noColor, light, got, want)
			}
		}
		if !noColor {
			output.err = errors.New("background write failed")
			a.light = false
			if err := a.render(); !errors.Is(err, output.err) {
				t.Fatalf("background write error was lost: %v", err)
			}
			if a.terminalBG != theme.Colors(true).Background {
				t.Fatal("failed write recorded an unapplied color")
			}
			output.err = nil
		}
		output.Reset()
		if err := p.Finish(); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(output.String(), ansi.ResetBackgroundColor); got == noColor {
			t.Fatalf("noColor=%t: terminal background reset=%t", noColor, got)
		}
	}
}

func TestTerminalMousePointer(t *testing.T) {
	output := &bellTestOutput{File: os.Stdout}
	terminal, err := term.New(os.Stdin, output, nil)
	if errors.Is(err, term.ErrNotTerminal) {
		t.Skip("requires terminal input and output; run the compiled test in a PTY")
	}
	if err != nil {
		t.Fatal(err)
	}
	p, err := program.New(terminal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Finish() })
	a := testApp()
	a.program, a.options.NoColor = p, true
	events := make(chan event.Event, 8)
	events <- event.MouseMoveEvent{X: 10, Y: 2}
	events <- event.MouseMoveEvent{X: 11, Y: 2}
	events <- event.MouseClickEvent{X: 24, Y: 10, Button: event.MouseLeft}
	events <- event.FocusOutEvent{}
	close(events)
	if err := a.run(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if a.dragging != "" || a.mouseKnown || a.pointerShape != "default" {
		t.Fatal("focus loss retained a drag or pointer")
	}
	for shape, want := range map[string]int{"default": 2, "text": 1, "ew-resize": 1} {
		if got := strings.Count(output.String(), ansi.SetPointerShape(shape)); got != want {
			t.Fatalf("%s writes=%d, want %d", shape, got, want)
		}
	}
	a.mouseKnown, a.mousePosition = true, layout.Pos(10, 2)
	output.err = errors.New("pointer write failed")
	if err := a.render(); !errors.Is(err, output.err) || !strings.Contains(err.Error(), "set mouse pointer") {
		t.Fatalf("pointer failure was not reported: %v", err)
	}
	if a.pointerShape != "default" {
		t.Fatal("failed write recorded an unapplied pointer")
	}
	output.err = nil
	if err := a.render(); err != nil {
		t.Fatal(err)
	}
	if a.pointerShape != "text" {
		t.Fatal("pointer did not recover after a failed write")
	}
	output.Reset()
	if err := p.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), ansi.SetPointerShape("default")) {
		t.Fatal("terminal cleanup did not reset the pointer")
	}
}

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
