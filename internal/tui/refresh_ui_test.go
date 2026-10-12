package tui

import (
	"context"
	"io"
	"net/http"
	"reflect"
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

func TestRefreshControls(t *testing.T) {
	a := testApp()
	options := a.options
	ctx := context.Background()
	a.key(ctx, event.Key{Code: 'r', Mod: event.KeyModShift})
	a.key(ctx, event.Key{Code: 'b', Mod: event.KeyModShift})
	if !a.autoRefresh || !a.bell || a.nextRefresh.IsZero() {
		t.Fatal("keyboard toggles did not enable refresh and bells")
	}
	for _, width := range []int{40, 80, 120} {
		sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
		a.paint(sc)
		out := text.String(sc, color.Disabled)
		if !strings.Contains(out, "R Auto:on") || !strings.Contains(out, "B Bell:on") {
			t.Fatalf("missing controls at width %d:\n%s", width, out)
		}
	}
	sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
	for _, action := range []string{"shift+r", "shift+b"} {
		a.paint(sc)
		found := false
		for _, target := range a.hits {
			if target.kind == action {
				a.mouse(ctx, event.MouseClickEvent{X: target.rect.Min.X, Y: target.rect.Min.Y, Button: event.MouseLeft}, sc)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no mouse target for %s", action)
		}
	}
	if a.autoRefresh || a.bell || !reflect.DeepEqual(a.options, options) {
		t.Fatal("mouse toggles failed or modified the startup options")
	}
	updated := time.Now()
	a.result.Items[0].UpdatedAt = updated
	a.observeActivity(a.result.Items)
	deadline := time.Now().Add(time.Minute)
	a.nextRefresh = deadline
	a.key(ctx, event.Key{Code: 'r', Mod: event.KeyModShift})
	if len(a.activity) != len(a.result.Items) || a.activity["1"] != updated ||
		a.latestActivity != updated || a.nextRefresh != deadline {
		t.Fatal("re-enabling refresh discarded the baseline or bypassed the deadline")
	}
}

func TestNextRefreshDisplayUsesServerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp()
		a.autoRefresh = true
		cache, err := notifs.OpenCache(t.TempDir(), "github.com", "test")
		if err != nil {
			t.Fatal(err)
		}
		a.client.Cache = cache
		a.client.BaseURL = "https://api.example/"
		a.client.HTTP = &http.Client{Transport: previewTestTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Poll-Interval": {"120"}},
				Body: io.NopCloser(strings.NewReader("[]"))}, nil
		})}
		if _, err := a.client.Fetch(context.Background(), notifs.Query{}, 2, nil); err != nil {
			t.Fatal(err)
		}
		a.nextRefresh = time.Now().Add(time.Minute)
		want := time.Now().Add(2 * time.Minute)
		if a.refreshDeadline() != want {
			t.Fatalf("display deadline differs from server deadline: %v", a.refreshDeadline())
		}
		for _, width := range []int{40, 80, 120} {
			sc := screen.New(io.Discard, layout.Size{Width: width, Height: 32})
			a.paint(sc)
			out := text.String(sc, color.Disabled)
			if !strings.Contains(out, "Next in 120s ("+want.Local().Format("15:04:05")+")") {
				t.Fatalf("missing next-refresh time at width %d:\n%s", width, out)
			}
		}
		sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 32})
		time.Sleep(time.Second)
		a.paint(sc)
		if !strings.Contains(text.String(sc, color.Disabled), "Next in 119s (") {
			t.Fatal("countdown did not advance")
		}
		time.Sleep(119 * time.Second)
		a.edit = true
		a.paint(sc)
		if !strings.Contains(text.String(sc, color.Disabled), "Next: waiting") {
			t.Fatal("deferred refresh shows an expired countdown")
		}
		a.loading = true
		a.paint(sc)
		if strings.Contains(text.String(sc, color.Disabled), "Next: waiting") {
			t.Fatal("active refresh still shows a deadline")
		}
		a.loading, a.autoRefresh = false, false
		a.paint(sc)
		if strings.Contains(text.String(sc, color.Disabled), "Next: waiting") {
			t.Fatal("disabled refresh still shows a deadline")
		}
	})
}
