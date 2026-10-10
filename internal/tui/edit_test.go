package tui

import (
	"context"
	"strings"
	"testing"

	"altscreen.io/vision/event"
)

func TestSearchEditingBindings(t *testing.T) {
	for _, tc := range []struct {
		input string
		keys  []string
		want  string
	}{
		{"repo:acme/app|", []string{"alt+b"}, "repo:acme/|app"},
		{"repo:acme/app|", []string{"ctrl+left"}, "repo:acme/|app"},
		{"repo:|acme/app", []string{"alt+f"}, "repo:acme|/app"},
		{"repo:|acme/app", []string{"ctrl+right"}, "repo:acme|/app"},
		{"repo:|acme/app", []string{"alt+right"}, "repo:acme|/app"},
		{"repo:acme/app |", []string{"ctrl+w"}, "|"},
		{"repo:acme/app|", []string{"alt+backspace"}, "repo:acme/|"},
		{"hello 世界|", []string{"ctrl+backspace"}, "hello |"},
		{"repo:|acme/app", []string{"alt+d"}, "repo:|/app"},
		{"repo:acme|/app", []string{"ctrl+delete"}, "repo:acme|"},
		{"hello |world", []string{"ctrl+k", "ctrl+y"}, "hello world|"},
		{"hello |world", []string{"ctrl+k", "ctrl+k", "ctrl+y"}, "hello world|"},
		{"hello |world", []string{"ctrl+u", "ctrl+y"}, "hello |world"},
		{"he|llo", []string{"ctrl+d"}, "he|lo"},
		{"is:u|", []string{"ctrl+w"}, "|"},
		{"cafe\u0301|", []string{"alt+b"}, "|cafe\u0301"},
		{"|", []string{"ctrl+w", "ctrl+backspace", "ctrl+delete", "ctrl+d", "ctrl+y"}, "|"},
		{"word|", []string{"ctrl+a", "ctrl+f"}, "w|ord"},
		{"|word", []string{"ctrl+e", "ctrl+b"}, "wor|d"},
		{"|", []string{"A"}, "A|"},
	} {
		t.Run(tc.input+"/"+strings.Join(tc.keys, ","), func(t *testing.T) {
			a := testApp()
			before, after, _ := strings.Cut(tc.input, "|")
			a.edit, a.input, a.caret = true, []rune(before+after), len([]rune(before))
			for _, binding := range tc.keys {
				key, err := event.ParseKey(binding)
				if err != nil {
					t.Fatal(err)
				}
				a.key(context.Background(), key)
			}
			got := string(a.input[:a.caret]) + "|" + string(a.input[a.caret:])
			if got != tc.want || !a.edit {
				t.Fatalf("got %q (editing %t), want %q", got, a.edit, tc.want)
			}
		})
	}
}
