package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

const previewDebounce = 200 * time.Millisecond

type discussionState struct {
	pages      []notifs.DiscussionPage
	page       int
	offset     int
	loading    bool
	err        error
	generation int
	cancel     context.CancelFunc
	markdown   previewRender
}

func supportsDiscussion(item *notifs.Thread, tab int) bool {
	return tab == 0 || (item != nil && item.Number > 0 &&
		(item.Type == "Issue" || item.Type == "PullRequest") && (tab == 1 || item.Type == "PullRequest"))
}

func (a *app) resetDiscussions() {
	for i := range a.discussions {
		if cancel := a.discussions[i].cancel; cancel != nil {
			cancel()
		}
		a.discussions[i] = discussionState{}
	}
}

func (a *app) selectPreviewTab(ctx context.Context, tab int) {
	if !a.showPreview {
		return
	}
	if !supportsDiscussion(a.current(), tab) {
		a.status = "This notification does not support that discussion view."
		return
	}
	if a.previewTab > 0 && a.previewTab != tab {
		state := &a.discussions[a.previewTab-1]
		if state.cancel != nil {
			state.cancel()
		}
		state.loading = false
		state.generation++
	}
	a.previewTab, a.metadataFocused = tab, false
	if tab > 0 {
		state := &a.discussions[tab-1]
		if !state.loading && state.page >= len(state.pages) {
			a.loadDiscussion(ctx, tab)
		}
	}
}

func (a *app) loadDiscussion(ctx context.Context, tab int) {
	state := &a.discussions[tab-1]
	current := a.current()
	if current == nil || current.Number < 1 || (current.Type != "Issue" && current.Type != "PullRequest") ||
		(tab == 2 && current.Type != "PullRequest") {
		state.err = fmt.Errorf("this notification has no supported discussion endpoint")
		return
	}
	if state.cancel != nil {
		state.cancel()
	}
	state.generation++
	state.loading, state.err = true, nil
	cursor := ""
	if state.page > 0 {
		cursor = state.pages[state.page-1].NextCursor
	}
	item, generation, previewGeneration, requestGeneration := *current, a.generation, a.previewGeneration, state.generation
	job, cancel := context.WithCancel(ctx)
	state.cancel = cancel
	a.workers.Go(func() {
		timer := time.NewTimer(previewDebounce)
		defer timer.Stop()
		select {
		case <-job.Done():
			return
		case <-timer.C:
		}
		page, err := a.client.Discussion(job, item, tab == 2, cursor)
		a.send(job, message{generation: generation, previewGeneration: previewGeneration, previewID: item.ID,
			discussionTab: tab, discussionGeneration: requestGeneration, discussion: &page, err: err})
	})
}

func (a *app) changeDiscussionPage(ctx context.Context, delta int) {
	if !a.showPreview || a.previewTab == 0 {
		return
	}
	state := &a.discussions[a.previewTab-1]
	if delta < 0 {
		if state.page == 0 {
			return
		}
		if state.cancel != nil {
			state.cancel()
		}
		state.loading = false
		state.generation++
		state.page--
		state.err = nil
	} else {
		if state.loading || state.page >= len(state.pages) || !state.pages[state.page].More {
			return
		}
		state.page++
	}
	state.offset, a.metadataFocused = 0, false
	if state.page >= len(state.pages) {
		a.loadDiscussion(ctx, a.previewTab)
	}
}

func (a *app) discussionLines(width int) ([]string, error) {
	state := &a.discussions[a.previewTab-1]
	if state.err != nil {
		return []string{"Error: " + notifs.Plain(state.err.Error()), "Press r to refresh and retry."}, nil
	}
	if state.page >= len(state.pages) {
		return []string{"Fetching discussion..."}, nil
	}
	return a.renderPreviewMarkdown(state.pages[state.page].Markdown(), width, &state.markdown)
}

func (a *app) contentOffset() *int {
	if a.previewTab > 0 {
		return &a.discussions[a.previewTab-1].offset
	}
	return &a.previewOffset
}
