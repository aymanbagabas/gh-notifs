package tui

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

type previewTestTransport func(*http.Request) (*http.Response, error)

func (f previewTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestPreviewDebouncesKeyboardAndMouseNavigation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		a.showPreview = true
		a.messages = make(chan message, 8)
		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			cancel()
			a.workers.Wait()
		}()
		requests := make(chan string, 8)
		a.client.BaseURL = "https://api.example/"
		a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
			requests <- r.URL.Path
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{"title":"Preview","body":"Body"}`))}, nil
		})}
		for i := range a.result.Items {
			a.result.Items[i].Type = "Release"
			a.result.Items[i].SubjectURL = a.client.BaseURL + a.result.Items[i].ID
		}
		sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
		a.paint(sc)
		a.loadPreview(ctx)
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		a.move(ctx, 1)
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		a.mouse(ctx, event.MouseClickEvent{X: 60, Y: 6, Button: event.MouseLeft}, sc)
		synctest.Wait()
		time.Sleep(199 * time.Millisecond)
		synctest.Wait()
		if len(requests) != 0 {
			t.Fatalf("navigation sent %d requests before settling", len(requests))
		}
		a.move(ctx, -1)
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if len(requests) != 1 {
			t.Fatalf("expected only the final selection to load, got %d requests", len(requests))
		}
		if path := <-requests; path != "/1" {
			t.Fatalf("loaded the wrong selection: %s", path)
		}
		a.receive(ctx, <-a.messages)
		if a.preview == nil || a.preview.Title != "Preview" {
			t.Fatal("settled selection did not display its preview")
		}
	})
}

func TestClosingPreviewCancelsDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		a.messages = make(chan message, 8)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.key(ctx, event.Key{Code: 'v'})
		synctest.Wait()
		time.Sleep(100 * time.Millisecond)
		a.key(ctx, event.Key{Code: 'v'})
		a.workers.Wait()
		if len(a.messages) != 0 {
			t.Fatal("closed preview sent a request")
		}
	})
}

func TestPreviewIgnoresSupersededResponseForSameItem(t *testing.T) {
	a := testApp()
	a.showPreview = true
	a.previewGeneration = 3
	msg := message{generation: a.generation, previewGeneration: 1, previewID: "1",
		preview: &notifs.Preview{Title: "Old response"}}
	a.receive(context.Background(), msg)
	if a.preview != nil {
		t.Fatal("superseded response replaced the latest selection")
	}
	msg.previewGeneration = 3
	a.showPreview = false
	a.receive(context.Background(), msg)
	if a.preview != nil {
		t.Fatal("response updated a closed preview")
	}
}
