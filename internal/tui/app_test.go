package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func testApp() *app {
	return &app{client: &notifs.Client{Host: "github.com"}, options: Options{PageSize: 2},
		selected: make(map[string]bool),
		result: notifs.Result{Items: []notifs.Thread{
			{ID: "1", Title: "Fix slow uploads", Type: "PullRequest", Repository: "acme/app", Number: 7, Unread: true},
			{ID: "2", Title: "Release v1", Type: "Release", Repository: "acme/app"},
		}, CheckedAt: time.Now()}, generation: 1}
}

func TestRenderAndInput(t *testing.T) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	out := text.String(sc, color.Disabled)
	for _, expected := range []string{"Notifications", "Fix slow uploads", "Views and scopes", "Refreshed"} {
		if !strings.Contains(out, expected) {
			t.Errorf("missing %q in screen", expected)
		}
	}
	a.key(context.Background(), event.Key{Code: 'j'})
	if a.cursor != 1 {
		t.Fatal("j did not move selection")
	}
	a.key(context.Background(), event.Key{Code: 'x'})
	if !a.selected["2"] {
		t.Fatal("x did not select the thread")
	}
	a.key(context.Background(), event.Key{Code: 'e'})
	if a.pendingAction != "done" {
		t.Fatal("done did not require confirmation")
	}
	a.key(context.Background(), event.Key{Code: 'n'})
	if a.pendingAction != "" || a.acting {
		t.Fatal("canceled action was started")
	}
}

func TestPaginationProgressDoesNotResetRequestedPage(t *testing.T) {
	a := testApp()
	a.page, a.cursor = 1, 2
	first := a.result
	a.receive(context.Background(), message{generation: 1, result: &first})
	if a.page != 1 || a.cursor != 2 {
		t.Fatal("partial response reset the requested page")
	}
	full := first
	full.Items = append(append([]notifs.Thread(nil), first.Items...), notifs.Thread{ID: "3", Title: "Next page"})
	a.receive(context.Background(), message{generation: 1, result: &full, final: true})
	if a.page != 1 || a.current().ID != "3" {
		t.Fatal("next page did not select its first item")
	}
}

func TestStaleResultsAndSelection(t *testing.T) {
	a := testApp()
	stale := notifs.Result{Items: []notifs.Thread{{ID: "other"}}}
	a.receive(context.Background(), message{generation: 0, result: &stale, final: true})
	if a.current().ID != "1" {
		t.Fatal("stale query replaced current result")
	}
	reordered := a.result
	reordered.Items = []notifs.Thread{a.result.Items[1], a.result.Items[0]}
	a.receive(context.Background(), message{generation: 1, result: &reordered, final: true})
	if a.current().ID != "1" || a.cursor != 1 {
		t.Fatal("refresh did not preserve selection")
	}
}

func TestQueryEditor(t *testing.T) {
	a := testApp()
	a.key(context.Background(), event.Key{Code: '/'})
	for _, r := range "is:saved" {
		a.key(context.Background(), event.Key{Code: r})
	}
	a.key(context.Background(), event.Key{Code: event.KeyEnter})
	if a.err == nil || !a.edit {
		t.Fatal("invalid query should remain editable and show its error")
	}
	result := a.result
	a.receive(context.Background(), message{generation: 1, result: &result, final: true})
	if a.err == nil {
		t.Fatal("background refresh cleared the query error")
	}
	a.key(context.Background(), event.Key{Code: event.KeyEscape})
	if a.edit {
		t.Fatal("Escape did not leave query input")
	}
}

func TestNarrowScreenAndPreviewWrapping(t *testing.T) {
	a := testApp()
	for _, size := range []layout.Size{{Width: 20, Height: 5}, {Width: 60, Height: 20}, {Width: 120, Height: 32}} {
		sc := screen.New(io.Discard, size)
		a.showPreview = true
		a.preview = &notifs.Preview{Title: "Preview", Body: strings.Repeat("wide 世界 ", 40)}
		a.paint(sc)
	}
	lines, err := a.previewLines(40)
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "世界") {
		t.Fatalf("Unicode Markdown preview failed: %v", err)
	}
	painter := text.NewPainter(screen.New(io.Discard, layout.Size{Width: 40, Height: 20}))
	for _, line := range lines {
		if width := painter.StringWidth(line); width > 40 {
			t.Fatalf("preview line exceeds 40 cells: %d", width)
		}
	}
}

func TestUnsafeURL(t *testing.T) {
	for _, target := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:password@example.com"} {
		if OpenBrowser(nil, target) == nil {
			t.Errorf("accepted %q", target)
		}
	}
}

func BenchmarkRenderInbox(b *testing.B) {
	a := testApp()
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 40})
	for b.Loop() {
		a.paint(sc)
	}
}
