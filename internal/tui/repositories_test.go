package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"github.com/aymanbagabas/gh-notifs/internal/config"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestRepositorySectionAndHistorySelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/history/notifications" {
			t.Errorf("repository selection used %s", r.URL.Path)
		}
		io.WriteString(w, "[]")
	}))
	defer server.Close()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	cache.Put("old", notifs.Page{Items: []notifs.Thread{{Repository: "acme/history"}}})
	a := testApp()
	a.client = &notifs.Client{HTTP: server.Client(), BaseURL: server.URL + "/", Cache: cache}
	a.repositories = a.client.RecentRepositories()
	a.options.Scopes = []config.Scope{{Name: "Inbox", Query: ""}}
	a.messages = make(chan message, 8)
	a.result.Items = nil
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	a.paint(sc)
	first := sc.CellAt(1, 4)
	second := sc.CellAt(1, 7)
	if first.Content() != "V" || second.Content() != "R" ||
		first.Style.GetForeground() != second.Style.GetForeground() ||
		!first.Style.GetBold() || !second.Style.GetBold() {
		t.Fatal("sidebar sections lack matching heading styles")
	}
	for x := 1; x < 24; x++ {
		if sc.CellAt(x, 6).Content() != " " {
			t.Fatal("sidebar sections lack a blank row")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); a.workers.Wait() }()
	a.mouse(ctx, event.MouseClickEvent{X: 5, Y: 8, Button: event.MouseLeft}, sc)
	if a.query.Raw != "repo:acme/history" {
		t.Fatalf("history click applied %q", a.query.Raw)
	}
	for {
		select {
		case msg := <-a.messages:
			a.receive(ctx, msg)
			if msg.final {
				if msg.err != nil {
					t.Fatal(msg.err)
				}
				entries := a.sideEntries()
				if len(entries) != 2 || entries[1].label != "acme/history" {
					t.Fatalf("empty query results erased history: %+v", entries)
				}
				a.edit, a.input, a.caret = true, []rune("repo:"), 5
				values, _, _ := a.suggestions()
				if !slices.Equal(values, []string{"repo:acme/history"}) {
					t.Fatalf("completion did not use history: %v", values)
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("repository fetch did not finish")
		}
	}
}

func TestRepositoryHistoryScrollAndKeyboard(t *testing.T) {
	a := testApp()
	a.repositories = nil
	for i := range 80 {
		a.repositories = append(a.repositories, notifs.Repository{Name: fmt.Sprintf("acme/repo-%02d", i)})
	}
	a.options.Scopes = []config.Scope{{Name: "Inbox", Query: ""}}
	a.sidebar = true
	sc := screen.New(io.Discard, layout.Size{Width: 80, Height: 20})
	a.key(context.Background(), event.Key{Code: event.KeyEnd})
	a.paint(sc)
	found := false
	for _, target := range a.hits {
		if target.kind == "scope" && target.index == 25 {
			found = true
			if target.rect.Min.Y < 4 || target.rect.Max.Y > 16 {
				t.Fatal("last repository lies outside the sidebar")
			}
		}
	}
	if !found || len(a.sideEntries()) != 26 || a.sideEntries()[25].label != "acme/repo-24" {
		t.Fatal("sidebar did not limit repositories to the first 25")
	}
	a.key(context.Background(), event.Key{Code: event.KeyHome})
	a.key(context.Background(), event.Key{Code: event.KeyDown})
	if a.sideIndex != 1 || a.sideEntries()[a.sideIndex].label != "acme/repo-00" {
		t.Fatal("keyboard navigation selected a heading or gap")
	}
	a.edit, a.input, a.caret = true, []rune("repo:"), 5
	values, _, _ := a.suggestions()
	if len(a.repositories) != 80 || len(values) != 80 || !slices.Contains(values, "repo:acme/repo-79") {
		t.Fatal("display limit removed repository history or completion candidates")
	}
}
