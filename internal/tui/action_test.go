package tui

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestActionsUpdateBeforeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, action, query string
		failAt              int
		pending, completed  []string
	}{
		{"read", "read", "", 0, []string{"1:read", "2:read", "3:unread"}, []string{"1:read", "2:read", "3:unread"}},
		{"unread view", "read", "is:unread", 0, []string{"3:unread"}, []string{"3:unread"}},
		{"both states", "read", "is:unread is:read", 0, []string{"1:read", "2:read", "3:unread"}, []string{"1:read", "2:read", "3:unread"}},
		{"participating", "read", "reason:participating", 0, []string{"1:read", "2:read", "3:unread"}, []string{"1:read", "2:read", "3:unread"}},
		{"done", "done", "", 0, []string{"3:unread"}, []string{"3:unread"}},
		{"read fails", "read", "", 1, []string{"1:read", "2:read", "3:unread"}, []string{"1:unread", "2:unread", "3:unread"}},
		{"done fails", "done", "", 1, []string{"3:unread"}, []string{"1:unread", "2:unread", "3:unread"}},
		{"partial read", "read", "is:unread", 2, []string{"3:unread"}, []string{"2:unread", "3:unread"}},
		{"partial done", "done", "", 2, []string{"3:unread"}, []string{"2:unread", "3:unread"}},
		{"subscribe", "subscribe", "", 0, []string{"1:unread", "2:unread", "3:unread"}, []string{"1:unread", "2:unread", "3:unread"}},
		{"unsubscribe fails", "unsubscribe", "", 1, []string{"1:unread", "2:unread", "3:unread"}, []string{"1:unread", "2:unread", "3:unread"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := testApp()
				a.query, _ = notifs.ParseQuery(tc.query)
				a.result.Items[1].Unread = true
				a.result.Items = append(a.result.Items, notifs.Thread{ID: "3", Unread: true})
				a.selected["1"], a.selected["2"] = true, true
				before := append([]notifs.Thread(nil), a.result.Items...)
				original := a.result.Items
				cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
				if err != nil {
					t.Fatal(err)
				}
				release := make(chan struct{})
				calls := 0
				a.client = &notifs.Client{Cache: cache, BaseURL: "https://api.example/",
					HTTP: &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
						if r.Method == http.MethodGet {
							<-r.Context().Done()
							return nil, r.Context().Err()
						}
						calls++
						select {
						case <-release:
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
						code := http.StatusNoContent
						if calls == tc.failAt {
							code = http.StatusForbidden
						}
						return &http.Response{StatusCode: code, Header: make(http.Header), Request: r,
							Body: io.NopCloser(strings.NewReader(`{"message":"denied"}`))}, nil
					})}}
				a.messages = make(chan message, 8)
				ctx, cancel := context.WithCancel(context.Background())
				defer func() { cancel(); a.workers.Wait() }()
				generation := a.generation
				a.act(ctx, tc.action, []string{"1", "2"})
				synctest.Wait()
				check := func(want []string) {
					t.Helper()
					var got []string
					for _, item := range a.result.Items {
						state := "read"
						if item.Unread {
							state = "unread"
						}
						got = append(got, item.ID+":"+state)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("items=%v, want %v", got, want)
					}
				}
				if !a.acting || calls != 1 {
					t.Fatal("the action did not wait for its response")
				}
				check(tc.pending)
				if !reflect.DeepEqual(original, before) {
					t.Fatal("optimistic changes mutated the rollback snapshot")
				}
				if tc.action == "subscribe" || tc.action == "unsubscribe" {
					if !strings.HasSuffix(a.itemReason(a.result.Items[0]), "scribing...") {
						t.Fatal("subscription action has no pending label")
					}
				}
				a.receive(ctx, message{generation: generation, result: &notifs.Result{Items: before}, final: true})
				check(tc.pending)
				close(release)
				msg := <-a.messages
				a.receive(ctx, msg)
				check(tc.completed)
				if a.acting || (a.err != nil) != (tc.failAt != 0) {
					t.Fatalf("completion: acting=%v, error=%v", a.acting, a.err)
				}
				if len(a.actionPending) != 0 {
					t.Fatal("completed action retained pending labels")
				}
				if a.current() == nil || a.page != a.cursor/a.options.PageSize {
					t.Fatal("action left the cursor on an invalid page")
				}
				if tc.failAt == 2 && (a.selected["1"] || !a.selected["2"]) {
					t.Fatal("partial failure lost the failed selection or retained a successful selection")
				}
			})
		})
	}
}

func TestActionRemovalPreservesFocusAndPreview(t *testing.T) {
	a := testApp()
	a.result.Items = append(a.result.Items, notifs.Thread{ID: "3"}, notifs.Thread{ID: "4"})
	a.cursor, a.page = 3, 1
	a.preview, a.previewID = &notifs.Preview{}, "4"
	ctx := context.Background()
	a.replaceActionItems(ctx, a.actionItems(a.result.Items, "done", []string{"1", "2", "3"}))
	if a.current().ID != "4" || a.cursor != 0 || a.page != 0 || a.preview == nil || a.previewID != "4" {
		t.Fatal("removing earlier pages moved focus or cleared the unaffected preview")
	}
	a.replaceActionItems(ctx, a.actionItems(a.result.Items, "done", []string{"4"}))
	if a.current() != nil || a.cursor != 0 || a.page != 0 || a.preview != nil || a.previewID != "" {
		t.Fatal("removing the last item retained a cursor or stale preview")
	}
}
