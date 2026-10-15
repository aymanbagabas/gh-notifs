package tui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func TestRefreshKeepsCachedAuthorsInProgress(t *testing.T) {
	a := testApp()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	item := a.result.Items[0]
	item.Author, item.AuthorID, item.State = "oldname", "U_42", "open"
	cache.Put(notifs.Endpoint(a.query, 1), notifs.Page{
		Items: []notifs.Thread{item}, ETag: `"inbox"`, CheckedAt: time.Now(),
	})
	a.client.Cache = cache
	a.client.BaseURL, a.client.GraphQL = "https://api.example/", "https://api.example/graphql"
	a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/notifications":
			return &http.Response{StatusCode: http.StatusNotModified, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "/graphql":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
				`{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":"newname"}}}}}`))}, nil
		default:
			return nil, errors.New("unexpected request: " + r.URL.Path)
		}
	})}
	a.messages = make(chan message, 8)
	a.result = a.client.Cached(a.query, a.options.PageSize)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		a.workers.Wait()
	}()
	a.fetch(ctx, false)
	a.workers.Wait()
	for _, name := range []string{"oldname", "oldname", "newname"} {
		msg := <-a.messages
		if msg.err != nil || msg.final != (name == "newname") {
			t.Fatalf("unexpected refresh message: %+v", msg)
		}
		a.receive(ctx, msg)
		if current := a.current(); current == nil || current.Author != name || current.AuthorID != "U_42" {
			t.Fatalf("refresh lost the displayed author: %+v", current)
		}
	}
}

func TestFailedNextPageKeepsUsableSelection(t *testing.T) {
	a := testApp()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("offline pagination failure")
	a.client.Cache = cache
	a.client.BaseURL = "https://api.example/"
	a.client.HTTP = &http.Client{Transport: previewTestTransport(func(*http.Request) (*http.Response, error) {
		return nil, failure
	})}
	a.messages = make(chan message, 8)
	a.result.More = true
	a.selected["1"] = true
	previous := a.result
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		a.workers.Wait()
	}()
	for range 2 {
		a.key(ctx, event.Key{Code: ']'})
		if !a.loading || a.page != 1 || a.cursor != 2 {
			t.Fatalf("next-page request did not start: page=%d cursor=%d loading=%t", a.page, a.cursor, a.loading)
		}
		a.workers.Wait()
		msg := <-a.messages
		if !msg.final || !errors.Is(msg.err, failure) {
			t.Fatalf("unexpected fetch response: %+v", msg)
		}
		a.receive(ctx, msg)
		if a.loading || a.page != 0 || a.current() == nil || a.cursor >= len(a.result.Items) {
			t.Fatalf("failed pagination left an invalid selection: page=%d cursor=%d current=%+v", a.page, a.cursor, a.current())
		}
		if !reflect.DeepEqual(a.result, previous) || !a.selected["1"] || !errors.Is(a.err, failure) {
			t.Fatal("failed pagination lost verified results, selection, or the error")
		}
	}
}

func TestHighlightedItemCounter(t *testing.T) {
	for _, width := range []int{40, 120} {
		for _, help := range []bool{false, true} {
			for _, tt := range []struct {
				cursor, count int
				more          bool
				want          string
			}{
				{0, 4, false, "Item 1/4"},
				{1, 4, false, "Item 2/4"},
				{3, 4, false, "Item 4/4"},
				{3, 4, true, "Item 4/4+"},
				{4, 4, true, "Item -/4+"},
				{0, 0, false, "Item 0/0"},
			} {
				a := testApp()
				a.result.Items = append(a.result.Items, notifs.Thread{ID: "3"}, notifs.Thread{ID: "4"})
				a.result.Items = a.result.Items[:tt.count]
				a.result.More = tt.more
				a.cursor, a.page = tt.cursor, tt.cursor/a.options.PageSize
				a.selected["1"], a.selected["2"] = true, true
				a.help, a.showPreview = help, true
				sc := screen.New(io.Discard, layout.Size{Width: width, Height: 40})
				a.paint(sc)
				y := 37
				if help {
					y = findHit(t, a, "help").rect.Min.Y + 1
				}
				var row strings.Builder
				for x := 0; x < width; x++ {
					row.WriteString(sc.CellAt(layout.Pos(x, y)).Content())
				}
				if !strings.HasSuffix(row.String(), tt.want+" ") || !strings.Contains(row.String(), "Refreshed") {
					t.Fatalf("width %d, help=%v: footer %q, want %q at right", width, help, row.String(), tt.want)
				}
			}
		}
	}
}

func TestPaginationButtonsShareHeader(t *testing.T) {
	for _, width := range []int{40, 60, 120} {
		a := testApp()
		a.result.More = true
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
		a.paint(sc)
		var row strings.Builder
		for x := 0; x < width; x++ {
			row.WriteString(sc.CellAt(layout.Pos(x, 4)).Content())
		}
		for _, want := range []string{"Page 1", "[ Previous", "Next ]"} {
			if !strings.Contains(row.String(), want) {
				t.Fatalf("width %d: header missing %q", width, want)
			}
		}
		if sc.CellAt(layout.Pos(width-22, 4)).Style.GetBackground() != theme.Colors(false).Surface ||
			sc.CellAt(layout.Pos(width-9, 4)).Style.GetBackground() != theme.Colors(false).Selection {
			t.Fatal("enabled and disabled buttons lack distinct backgrounds")
		}
		for _, h := range a.hits {
			if h.kind == "[" || h.kind == "]" {
				if h.rect.Min.Y != 4 || h.kind == "[" {
					t.Fatal("button hit area is misplaced or disabled button is clickable")
				}
			}
		}
		a.result.Items = append(a.result.Items, notifs.Thread{ID: "3"})
		a.page, a.cursor = 1, 2
		a.paint(sc)
		a.mouse(context.Background(), event.MouseClickEvent{X: width - 20, Y: 4, Button: event.MouseLeft}, sc)
		if a.page != 0 {
			t.Fatal("Previous click did not use the new header position")
		}
	}
}
