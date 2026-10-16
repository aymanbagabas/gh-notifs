package tui

import (
	"context"
	"io"
	"net/http"
	"reflect"
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

func TestFloatingDialogDragAndCancel(t *testing.T) {
	a := testApp()
	a.confirmAction("done")
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	ctx := context.Background()
	a.paint(sc)
	original := a.dialogRect
	if original.Min.X <= 0 || original.Min.Y <= 0 || !strings.Contains(text.String(sc, color.Disabled), "Confirm action") {
		t.Fatal("confirmation is not a centered floating dialog")
	}
	for x := original.Min.X + 2 + sc.StringWidth(" Confirm action "); x < original.Max.X-3; x++ {
		if sc.CellAt(x, original.Min.Y).Content() != "─" {
			t.Fatal("dialog title erased the top border")
		}
	}
	a.mouse(ctx, event.MouseClickEvent{X: 90, Y: 9, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseWheelEvent{X: 90, Y: 9, Button: event.MouseWheelDown}, sc)
	a.key(ctx, event.Key{Code: 'j'})
	if a.cursor != 0 || a.pendingAction != "done" {
		t.Fatal("dialog did not block background input")
	}
	title := findHit(t, a, "dialog-title")
	a.mouse(ctx, event.MouseClickEvent{X: title.rect.Min.X + 3, Y: title.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseMoveEvent{X: title.rect.Min.X + 13, Y: title.rect.Min.Y + 4, Button: event.MouseLeft}, sc)
	a.mouse(ctx, event.MouseReleaseEvent{Button: event.MouseLeft}, sc)
	a.paint(sc)
	if a.dialogRect.Min != original.Min.Add(layout.Pos(10, 4)) {
		t.Fatal("dialog drag lost the pointer offset")
	}
	a.key(ctx, event.Key{Code: event.KeyTab})
	if !a.dialogConfirm {
		t.Fatal("Tab did not focus Confirm")
	}
	a.key(ctx, event.Key{Code: event.KeyTab})
	a.key(ctx, event.Key{Code: event.KeyEnter})
	if a.pendingAction != "" || a.acting {
		t.Fatal("Enter on Cancel did not dismiss the dialog")
	}
}

func TestDialogButtonHover(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, noColor := range []bool{false, true} {
			a := testApp()
			a.light, a.options.NoColor = light, noColor
			a.confirmAction("done")
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
			a.paint(sc)
			for _, target := range append([]hit(nil), a.hits...) {
				if target.kind != "confirm" && target.kind != "cancel" {
					continue
				}
				pos := target.rect.Min
				before := sc.CellAt(pos.X, pos.Y).Style
				a.mouse(context.Background(), event.MouseMoveEvent{X: pos.X, Y: pos.Y, Button: event.MouseNone}, sc)
				a.paint(sc)
				after := sc.CellAt(pos.X, pos.Y).Style
				if a.dialogHover != target.index || reflect.DeepEqual(before, after) {
					t.Fatalf("dialog control %d did not highlight on hover", target.index)
				}
				if !noColor && after.GetBackground() != theme.Colors(light).Accent {
					t.Fatal("hovered dialog button has the wrong background")
				}
				if a.dialogConfirm || a.pendingAction != "done" || a.acting {
					t.Fatal("hover activated a button or changed keyboard focus")
				}
				a.mouse(context.Background(), event.MouseMoveEvent{X: 0, Y: 0, Button: event.MouseNone}, sc)
				a.paint(sc)
				if a.dialogHover != 0 || !reflect.DeepEqual(before, sc.CellAt(pos.X, pos.Y).Style) {
					t.Fatal("dialog button kept its hover style after the pointer left")
				}
			}
		}
	}
}

func TestDialogClampsAfterResizeAndClosesByMouse(t *testing.T) {
	a := testApp()
	a.confirmAction("unsubscribe")
	a.dialogMoved, a.dialogPosition = true, layout.Pos(100, 50)
	sc := screen.New(io.Discard, layout.Size{Width: 40, Height: 10})
	a.paint(sc)
	if !a.dialogRect.In(sc.Bounds()) {
		t.Fatal("dialog moved outside the resized terminal")
	}
	cancel := findHit(t, a, "cancel")
	a.mouse(context.Background(), event.MouseClickEvent{X: cancel.rect.Min.X, Y: cancel.rect.Min.Y, Button: event.MouseLeft}, sc)
	if a.pendingAction != "" || a.acting {
		t.Fatal("close control did not cancel the dialog")
	}
}

func TestDialogConfirmsOriginalTargets(t *testing.T) {
	a := testApp()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "dialog-test")
	if err != nil {
		t.Fatal(err)
	}
	paths := make(chan string, 2)
	a.client = &notifs.Client{BaseURL: "https://api.example/", Cache: cache,
		HTTP: &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
			paths <- r.URL.Path
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(""))}, nil
		})}}
	a.messages = make(chan message, 8)
	a.confirmAction("done")
	a.cursor = 1
	a.selected["2"] = true
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	confirm := findHit(t, a, "confirm")
	a.mouse(context.Background(), event.MouseClickEvent{X: confirm.rect.Min.X, Y: confirm.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.workers.Wait()
	if len(paths) != 1 {
		t.Fatalf("expected one action, got %d", len(paths))
	}
	if path := <-paths; path != "/notifications/threads/1" {
		t.Fatalf("dialog acted on a changed selection: %s", path)
	}
	if msg := <-a.messages; msg.err != nil {
		t.Fatal(msg.err)
	}
}
