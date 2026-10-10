package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/text"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/cli/go-gh/v2/pkg/browser"
)

func TestPreviewBrowserStub(t *testing.T) {
	if os.Getenv("GH_NOTIFS_BROWSER_STUB") != "1" {
		return
	}
	fmt.Print(os.Args[len(os.Args)-1])
	os.Exit(0)
}

func TestPreviewBrowserTarget(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_NOTIFS_BROWSER_STUB", "1")
	for _, failure := range []string{"none", "request", "metadata"} {
		for _, visible := range []bool{false, true} {
			t.Run(fmt.Sprintf("failure=%s/visible=%t", failure, visible), func(t *testing.T) {
				a := testApp()
				a.current().URL = "https://github.com/acme/app/pull/7"
				a.showPreview = true
				preview := notifs.Preview{URL: "https://github.com/acme/app/pull/7#discussion"}
				var previewErr error
				want := preview.URL
				switch failure {
				case "request":
					preview, previewErr = notifs.Preview{}, errors.New("preview request failed")
					want = a.current().URL
				case "metadata":
					previewErr = errors.New("preview metadata incomplete")
				}
				a.receive(context.Background(), message{
					generation: a.generation, previewGeneration: a.previewGeneration,
					previewID: a.current().ID, preview: &preview, err: previewErr,
				})
				if !visible {
					a.key(context.Background(), event.Key{Code: 'v'})
				}
				var output bytes.Buffer
				a.browser = browser.New(strconv.Quote(executable)+" -test.run=^TestPreviewBrowserStub$ --", &output, &output)
				a.key(context.Background(), event.Key{Code: 'o'})
				if a.err != nil || output.String() != want {
					t.Fatalf("browser target=%q error=%v, want %q", output.String(), a.err, want)
				}
			})
		}
	}
}

func TestFetchRemovesStalePreview(t *testing.T) {
	for _, final := range []bool{false, true} {
		t.Run(fmt.Sprintf("final=%t", final), func(t *testing.T) {
			a := testApp()
			a.showPreview, a.previewID, a.previewGeneration = true, "1", 4
			old := &notifs.Preview{Title: "Old preview", Body: "Old notification body"}
			a.preview = old
			a.previewOffset, a.metadataOffset = 3, 2
			a.discussions[0].pages = []notifs.DiscussionPage{{Total: 1}}
			previewCanceled, discussionCanceled := false, false
			a.cancelPreview = func() { previewCanceled = true }
			a.discussions[0].cancel = func() { discussionCanceled = true }
			result := notifs.Result{}
			if !final {
				result.Items = []notifs.Thread{{ID: "3"}, {ID: "4"}}
			} else {
				a.previewTab = 1
			}
			a.receive(context.Background(), message{generation: a.generation, result: &result, final: final})
			if a.preview != nil || a.previewID != "" || len(a.discussions[0].pages) != 0 {
				t.Errorf("refresh retained stale preview: id=%q preview=%+v", a.previewID, a.preview)
			}
			if !previewCanceled || !discussionCanceled || a.previewOffset != 0 || a.metadataOffset != 0 {
				t.Errorf("refresh retained preview work or offsets")
			}
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 40})
			a.paint(sc)
			out := text.String(sc, color.Disabled)
			if strings.Contains(out, "Old notification body") || (final && strings.Contains(out, "Fetching")) {
				t.Errorf("stale or unstarted preview remains on screen:\n%s", out)
			}
			a.result.Items = []notifs.Thread{{ID: "1"}}
			a.cursor = 0
			a.receive(context.Background(), message{
				generation: a.generation, previewGeneration: 4, previewID: "1", preview: old,
			})
			if a.preview != nil {
				t.Error("invalidated preview response was accepted after returning to its item")
			}
		})
	}
}

func TestIntermediateFetchPreservesCurrentPreview(t *testing.T) {
	a := testApp()
	a.showPreview, a.previewID = true, "1"
	a.preview = &notifs.Preview{Title: "Current preview"}
	a.discussions[0].pages = []notifs.DiscussionPage{{Total: 1}}
	a.previewOffset, a.metadataOffset = 3, 2
	result := notifs.Result{Items: []notifs.Thread{a.result.Items[1], a.result.Items[0]}}
	a.receive(context.Background(), message{generation: a.generation, result: &result})
	if a.preview == nil || a.preview.Title != "Current preview" || a.current().ID != "1" ||
		a.previewOffset != 3 || a.metadataOffset != 2 || len(a.discussions[0].pages) != 1 {
		t.Fatal("reordering the same current item discarded its preview state")
	}
}
