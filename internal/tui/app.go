package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/program"
	"github.com/aymanbagabas/gh-notifs/internal/config"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
	"github.com/cli/go-gh/v2/pkg/browser"
)

type Options struct {
	Query    notifs.Query
	PageSize int
	NoColor  bool
	Scopes   []config.Scope
}

type message struct {
	generation           int
	result               *notifs.Result
	final                bool
	preview              *notifs.Preview
	previewID            string
	previewGeneration    int
	discussionTab        int
	discussionGeneration int
	discussion           *notifs.DiscussionPage
	action               string
	ids                  []string
	err                  error
}

type app struct {
	client            *notifs.Client
	program           *program.Program
	browser           *browser.Browser
	options           Options
	query             notifs.Query
	result            notifs.Result
	repositories      []notifs.Repository
	cursor            int
	page              int
	scroll            int
	selected          map[string]bool
	edit              bool
	input             []rune
	caret             int
	cutBuffer         string
	showCursor        bool
	title             string
	paste             strings.Builder
	pasting           bool
	light             bool
	markdown          previewRender
	metadataMarkdown  previewRender
	help              bool
	helpOffset        int
	sidebar           bool
	sideIndex         int
	preview           *notifs.Preview
	previewID         string
	previewOffset     int
	previewTab        int
	discussions       [2]discussionState
	metadataOffset    int
	metadataWidth     int
	metadataHidden    bool
	metadataFocused   bool
	previewNarrow     bool
	previewBounds     layout.Rectangle
	showPreview       bool
	status            string
	err               error
	loading           bool
	acting            bool
	pendingAction     string
	pendingIDs        []string
	dialogPosition    layout.Position
	dialogRect        layout.Rectangle
	dialogMoved       bool
	dialogConfirm     bool
	dialogHover       int
	generation        int
	cancelFetch       context.CancelFunc
	cancelPreview     context.CancelFunc
	previewGeneration int
	messages          chan message
	workers           sync.WaitGroup
	hits              []hit
	sideWidth         int
	sideHidden        bool
	dragging          string
	dragOffset        layout.Position
	hoverID           string
	hoverSelect       bool
	completionKey     string
	completionIndex   int
}

func Run(ctx context.Context, client *notifs.Client, options Options) (err error) {
	p, err := program.Stdio()
	if err != nil {
		return err
	}
	defer func() {
		if value := recover(); value != nil {
			err = errors.Join(err, fmt.Errorf("inbox panic: %v\n%s", value, debug.Stack()))
		}
		err = errors.Join(err, p.Finish())
	}()
	programOptions := program.DefaultOptions()
	programOptions.QueryBackgroundColor = !options.NoColor
	if err := p.InitWith(programOptions); err != nil {
		return err
	}
	if err := p.EnterAltScreen(); err != nil {
		return err
	}
	if err := p.HideCursor(); err != nil {
		return err
	}
	if err := p.EnableFocusEvents(); err != nil {
		return err
	}
	if err := p.EnableMouse(program.MouseMotion); err != nil {
		return err
	}
	if !options.NoColor {
		if err := p.EnableColorSchemeUpdates(); err != nil {
			return err
		}
	}
	if err := p.QueryCapabilities(nil); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	a := &app{client: client, program: p, options: options, query: options.Query,
		selected: make(map[string]bool), messages: make(chan message, 8),
		browser: browser.New("", io.Discard, io.Discard)}
	defer func() {
		cancel()
		a.workers.Wait()
	}()
	a.result = client.Cached(a.query, options.PageSize)
	a.repositories = client.RecentRepositories()
	a.fetch(ctx, false)
	events := p.Events(ctx)
	for {
		if err := a.render(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			switch ev := ev.(type) {
			case error:
				return ev
			case event.KeyPressEvent:
				if a.key(ctx, event.Key(ev)) {
					return nil
				}
			case event.KeyRepeatEvent:
				if a.key(ctx, event.Key(ev)) {
					return nil
				}
			case event.ResizeEvent:
				if err := p.Autoresize(); err != nil {
					return err
				}
			case event.PasteStartEvent, event.PasteChunkEvent, event.PasteEndEvent:
				a.handlePaste(ev)
			case event.MouseClickEvent, event.MouseMoveEvent, event.MouseReleaseEvent, event.MouseWheelEvent:
				if a.mouse(ctx, ev, p.Screen()) {
					return nil
				}
			case event.ColorSchemeEvent:
				a.light = event.ColorScheme(ev) == event.ColorSchemeLight
				if !a.options.NoColor {
					if err := p.RequestBackgroundColor(); err != nil {
						return err
					}
				}
			case event.BackgroundColorEvent:
				a.light = theme.IsLight(ev.Color)
			case event.FocusOutEvent:
				a.hoverID, a.hoverSelect = "", false
				a.dialogHover = 0
			}
		case msg := <-a.messages:
			a.receive(ctx, msg)
		}
	}
}

func (a *app) send(ctx context.Context, msg message) {
	select {
	case a.messages <- msg:
	case <-ctx.Done():
	}
}

func (a *app) fetch(ctx context.Context, changed bool) {
	if a.acting {
		return
	}
	a.resetDiscussions()
	if a.cancelFetch != nil {
		a.cancelFetch()
	}
	if a.cancelPreview != nil {
		a.cancelPreview()
	}
	a.generation++
	generation := a.generation
	job, cancel := context.WithCancel(ctx)
	a.cancelFetch = cancel
	a.loading, a.err = true, nil
	if changed {
		a.page, a.cursor, a.scroll = 0, 0, 0
		a.selected = make(map[string]bool)
		a.preview, a.previewID = nil, ""
		a.result = a.client.Cached(a.query, a.options.PageSize)
	}
	query, limit := a.query, (a.page+1)*a.options.PageSize
	a.workers.Go(func() {
		result, err := a.client.Fetch(job, query, limit, func(result notifs.Result) {
			a.send(job, message{generation: generation, result: &result})
		})
		if err == nil {
			err = a.client.Enrich(job, result.Items)
		}
		a.send(job, message{generation: generation, result: &result, final: true, err: err})
	})
}

func (a *app) receive(ctx context.Context, msg message) {
	if msg.action != "" {
		a.acting = false
		if msg.err != nil {
			a.err = msg.err
		} else {
			a.status = fmt.Sprintf("%s: %d notification(s)", msg.action, len(msg.ids))
			a.selected = make(map[string]bool)
		}
		a.fetch(ctx, false)
		if msg.err != nil {
			a.err = msg.err
		}
		return
	}
	if msg.generation != a.generation {
		return
	}
	if msg.previewID != "" {
		if !a.showPreview || msg.previewGeneration != a.previewGeneration {
			return
		}
		if current := a.current(); current != nil && current.ID == msg.previewID {
			if msg.discussionTab > 0 {
				state := &a.discussions[msg.discussionTab-1]
				if msg.discussionGeneration != state.generation {
					return
				}
				state.loading, state.err = false, msg.err
				if msg.err == nil && msg.discussion != nil {
					if state.page == len(state.pages) {
						state.pages = append(state.pages, *msg.discussion)
					} else {
						state.pages[state.page] = *msg.discussion
					}
				}
				return
			}
			a.preview, a.previewID, a.err = msg.preview, msg.previewID, msg.err
		}
		return
	}
	if msg.result != nil {
		a.repositories = a.client.RecentRepositories()
		var id string
		if t := a.current(); t != nil {
			id = t.ID
		}
		// Keep the previous verified list while an incomplete refresh is shorter.
		if msg.final || (len(msg.result.Items) >= len(a.result.Items) && len(msg.result.Items) > a.page*a.options.PageSize) {
			if msg.err == nil || len(msg.result.Items) > 0 {
				a.result = *msg.result
				for i, item := range a.result.Items {
					if item.ID == id {
						a.cursor = i
						a.page = i / a.options.PageSize
						break
					}
				}
				a.clamp()
				if current := a.current(); current == nil || current.ID != id {
					a.resetPreview()
				}
			}
		}
	}
	if msg.final {
		a.clamp()
		a.loading = false
		if !a.edit || msg.err != nil {
			a.err = msg.err
		}
		if a.showPreview {
			a.loadPreview(ctx)
		}
	}
}

func (a *app) current() *notifs.Thread {
	if a.cursor < 0 || a.cursor >= len(a.result.Items) {
		return nil
	}
	return &a.result.Items[a.cursor]
}

func (a *app) clamp() {
	a.cursor = max(0, min(a.cursor, len(a.result.Items)-1))
	a.page = min(a.page, max(0, (len(a.result.Items)-1)/a.options.PageSize))
}

func (a *app) move(ctx context.Context, delta int) {
	previous := a.cursor
	start := a.page * a.options.PageSize
	end := min(start+a.options.PageSize, len(a.result.Items))
	a.cursor = max(start, min(a.cursor+delta, max(start, end-1)))
	if a.showPreview && a.cursor != previous {
		a.loadPreview(ctx)
	}
}

func (a *app) resetPreview() {
	a.resetDiscussions()
	if a.cancelPreview != nil {
		a.cancelPreview()
	}
	a.preview, a.previewID, a.previewOffset, a.metadataOffset = nil, "", 0, 0
	a.previewGeneration++
}

func (a *app) loadPreview(ctx context.Context) {
	a.resetPreview()
	current := a.current()
	if current == nil {
		a.previewTab = 0
		return
	}
	a.previewID = current.ID
	if !supportsDiscussion(current, a.previewTab) {
		a.previewTab = 0
	}
	previewGeneration := a.previewGeneration
	item, generation := *current, a.generation
	job, cancel := context.WithCancel(ctx)
	a.cancelPreview = cancel
	a.workers.Go(func() {
		timer := time.NewTimer(previewDebounce)
		defer timer.Stop()
		select {
		case <-job.Done():
			return
		case <-timer.C:
		}
		preview, err := a.client.Preview(job, item)
		a.send(job, message{generation: generation, preview: &preview, previewID: item.ID, previewGeneration: previewGeneration, err: err})
	})
	if a.previewTab > 0 {
		a.loadDiscussion(ctx, a.previewTab)
	}
}

func (a *app) actionIDs() []string {
	var ids []string
	for _, item := range a.result.Items {
		if a.selected[item.ID] {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		if item := a.current(); item != nil {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func (a *app) act(ctx context.Context, action string, ids []string) {
	if len(ids) == 0 || a.acting {
		return
	}
	if a.cancelFetch != nil {
		a.cancelFetch()
	}
	a.generation++
	a.loading, a.acting = false, true
	a.workers.Go(func() {
		var result error
		for _, id := range ids {
			if err := a.client.Action(ctx, id, action); err != nil {
				result = errors.Join(result, fmt.Errorf("thread %s: %w", id, err))
				break
			}
		}
		a.send(ctx, message{action: action, ids: ids, err: result})
	})
}

func (a *app) key(ctx context.Context, key event.Key) bool {
	if key.Matches("ctrl+c") {
		return true
	}
	if a.edit {
		return a.editKey(ctx, key)
	}
	if a.pendingAction != "" {
		a.dialogKey(ctx, key)
		return false
	}
	if a.help {
		switch {
		case key.MatchesAny("?", "esc"):
			a.help = false
			return false
		case key.MatchesAny("down", "pagedown"):
			a.helpOffset += 3
			return false
		case key.MatchesAny("up", "pageup"):
			a.helpOffset = max(0, a.helpOffset-3)
			return false
		}
	}
	switch {
	case key.Matches("q"):
		return true
	case key.Matches("?"):
		a.help, a.helpOffset = true, 0
	case key.MatchesAny("/", "s"):
		a.edit, a.input = true, []rune(a.query.Raw)
		a.caret = len(a.input)
		a.completionKey = ""
	case key.Matches("i"):
		a.apply(ctx, a.options.Query.Raw)
	case key.Matches("tab"):
		a.sidebar = !a.sidebar
	case key.Matches("b"):
		a.sideHidden = !a.sideHidden
		if a.sideHidden {
			a.sidebar = false
		}
	case key.MatchesAny("j", "down"):
		if a.sidebar {
			a.sideIndex = max(0, min(a.sideIndex+1, len(a.sideEntries())-1))
		} else {
			a.move(ctx, 1)
		}
	case key.MatchesAny("k", "up"):
		if a.sidebar {
			a.sideIndex = max(0, a.sideIndex-1)
		} else {
			a.move(ctx, -1)
		}
	case key.MatchesAny("g", "home"):
		if a.sidebar {
			a.sideIndex = 0
		} else {
			a.move(ctx, -len(a.result.Items))
		}
	case key.MatchesAny("G", "shift+g", "end"):
		if a.sidebar {
			a.sideIndex = max(0, len(a.sideEntries())-1)
		} else {
			a.move(ctx, len(a.result.Items))
		}
	case key.MatchesAny("pagedown", "]"):
		if !a.acting && !a.loading && a.hasNextPage() {
			a.page++
			a.cursor, a.scroll = a.page*a.options.PageSize, 0
			if a.cursor >= len(a.result.Items) {
				a.preview, a.previewID = nil, ""
				a.fetch(ctx, false)
			} else if a.showPreview {
				a.loadPreview(ctx)
			}
		}
	case key.MatchesAny("pageup", "["):
		if a.page > 0 {
			a.page--
			a.cursor, a.scroll = a.page*a.options.PageSize, 0
			if a.showPreview {
				a.loadPreview(ctx)
			}
		}
	case key.Matches("ctrl+d"):
		if a.showPreview {
			a.scrollPreview(5)
		} else {
			a.move(ctx, 10)
		}
	case key.Matches("ctrl+u"):
		if a.showPreview {
			a.scrollPreview(-5)
		} else {
			a.move(ctx, -10)
		}
	case key.Matches("x"):
		if item := a.current(); item != nil {
			a.selected[item.ID] = !a.selected[item.ID]
		}
	case key.MatchesAny("shift+a", "A"):
		for _, item := range a.result.Items {
			a.selected[item.ID] = true
		}
	case key.MatchesAny("shift+x", "X"):
		clear(a.selected)
	case key.MatchesAny("shift+i", "I"):
		a.act(ctx, "read", a.actionIDs())
	case key.MatchesAny("shift+u", "U"):
		a.status = "Mark unread is not available through the supported public API."
	case key.Matches("e"):
		a.confirmAction("done")
	case key.MatchesAny("shift+m", "M"):
		a.confirmAction("unsubscribe")
	case key.MatchesAny("shift+s", "S"):
		a.confirmAction("subscribe")
	case key.Matches("m"):
		if a.showPreview {
			a.toggleMetadata()
		}
	case key.Matches("1"):
		a.selectPreviewTab(ctx, 0)
	case key.Matches("2"):
		a.selectPreviewTab(ctx, 1)
	case key.Matches("3"):
		a.selectPreviewTab(ctx, 2)
	case key.Matches(","):
		a.changeDiscussionPage(ctx, -1)
	case key.Matches("."):
		a.changeDiscussionPage(ctx, 1)
	case key.Matches("alt+left"):
		a.metadataFocused = false
	case key.Matches("alt+right"):
		if a.showPreview {
			a.metadataHidden, a.metadataFocused = false, true
		}
	case key.Matches("r"):
		if a.loading {
			a.status = "A refresh is already in progress."
		} else {
			a.status = ""
			a.fetch(ctx, false)
		}
	case key.Matches("v"):
		a.showPreview = !a.showPreview
		if a.showPreview {
			a.loadPreview(ctx)
		} else {
			a.resetDiscussions()
			if a.cancelPreview != nil {
				a.cancelPreview()
			}
		}
	case key.MatchesAny("enter", "o"):
		if a.sidebar {
			entries := a.sideEntries()
			if a.sideIndex >= len(entries) {
				return false
			}
			a.apply(ctx, entries[a.sideIndex].query)
			a.sidebar = false
		} else if current := a.current(); current != nil {
			target := current.URL
			if a.preview != nil && a.previewID == current.ID && a.preview.URL != "" {
				target = a.preview.URL
			}
			a.err = OpenBrowser(a.browser, target)
		}
	}
	return false
}

func (a *app) hasNextPage() bool {
	return (a.page+1)*a.options.PageSize < len(a.result.Items) || a.result.More
}

func (a *app) insert(s string) {
	runes := []rune(s)
	tail := append([]rune(nil), a.input[a.caret:]...)
	a.input = append(append(a.input[:a.caret], runes...), tail...)
	a.caret += len(runes)
}

func (a *app) handlePaste(ev event.Event) {
	switch ev := ev.(type) {
	case event.PasteStartEvent:
		a.paste.Reset()
		a.pasting = a.edit
	case event.PasteChunkEvent:
		if a.pasting {
			a.paste.Write([]byte(ev))
		}
	case event.PasteEndEvent:
		if a.pasting && a.edit {
			a.insert(notifs.Plain(a.paste.String()))
		}
		a.paste.Reset()
		a.pasting = false
	}
}

func (a *app) editKey(ctx context.Context, key event.Key) bool {
	suggestions, start, end := a.suggestions()
	if key.MatchesAny("ctrl+space", "ctrl+@") {
		a.completionIndex = 0
		return false
	}
	if len(suggestions) > 0 {
		switch {
		case key.Matches("down"):
			a.completionIndex = (a.completionIndex + 1) % len(suggestions)
			return false
		case key.Matches("up"):
			a.completionIndex = (max(0, a.completionIndex) + len(suggestions) - 1) % len(suggestions)
			return false
		case key.Matches("tab") || (key.Matches("enter") && a.completionIndex >= 0):
			a.complete(suggestions[max(0, a.completionIndex)], start, end)
			return false
		}
	}
	switch {
	case key.Matches("esc"):
		a.edit = false
		a.pasting = false
		a.paste.Reset()
	case key.Matches("enter"):
		if a.apply(ctx, string(a.input)) {
			a.edit = false
			a.pasting = false
			a.paste.Reset()
		}
	case key.MatchesAny("left", "ctrl+b"):
		a.caret = max(0, a.caret-1)
	case key.MatchesAny("right", "ctrl+f"):
		a.caret = min(len(a.input), a.caret+1)
	case key.MatchesAny("alt+b", "ctrl+left", "alt+left"):
		a.caret = a.wordBoundary(-1, false)
	case key.MatchesAny("alt+f", "ctrl+right", "alt+right"):
		a.caret = a.wordBoundary(1, false)
	case key.MatchesAny("home", "ctrl+a"):
		a.caret = 0
	case key.MatchesAny("end", "ctrl+e"):
		a.caret = len(a.input)
	case key.MatchesAny("backspace", "ctrl+h"):
		if a.caret > 0 {
			a.input = append(a.input[:a.caret-1], a.input[a.caret:]...)
			a.caret--
		}
	case key.MatchesAny("delete", "ctrl+d"):
		if a.caret < len(a.input) {
			a.input = append(a.input[:a.caret], a.input[a.caret+1:]...)
		}
	case key.Matches("ctrl+u"):
		a.cutQuery(0, a.caret)
	case key.Matches("ctrl+k"):
		a.cutQuery(a.caret, len(a.input))
	case key.Matches("ctrl+w"):
		a.cutQuery(a.wordBoundary(-1, true), a.caret)
	case key.MatchesAny("alt+backspace", "ctrl+backspace"):
		a.cutQuery(a.wordBoundary(-1, false), a.caret)
	case key.MatchesAny("alt+d", "ctrl+delete"):
		a.cutQuery(a.caret, a.wordBoundary(1, false))
	case key.Matches("ctrl+y"):
		a.insert(a.cutBuffer)
	case key.Mod&^(event.KeyModShift|event.KeyModCapsLock|event.KeyModNumLock) == 0 && unicode.IsPrint(key.Code):
		value := key.Text
		if value == "" {
			r := key.Code
			if key.Mod&event.KeyModShift != 0 {
				r = unicode.ToUpper(r)
			}
			value = string(r)
		}
		a.insert(value)
	}
	return false
}

func (a *app) cutQuery(start, end int) {
	if start == end {
		return
	}
	a.cutBuffer = string(a.input[start:end])
	a.input = append(a.input[:start], a.input[end:]...)
	a.caret = start
}

func (a *app) wordBoundary(direction int, whitespaceOnly bool) int {
	word := func(r rune) bool {
		if whitespaceOnly {
			return !unicode.IsSpace(r)
		}
		return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
	}
	pos := a.caret
	if direction < 0 {
		for pos > 0 && !word(a.input[pos-1]) {
			pos--
		}
		for pos > 0 && word(a.input[pos-1]) {
			pos--
		}
	} else {
		for pos < len(a.input) && !word(a.input[pos]) {
			pos++
		}
		for pos < len(a.input) && word(a.input[pos]) {
			pos++
		}
	}
	return pos
}

func (a *app) apply(ctx context.Context, raw string) bool {
	if a.acting {
		a.status = "Wait for the current action before changing the query."
		return false
	}
	query, err := notifs.ParseQuery(raw)
	if err != nil {
		a.err = err
		return false
	}
	a.query = query
	a.status = ""
	a.fetch(ctx, true)
	return true
}

type sideEntry struct {
	label, query string
	repository   bool
}

func (a *app) sideEntries() []sideEntry {
	var entries []sideEntry
	for _, scope := range a.options.Scopes {
		entries = append(entries, sideEntry{label: scope.Name, query: scope.Query})
	}
	for _, repo := range a.repositories[:min(25, len(a.repositories))] {
		entries = append(entries, sideEntry{label: repo.Name, query: "repo:" + repo.Name, repository: true})
	}
	return entries
}
