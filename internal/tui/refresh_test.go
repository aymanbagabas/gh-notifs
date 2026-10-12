package tui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"altscreen.io/vision/event"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestActivityBaseline(t *testing.T) {
	now := time.Now()
	item := notifs.Thread{ID: "1", Unread: true, UpdatedAt: now}
	for _, tc := range []struct {
		name string
		item notifs.Thread
		want bool
	}{
		{"unchanged", item, false},
		{"new thread", notifs.Thread{ID: "2", Unread: true, UpdatedAt: now.Add(time.Second)}, true},
		{"new activity", notifs.Thread{ID: "1", Unread: true, UpdatedAt: now.Add(time.Second)}, true},
		{"new read thread", notifs.Thread{ID: "2", UpdatedAt: now.Add(time.Second)}, false},
		{"older unseen row", notifs.Thread{ID: "2", Unread: true, UpdatedAt: now.Add(-time.Second)}, false},
		{"same-second arrival", notifs.Thread{ID: "2", Unread: true, UpdatedAt: now}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp()
			if a.observeActivity([]notifs.Thread{item}) {
				t.Fatal("initial baseline produced an alert")
			}
			if got := a.observeActivity([]notifs.Thread{tc.item}); got != tc.want {
				t.Fatalf("new activity=%t, want %t", got, tc.want)
			}
			if a.observeActivity(nil) || a.observeActivity([]notifs.Thread{tc.item}) {
				t.Fatal("a repeated or returning row produced another alert")
			}
		})
	}
	a := testApp()
	a.observeActivity(nil)
	if !a.observeActivity([]notifs.Thread{item}) {
		t.Fatal("an empty inbox never detected its first arrival")
	}
}

func TestAutomaticRefreshScheduling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		release := make(chan struct{})
		a.client = &notifs.Client{Cache: cache, BaseURL: "https://api.example/",
			HTTP: &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"X-Poll-Interval": {"120"}},
					Body: io.NopCloser(strings.NewReader("[]"))}, nil
			})}}
		a.messages = make(chan message, 8)
		ctx, cancel := context.WithCancel(context.Background())
		defer func() { cancel(); a.workers.Wait() }()
		a.nextRefresh = time.Now()
		a.refreshIfDue(ctx, time.Now())
		if a.loading {
			t.Fatal("automatic refresh is enabled by default")
		}
		a.autoRefresh = true
		for _, busy := range []*bool{&a.loading, &a.acting, &a.edit, &a.pasting} {
			*busy = true
			generation := a.generation
			a.refreshIfDue(ctx, time.Now())
			if a.generation != generation {
				t.Fatal("automatic refresh interrupted an active operation")
			}
			*busy = false
		}
		a.pendingAction = "done"
		a.refreshIfDue(ctx, time.Now())
		if a.loading {
			t.Fatal("automatic refresh interrupted a confirmation")
		}
		a.pendingAction = ""
		a.nextRefresh = time.Now().Add(time.Second)
		a.refreshIfDue(ctx, time.Now())
		if a.loading {
			t.Fatal("automatic refresh started before its deadline")
		}
		time.Sleep(time.Second)
		oldError := errors.New("previous refresh failed")
		a.err = oldError
		a.refreshIfDue(ctx, time.Now())
		synctest.Wait()
		if !a.loading || !a.autoFetching || calls != 1 || a.err != oldError {
			t.Fatalf("automatic start: loading=%t auto=%t calls=%d error=%v", a.loading, a.autoFetching, calls, a.err)
		}
		a.refreshIfDue(ctx, time.Now())
		synctest.Wait()
		if calls != 1 {
			t.Fatal("refresh requests overlapped")
		}
		close(release)
		a.workers.Wait()
		if len(a.messages) != 1 {
			t.Fatal("automatic refresh published partial results")
		}
		a.receive(ctx, <-a.messages)
		if a.loading || a.err != oldError || a.nextRefresh != time.Now().Add(time.Minute) {
			t.Fatal("successful refresh lost the error or did not reset its deadline")
		}
		time.Sleep(time.Minute)
		a.refreshIfDue(ctx, time.Now())
		if a.loading {
			t.Fatal("refresh ignored the longer server interval")
		}
		time.Sleep(time.Minute)
		a.refreshIfDue(ctx, time.Now())
		a.workers.Wait()
		if calls != 2 {
			t.Fatal("refresh did not resume at the server deadline")
		}
	})
}

func TestAutomaticRefreshPreservesPreviewAndErrors(t *testing.T) {
	a := testApp()
	a.autoRefresh = true
	a.autoFetching, a.showPreview = true, true
	a.previewID = a.current().ID
	a.preview = &notifs.Preview{Title: "Retained"}
	a.previewOffset, a.metadataOffset = 3, 2
	a.discussions[0].pages = []notifs.DiscussionPage{{Total: 1}}
	a.selected["1"] = true
	result := a.result
	a.receive(context.Background(), message{generation: a.generation, result: &result, final: true})
	if a.preview == nil || a.preview.Title != "Retained" || a.previewOffset != 3 ||
		a.metadataOffset != 2 || len(a.discussions[0].pages) != 1 || !a.selected["1"] {
		t.Fatal("unchanged automatic result disrupted the current item")
	}
	previousActivity := a.latestActivity
	failure := errors.New("offline")
	partial := notifs.Result{Items: []notifs.Thread{{ID: "3", Unread: true, UpdatedAt: time.Now().Add(time.Hour)}}}
	a.receive(context.Background(), message{generation: a.generation, result: &partial, final: true, err: failure})
	if a.err != failure || !reflect.DeepEqual(a.result, result) || a.latestActivity != previousActivity || a.loading {
		t.Fatal("failed automatic refresh replaced the last successful result or baseline")
	}
	a.receive(context.Background(), message{generation: a.generation - 1, result: &partial, final: true})
	if !reflect.DeepEqual(a.result, result) {
		t.Fatal("stale refresh result was accepted")
	}
	// Preview workers use their own generation and can finish across unchanged refreshes.
	a.receive(context.Background(), message{generation: a.generation - 1, previewGeneration: a.previewGeneration,
		previewID: a.current().ID, preview: &notifs.Preview{Title: "Finished"}})
	if a.preview.Title != "Finished" {
		t.Fatal("unchanged refresh discarded an in-flight preview")
	}
}

func TestQueryChangeResetsActivity(t *testing.T) {
	a := testApp()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	a.client.Cache = cache
	a.observeActivity(a.result.Items)
	a.messages = make(chan message, 8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.startFetch(ctx, true, false)
	a.workers.Wait()
	if a.activity != nil || !a.latestActivity.IsZero() {
		t.Fatal("query change retained the old notification baseline")
	}
}

func TestAutomaticRefreshKeepsLoadedWindow(t *testing.T) {
	a := testApp()
	cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
	if err != nil {
		t.Fatal(err)
	}
	a.client.Cache = cache
	a.client.BaseURL = "https://api.example/"
	a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`[{"id":"1"},{"id":"2"},{"id":"3"},{"id":"4"}]`))}, nil
	})}
	a.result.Items = append(a.result.Items, notifs.Thread{ID: "3"}, notifs.Thread{ID: "4"})
	a.messages = make(chan message, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); a.workers.Wait() }()
	a.startFetch(ctx, false, true)
	a.workers.Wait()
	msg := <-a.messages
	if msg.err != nil || len(msg.result.Items) != 4 {
		t.Fatalf("automatic refresh discarded previously loaded pages: %+v, %v", msg.result, msg.err)
	}
	a.receive(ctx, msg)
	a.key(ctx, event.Key{Code: 'r'})
	a.workers.Wait()
	if !a.notifyRefresh || a.autoFetching {
		t.Fatal("manual refresh did not enable new-activity alerts")
	}
	a.receive(ctx, <-a.messages)
	a.receive(ctx, <-a.messages)
	a.fetch(ctx, false)
	a.workers.Wait()
	if a.notifyRefresh {
		t.Fatal("pagination or action refresh inherited the manual bell setting")
	}
}

func TestAutomaticRefreshCancelsOnExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
		if err != nil {
			t.Fatal(err)
		}
		a.client.Cache = cache
		a.client.BaseURL = "https://api.example/"
		canceled := false
		a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			canceled = true
			return nil, r.Context().Err()
		})}
		a.messages = make(chan message, 8)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.startFetch(ctx, false, true)
		synctest.Wait()
		cancel()
		a.workers.Wait()
		if !canceled {
			t.Fatal("in-flight automatic request did not observe cancellation")
		}
	})
}
