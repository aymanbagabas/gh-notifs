package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestManualRefreshDoesNotOverlap(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	a := testApp()
	a.client = &notifs.Client{HTTP: server.Client(), BaseURL: server.URL + "/", Cache: cache}
	a.messages = make(chan message, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); a.workers.Wait() }()
	a.key(ctx, event.Key{Code: 'r'})
	if !a.loading {
		t.Fatal("manual refresh did not start")
	}
	a.key(ctx, event.Key{Code: 'r'})
	for {
		select {
		case msg := <-a.messages:
			a.receive(ctx, msg)
			if msg.final {
				if msg.err != nil || calls.Load() != 1 {
					t.Fatalf("refresh failed or overlapped: %v, %d requests", msg.err, calls.Load())
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("manual refresh did not finish")
		}
	}
}

func TestPageNavigationAndBounds(t *testing.T) {
	a := testApp()
	a.result.Items = append(a.result.Items, notifs.Thread{ID: "3"}, notifs.Thread{ID: "4"}, notifs.Thread{ID: "5"})
	ctx := context.Background()
	a.key(ctx, event.Key{Code: 'g', Mod: event.KeyModShift})
	if a.cursor != 1 || a.page != 0 {
		t.Fatal("G did not select the current page's last item")
	}
	a.key(ctx, event.Key{Code: ']'})
	if a.page != 1 || a.cursor != 2 || a.loading {
		t.Fatal("loaded page navigation fetched again or selected the wrong item")
	}
	a.key(ctx, event.Key{Code: 'g', Mod: event.KeyModShift})
	a.key(ctx, event.Key{Code: 'g'})
	if a.cursor != 2 || a.page != 1 {
		t.Fatal("g crossed a page boundary")
	}
	a.key(ctx, event.Key{Code: ']'})
	a.key(ctx, event.Key{Code: ']'})
	if a.page != 2 || a.cursor != 4 || a.hasNextPage() {
		t.Fatal("pagination passed the final page")
	}
	a.key(ctx, event.Key{Code: '['})
	if a.page != 1 || a.cursor != 2 {
		t.Fatal("previous page did not select its first item")
	}
	a.sidebar = true
	a.key(ctx, event.Key{Code: 'g', Mod: event.KeyModShift})
	if a.sideIndex != len(a.sideEntries())-1 {
		t.Fatal("G did not select the sidebar's last entry")
	}
	a.key(ctx, event.Key{Code: 'g'})
	if a.sideIndex != 0 || a.cursor != 2 {
		t.Fatal("g did not select the sidebar's first entry")
	}
}

func TestPolishedLayout(t *testing.T) {
	a := testApp()
	a.result.Items[0].Reason = "review_requested"
	for _, width := range []int{40, 60, 120} {
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
		a.paint(sc)
		out := text.String(sc, color.Disabled)
		for _, want := range []string{"╭", "╯", "─", "[ Previous", "Next ]", "Page 1/1"} {
			if !strings.Contains(out, want) {
				t.Fatalf("width %d: missing %q", width, want)
			}
		}
		if width == 120 {
			for _, want := range []string{"┬", "┴", "⇄", "review-requested", "Refreshed"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q", want)
				}
			}
		}
	}
}
