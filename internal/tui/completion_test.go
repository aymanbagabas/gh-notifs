package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func TestEveryCompletionHasDescription(t *testing.T) {
	for _, input := range []string{"", "is:", "reason:", "state:", "repo:", "org:", "author:", "since:", "before:"} {
		a := testApp()
		a.result.Items[0].Repository = "acme/工具"
		a.result.Items[0].Author = "octocat"
		a.edit, a.input, a.caret = true, []rune(input), len(input)
		values, _, _ := a.suggestions()
		if len(values) == 0 {
			t.Fatalf("no suggestions for %q", input)
		}
		for _, value := range values {
			if completionDescription(value) == "" {
				t.Errorf("missing description for %q", value)
			}
		}
	}
}

func TestTypeAliasCompletionDescriptions(t *testing.T) {
	for _, value := range []string{"is:issue", "is:pr", "is:pull-request"} {
		if got := completionDescription(value); got != "Issues and pull requests (alias)" {
			t.Fatalf("%s describes a separate type: %q", value, got)
		}
	}
}

func TestCompletionDescriptionLayoutAndInsertion(t *testing.T) {
	for _, width := range []int{40, 60, 120} {
		for _, noColor := range []bool{false, true} {
			a := testApp()
			a.options.NoColor = noColor
			a.edit, a.input, a.caret = true, []rune("reason:ment"), 11
			sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
			a.paint(sc)
			target := findHit(t, a, "completion")
			x := target.rect.Min.X + sc.StringWidth("reason:mention") + 3
			if target.rect.Min.X < 0 || target.rect.Max.X > width {
				t.Fatal("description extended the popup beyond the screen")
			}
			if sc.CellAt(layout.Pos(x, target.rect.Min.Y)).Content() != "Y" {
				t.Fatal("completion description is missing or misaligned")
			}
			descriptionStyle := sc.CellAt(layout.Pos(x, target.rect.Min.Y)).Style
			labelStyle := sc.CellAt(target.rect.Min.Add(layout.Pos(1, 0))).Style
			if descriptionStyle.GetBackground() != labelStyle.GetBackground() {
				t.Fatal("description lost the selection background")
			}
			if !noColor && descriptionStyle.GetForeground() != theme.Colors(false).Muted {
				t.Fatal("description is not muted")
			}
			if width == 40 {
				if sc.CellAt(layout.Pos(target.rect.Max.X-2, target.rect.Min.Y)).Content() != "…" {
					t.Fatal("narrow popup did not truncate the description")
				}
			} else if !strings.Contains(text.String(sc, color.Disabled), "You were directly @mentioned") {
				t.Fatal("wide popup did not show the complete description")
			}
			a.mouse(context.Background(), event.MouseClickEvent{
				X: x, Y: target.rect.Min.Y, Button: event.MouseLeft,
			}, sc)
			if string(a.input) != "reason:mention" {
				t.Fatalf("description click inserted %q", string(a.input))
			}
		}
	}
}

func TestEmptySearchShowsFilters(t *testing.T) {
	for _, input := range []string{"", "   "} {
		a := testApp()
		a.edit, a.input, a.caret = true, []rune(input), len(input)
		values, _, _ := a.suggestions()
		want := []string{"repo:", "org:", "reason:", "is:", "author:", "state:", "since:", "before:"}
		if !slices.Equal(values, want) || a.completionIndex != -1 {
			t.Fatalf("empty search: suggestions=%v, selection=%d", values, a.completionIndex)
		}
		sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
		a.paint(sc)
		count := 0
		for _, target := range a.hits {
			if target.kind == "completion" {
				wantY := findHit(t, a, "query").rect.Max.Y + count
				if target.rect.Min.Y != wantY {
					t.Fatalf("completion row is at %d, want %d directly below the search", target.rect.Min.Y, wantY)
				}
				count++
			}
		}
		if count != len(want) {
			t.Fatalf("empty search displays %d filters, want %d", count, len(want))
		}
		target := findHit(t, a, "completion")
		a.mouse(context.Background(), event.MouseClickEvent{
			X: target.rect.Min.X + 1, Y: target.rect.Min.Y, Button: event.MouseLeft,
		}, sc)
		if string(a.input) != input+"repo:" {
			t.Fatal("clicking the first completion did not insert the filter")
		}
	}
}

func TestCompletionKeysAtCursor(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		keys        []string
		want        string
	}{
		{"empty tab", "|", []string{"tab"}, "repo:|"},
		{"empty down", "|", []string{"down", "enter"}, "repo:|"},
		{"empty up", "|", []string{"up", "enter"}, "before:|"},
		{"empty explicit", "|", []string{"ctrl+space", "enter"}, "repo:|"},
		{"new term", "is:unread |", []string{"ctrl+space", "enter"}, "is:unread repo:|"},
		{"legacy NUL", "is:unread |", []string{"ctrl+@", "tab"}, "is:unread repo:|"},
		{"field name", "rea|", []string{"ctrl+space", "enter"}, "reason:|"},
		{"field values", "state:|", []string{"ctrl+space", "tab"}, "state:open|"},
		{"participation reason", "reason:part|", []string{"tab"}, "reason:participating|"},
		{"repeated reason", "reason:mention reason:comm| is:unread", []string{"tab"}, "reason:mention reason:comment| is:unread"},
		{"repeated type", "is:issue is:p| is:unread", []string{"tab"}, "is:issue is:pr| is:unread"},
		{"repeated read state", "is:read is:un|", []string{"tab"}, "is:read is:unread|"},
		{"middle term", "org:acme is:un| state:open", []string{"ctrl+space", "tab"}, "org:acme is:unread| state:open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

func TestCompletionRequestResets(t *testing.T) {
	a := testApp()
	a.query.Raw = "is:unread "
	a.key(context.Background(), event.Key{Code: '/'})
	if values, _, _ := a.suggestions(); len(values) != 0 {
		t.Fatal("a trailing space should still allow Enter to apply the query")
	}
	a.key(context.Background(), event.Key{Code: ' ', Mod: event.KeyModCtrl})
	if values, _, _ := a.suggestions(); len(values) != 8 {
		t.Fatal("Ctrl+Space did not show the available filters")
	}
	a.key(context.Background(), event.Key{Code: ' '})
	if values, _, _ := a.suggestions(); len(values) != 0 {
		t.Fatal("the completion request persisted after an edit")
	}
	a.key(context.Background(), event.Key{Code: ' ', Mod: event.KeyModCtrl})
	a.key(context.Background(), event.Key{Code: event.KeyEscape})
	a.key(context.Background(), event.Key{Code: '/'})
	if values, _, _ := a.suggestions(); len(values) != 0 {
		t.Fatal("the completion request persisted after reopening search")
	}
	a.key(context.Background(), event.Key{Code: 'u', Mod: event.KeyModCtrl})
	if values, _, _ := a.suggestions(); len(values) != 8 || a.completionIndex != -1 {
		t.Fatal("clearing the search did not restore the unselected filter list")
	}
}

func TestEnterAppliesEmptySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "[]")
	}))
	defer server.Close()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	a := testApp()
	a.client = &notifs.Client{HTTP: server.Client(), BaseURL: server.URL + "/", Cache: cache}
	a.messages = make(chan message, 8)
	a.query.Raw = "is:unread"
	a.edit = true
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); a.workers.Wait() }()
	a.key(ctx, event.Key{Code: event.KeyEnter})
	if a.edit || a.query.Raw != "" || a.err != nil {
		t.Fatalf("empty search was not applied: editing=%t, query=%q, error=%v", a.edit, a.query.Raw, a.err)
	}
}

func TestCompletionScrollIndicator(t *testing.T) {
	for _, height := range []int{12, 32} {
		a := testApp()
		a.edit, a.input, a.caret = true, []rune("is:"), 3
		sc := screen.New(io.Discard, layout.Size{Width: 60, Height: height})
		a.paint(sc)
		var first, last hit
		for _, target := range a.hits {
			if target.kind == "completion" {
				if first.kind == "" {
					first = target
				}
				last = target
			}
		}
		if first.kind == "" {
			t.Fatal("completion popup is missing")
		}
		top := layout.Pos(first.rect.Max.X-1, first.rect.Min.Y)
		bottom := layout.Pos(last.rect.Max.X-1, last.rect.Min.Y)
		if sc.CellAt(top).Content() != "┃" || sc.CellAt(bottom).Content() != "│" {
			t.Fatal("scroll indicator does not start at the top")
		}
		values, _, _ := a.suggestions()
		a.completionIndex = len(values) - 1
		a.paint(sc)
		if sc.CellAt(top).Content() != "│" || sc.CellAt(bottom).Content() != "┃" {
			t.Fatal("scroll indicator did not move to the bottom")
		}
		a.input, a.caret = []rune("is:un"), 5
		a.paint(sc)
		for _, target := range a.hits {
			if target.kind == "completion" && sc.CellAt(layout.Pos(target.rect.Max.X-1, target.rect.Min.Y)).Content() != " " {
				t.Fatal("popup has a scrollbar when all suggestions fit")
			}
		}
	}
}
