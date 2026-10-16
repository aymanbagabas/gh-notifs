package tui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestDiscussionDebounceAndCachedPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		a.showPreview = true
		a.options.NoColor = true
		a.messages = make(chan message, 8)
		ctx, cancel := context.WithCancel(context.Background())
		defer func() { cancel(); a.workers.Wait() }()
		calls := make(chan string, 8)
		a.client.BaseURL, a.client.GraphQL = "https://api.example/", "https://api.example/graphql"
		a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
			var request struct {
				Query     string
				Variables struct{ Cursor *string }
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, err
			}
			cursor := ""
			if request.Variables.Cursor != nil {
				cursor = *request.Variables.Cursor
			}
			calls <- cursor
			body := `{"data":{"repository":{"issueOrPullRequest":{"conversation":{"totalCount":2,"pageInfo":{"endCursor":"next","hasNextPage":true},"nodes":[{"body":"First comment","author":{"login":"author"}}]}}}}}`
			if cursor == "next" {
				body = `{"data":{"repository":{"issueOrPullRequest":{"conversation":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"body":"Second comment"}]}}}}}`
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		a.selectPreviewTab(ctx, 1)
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		a.selectPreviewTab(ctx, 2)
		synctest.Wait()
		time.Sleep(150 * time.Millisecond)
		a.selectPreviewTab(ctx, 1)
		synctest.Wait()
		time.Sleep(199 * time.Millisecond)
		synctest.Wait()
		if len(calls) != 0 {
			t.Fatal("discussion requested before the tab settled")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		a.receive(ctx, <-a.messages)
		if len(calls) != 1 || len(a.discussions[0].pages) != 1 {
			t.Fatal("settled discussion did not load once")
		}
		lines, err := a.discussionLines(60)
		if err != nil || !strings.Contains(strings.Join(lines, "\n"), "First comment") || a.preview != nil {
			t.Fatal("discussion could not render independently of the body")
		}
		a.changeDiscussionPage(ctx, 1)
		synctest.Wait()
		time.Sleep(previewDebounce)
		synctest.Wait()
		a.receive(ctx, <-a.messages)
		if a.discussions[0].page != 1 || len(a.discussions[0].pages) != 2 || len(calls) != 2 {
			t.Fatal("next discussion page did not load")
		}
		a.changeDiscussionPage(ctx, -1)
		synctest.Wait()
		time.Sleep(previewDebounce)
		synctest.Wait()
		if a.discussions[0].page != 0 || len(calls) != 2 {
			t.Fatal("previous discussion page fetched instead of reusing its cache")
		}
		a.key(ctx, event.Key{Code: 'v'})
		a.receive(ctx, message{generation: a.generation, previewGeneration: a.previewGeneration,
			previewID: "1", discussionTab: 1, discussionGeneration: 1, discussion: &notifs.DiscussionPage{Total: 99}})
		if len(a.discussions[0].pages) != 0 {
			t.Fatal("closed preview accepted a discussion response")
		}
	})
}

func TestDiscussionErrorsAndStaleResponsesStayInTheirPane(t *testing.T) {
	a := testApp()
	a.showPreview, a.previewTab = true, 1
	a.preview = &notifs.Preview{Title: "Preserved body", Body: "Keep this"}
	a.discussions[0].generation = 3
	msg := message{generation: a.generation, previewGeneration: a.previewGeneration, previewID: "1",
		discussionTab: 1, discussionGeneration: 2, err: errors.New("Permission denied")}
	a.receive(context.Background(), msg)
	if a.discussions[0].err != nil {
		t.Fatal("superseded discussion response was accepted")
	}
	msg.discussionGeneration = 3
	a.receive(context.Background(), msg)
	lines, err := a.discussionLines(60)
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "Permission denied") ||
		a.preview.Body != "Keep this" || a.err != nil {
		t.Fatal("discussion error was hidden or replaced the body")
	}
}

func TestPreviewTabPadding(t *testing.T) {
	for _, width := range []int{40, 85, 86, 87, 120} {
		a := testApp()
		a.showPreview = true
		a.metadataWidth = 1000
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 50})
		a.paint(sc)
		count := 0
		for _, target := range a.hits {
			if target.kind != "preview-tab" {
				continue
			}
			want := []string{" 1 Body ", " 2 Discussion ", " 3 Threads "}[target.index]
			var got strings.Builder
			for x := target.rect.Min.X; x < target.rect.Max.X; x++ {
				got.WriteString(sc.CellAt(x, target.rect.Min.Y).Content())
			}
			if got.String() != want {
				t.Fatalf("width %d: tab %d has %q, want %q", width, target.index, got.String(), want)
			}
			count++
		}
		if count != 3 {
			t.Fatalf("width %d: only %d padded tabs fit", width, count)
		}
	}
}

func TestDiscussionMouseTabsAndPagination(t *testing.T) {
	a := testApp()
	a.showPreview, a.previewTab = true, 1
	a.preview = &notifs.Preview{Title: "Description", Body: "Original body"}
	a.discussions[0].pages = []notifs.DiscussionPage{
		{Total: 2, More: true, Comments: []notifs.DiscussionComment{{Author: "@one", Body: "First page message"}}},
		{Total: 2, Comments: []notifs.DiscussionComment{{Author: "@two", Body: "Second page message"}}},
	}
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 50})
	a.paint(sc)
	next := findHit(t, a, "discussion-next")
	a.mouse(context.Background(), event.MouseClickEvent{X: next.rect.Min.X, Y: next.rect.Min.Y, Button: event.MouseLeft}, sc)
	a.paint(sc)
	if a.discussions[0].page != 1 || !strings.Contains(text.String(sc, color.Disabled), "Second page message") {
		t.Fatal("discussion Next button did not switch pages")
	}
	for _, target := range a.hits {
		if target.kind == "preview-tab" && target.index == 0 {
			a.mouse(context.Background(), event.MouseClickEvent{X: target.rect.Min.X, Y: target.rect.Min.Y, Button: event.MouseLeft}, sc)
			break
		}
	}
	a.paint(sc)
	if a.previewTab != 0 || !strings.Contains(text.String(sc, color.Disabled), "Original body") {
		t.Fatal("Body tab did not restore the description")
	}
}
