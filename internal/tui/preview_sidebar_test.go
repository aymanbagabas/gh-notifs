package tui

import (
	"context"
	"io"
	"strings"
	"testing"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func findHit(t *testing.T, a *app, kind string) hit {
	t.Helper()
	for _, target := range a.hits {
		if target.kind == kind {
			return target
		}
	}
	t.Fatalf("missing %s target", kind)
	return hit{}
}

func TestPreviewSidebarResizeCollapseAndScroll(t *testing.T) {
	a := testApp()
	a.showPreview = true
	a.preview = &notifs.Preview{
		Title: "Body title", Body: strings.Repeat("Body content.\n\n", 30),
		Metadata: []notifs.PreviewSection{
			{Name: "Author", Items: []string{"@octocat"}, Total: -1},
			{Name: "Labels", Total: 0},
			{Name: "Reviewers", Items: []string{strings.Repeat("@reviewer ", 30)}, Total: 1},
		},
	}
	preview := a.preview
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	ctx := context.Background()
	a.paint(sc)
	body, details := findHit(t, a, "preview"), findHit(t, a, "metadata")
	if body.rect.Max.X >= details.rect.Min.X {
		t.Fatal("preview panes overlap")
	}
	lines, err := a.metadataLines(30)
	if err != nil || strings.Contains(strings.Join(lines, "\n"), "Labels") {
		t.Fatal("details did not omit an empty section")
	}
	a.mouse(ctx, event.MouseWheelEvent{X: details.rect.Min.X, Y: details.rect.Min.Y, Button: event.MouseWheelDown}, sc)
	a.paint(sc)
	if a.metadataOffset != 3 || a.previewOffset != 0 {
		t.Fatal("details wheel changed body scroll")
	}
	a.key(ctx, event.Key{Code: event.KeyLeft, Mod: event.KeyModAlt})
	a.key(ctx, event.Key{Code: 'd', Mod: event.KeyModCtrl})
	if a.previewOffset != 5 || a.metadataOffset != 3 {
		t.Fatal("keyboard scroll did not follow body focus")
	}
	divider := findHit(t, a, "metadata-divider")
	a.mouse(ctx, event.MouseClickEvent{X: divider.rect.Min.X, Y: divider.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseMoveEvent{X: divider.rect.Min.X - 8, Y: divider.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseReleaseEvent{Button: event.MouseLeft}, sc)
	a.paint(sc)
	if a.metadataWidth != 40 || findHit(t, a, "metadata").rect.Min.X != details.rect.Min.X-8 {
		t.Fatal("metadata drag did not resize the pane")
	}
	a.key(ctx, event.Key{Code: 'm'})
	a.paint(sc)
	if !a.metadataHidden || findHit(t, a, "preview").rect.Dx() <= body.rect.Dx() {
		t.Fatal("collapse did not return space to the body")
	}
	a.key(ctx, event.Key{Code: 'm'})
	a.paint(sc)
	if a.metadataHidden || a.metadataWidth != 40 || a.preview != preview || a.previewGeneration != 0 {
		t.Fatal("restoring details lost width or triggered a request")
	}
}

func TestNarrowPreviewSwitchesBodyAndDetails(t *testing.T) {
	a := testApp()
	a.showPreview = true
	a.preview = &notifs.Preview{Title: "Body title", Body: "Only body content",
		Metadata: []notifs.PreviewSection{{Name: "Author", Items: []string{"@octocat"}, Total: -1}}}
	sc := screen.New(io.Discard, layout.Size{Width: 40, Height: 32})
	a.paint(sc)
	if !a.previewNarrow || !strings.Contains(text.String(sc, color.Disabled), "Only body content") {
		t.Fatal("narrow preview did not start with the body")
	}
	toggle := findHit(t, a, "metadata-toggle")
	a.mouse(context.Background(), event.MouseClickEvent{X: toggle.rect.Min.X, Y: toggle.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.paint(sc)
	out := text.String(sc, color.Disabled)
	if !strings.Contains(out, "@octocat") || strings.Contains(out, "Only body content") {
		t.Fatal("narrow preview did not switch to details")
	}
	body := findHit(t, a, "preview-body")
	a.mouse(context.Background(), event.MouseClickEvent{X: body.rect.Min.X, Y: body.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.paint(sc)
	if a.metadataFocused || !strings.Contains(text.String(sc, color.Disabled), "Only body content") {
		t.Fatal("body tab did not restore the body")
	}
}
