package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestPasteAssemblesBytesBeforeDecoding(t *testing.T) {
	input := "repo:acme/app 世界 café"
	for boundary := 0; boundary <= len(input); boundary++ {
		a := testApp()
		a.edit = true
		a.handlePaste(event.PasteStartEvent{})
		a.handlePaste(event.PasteChunkEvent([]byte(input[:boundary])))
		if len(a.input) != 0 {
			t.Fatal("paste inserted text before PasteEnd")
		}
		a.handlePaste(event.PasteChunkEvent([]byte(input[boundary:])))
		a.handlePaste(event.PasteEndEvent{})
		if got := string(a.input); got != input {
			t.Fatalf("boundary %d: got %q, want %q", boundary, got, input)
		}
	}
	a := testApp()
	a.edit = true
	a.handlePaste(event.PasteStartEvent{})
	for _, b := range []byte(input) {
		a.handlePaste(event.PasteChunkEvent{b})
	}
	a.handlePaste(event.PasteEndEvent{})
	if string(a.input) != input {
		t.Fatalf("one-byte chunks corrupted UTF-8: %q", a.input)
	}
}

func TestPasteCanBeCanceledAndDoesNotRunCommands(t *testing.T) {
	a := testApp()
	a.handlePaste(event.PasteStartEvent{})
	a.handlePaste(event.PasteChunkEvent("e"))
	a.handlePaste(event.PasteEndEvent{})
	if a.pendingAction != "" || len(a.input) != 0 {
		t.Fatal("paste outside the editor changed state")
	}
	a.edit = true
	a.handlePaste(event.PasteStartEvent{})
	a.handlePaste(event.PasteChunkEvent("old"))
	a.key(context.Background(), event.Key{Code: event.KeyEscape})
	a.edit = true
	a.handlePaste(event.PasteEndEvent{})
	if len(a.input) != 0 {
		t.Fatal("canceled paste appeared in a later edit")
	}
	a.handlePaste(event.PasteStartEvent{})
	a.handlePaste(event.PasteChunkEvent("hello\nworld"))
	a.handlePaste(event.PasteEndEvent{})
	if string(a.input) != "hello world" {
		t.Fatalf("query paste contains control characters: %q", a.input)
	}
}

func TestQueryCursorUsesCellsAndScrolls(t *testing.T) {
	sc := screen.New(io.Discard, layout.Size{Width: 80, Height: 24})
	for _, tc := range []struct {
		input string
		caret int
		width int
		text  string
		x     int
	}{
		{"", 0, 10, "", 0},
		{"hello", 3, 10, "hello", 3},
		{"世界abc", 2, 10, "世界abc", 4},
		{"abcdefghij", 10, 5, "ghij", 4},
		{"世界abc", 5, 5, "abc", 3},
		{"cafe\u0301", 5, 10, "cafe\u0301", 4},
		{"🚀abc", 1, 10, "🚀abc", 2},
	} {
		text, x := queryViewport(sc, []rune(tc.input), tc.caret, tc.width)
		if text != tc.text || x != tc.x {
			t.Errorf("%q: viewport (%q,%d), want (%q,%d)", tc.input, text, x, tc.text, tc.x)
		}
	}
	a := testApp()
	a.edit, a.input, a.caret = true, []rune("hello"), 5
	a.paint(sc)
	if !a.showCursor {
		t.Fatal("editing does not request a visible cursor")
	}
	a.edit = false
	a.paint(sc)
	if a.showCursor {
		t.Fatal("cursor remains visible outside the editor")
	}
}

func TestPreviewCacheTracksWidthAndTheme(t *testing.T) {
	a := testApp()
	a.preview = &notifs.Preview{Title: "Markdown preview", Body: "**Bold** text and a [link](https://github.com)."}
	first, err := a.previewLines(60)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.previewLines(60)
	if err != nil || &first[0] != &again[0] {
		t.Fatal("unchanged preview was rendered again")
	}
	a.light = true
	light, err := a.previewLines(60)
	if err != nil || strings.Join(first, "\n") == strings.Join(light, "\n") {
		t.Fatal("theme change did not update the preview")
	}
	if _, err := a.previewLines(30); err != nil || a.markdown.width != 30 {
		t.Fatal("resize did not update the preview")
	}
	a.preview.Body = "A replacement body."
	changed, err := a.previewLines(30)
	if err != nil || !strings.Contains(strings.Join(changed, "\n"), "replacement") {
		t.Fatal("body change did not update the preview")
	}
	a.options.NoColor = true
	plain, err := a.previewLines(30)
	if err != nil || strings.Contains(strings.Join(plain, "\n"), "\x1b") {
		t.Fatal("no-color change did not update the preview")
	}
	a.preview.Metadata = []notifs.PreviewSection{{Name: "Assignees", Items: []string{"@octocat"}, Total: 1}}
	metadata, err := a.metadataLines(60)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(metadata, "\n")
	if !strings.Contains(out, "@octocat") || strings.Contains(out, "replacement") {
		t.Fatal("metadata change did not update the separate details pane")
	}
}
