package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"altscreen.io/vision/event"
	"altscreen.io/vision/program"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
	"github.com/cli/go-gh/v2/pkg/browser"
)

type Options struct {
	Query    notifs.Query
	PageSize int
	NoColor  bool
	Interval time.Duration
}

type message struct {
	generation int
	result     *notifs.Result
	final      bool
	preview    *notifs.Preview
	previewID  string
	action     string
	ids        []string
	err        error
}

type app struct {
	client        *notifs.Client
	program       *program.Program
	browser       *browser.Browser
	options       Options
	query         notifs.Query
	result        notifs.Result
	cursor        int
	page          int
	scroll        int
	selected      map[string]bool
	edit          bool
	input         []rune
	caret         int
	showCursor    bool
	paste         strings.Builder
	pasting       bool
	light         bool
	markdown      previewRender
	help          bool
	sidebar       bool
	sideIndex     int
	preview       *notifs.Preview
	previewID     string
	previewOffset int
	showPreview   bool
	status        string
	err           error
	loading       bool
	acting        bool
	pendingAction string
	generation    int
	cancelFetch   context.CancelFunc
	cancelPreview context.CancelFunc
	messages      chan message
	workers       sync.WaitGroup
	focused       bool
}

func Run(ctx context.Context, client *notifs.Client, options Options) (err error) {
	p, err := program.Stdio()
	if err != nil {
		return err
	}
	programOptions := program.DefaultOptions()
	programOptions.QueryBackgroundColor = !options.NoColor
	if err := p.InitWith(programOptions); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, p.Finish()) }()
	if err := p.EnterAltScreen(); err != nil {
		return err
	}
	if err := p.HideCursor(); err != nil {
		return err
	}
	if err := p.EnableFocusEvents(); err != nil {
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
		selected: make(map[string]bool), messages: make(chan message, 8), focused: true,
		browser: browser.New("", io.Discard, io.Discard)}
	defer func() {
		cancel()
		a.workers.Wait()
	}()
	a.result = client.Cached(a.query, options.PageSize)
	a.fetch(ctx, false)
	events := p.Events(ctx)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
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
			case event.ColorSchemeEvent:
				a.light = event.ColorScheme(ev) == event.ColorSchemeLight
				if !a.options.NoColor {
					if err := p.RequestBackgroundColor(); err != nil {
						return err
					}
				}
			case event.BackgroundColorEvent:
				a.light = theme.IsLight(ev.Color)
			case event.FocusInEvent:
				a.focused = true
			case event.FocusOutEvent:
				a.focused = false
			}
		case msg := <-a.messages:
			a.receive(ctx, msg)
		case <-ticker.C:
			if a.refreshDue(time.Now()) {
				a.fetch(ctx, false)
			}
		}
	}
}

func (a *app) refreshDue(now time.Time) bool {
	return a.options.Interval > 0 && a.focused && !a.loading && !a.acting && !now.Before(a.result.NextPoll)
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
		if current := a.current(); current != nil && current.ID == msg.previewID {
			a.preview, a.previewID, a.err = msg.preview, msg.previewID, msg.err
		}
		return
	}
	if msg.result != nil {
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
			}
		}
	}
	if msg.final {
		a.loading = false
		if !a.edit || msg.err != nil {
			a.err = msg.err
		}
		a.result.NextPoll = a.client.NextPoll()
		if next := time.Now().Add(a.options.Interval); next.After(a.result.NextPoll) {
			a.result.NextPoll = next
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
	start := a.page * a.options.PageSize
	end := min(start+a.options.PageSize, len(a.result.Items))
	a.cursor = max(start, min(a.cursor+delta, max(start, end-1)))
	if a.showPreview {
		a.loadPreview(ctx)
	}
}

func (a *app) loadPreview(ctx context.Context) {
	current := a.current()
	if current == nil {
		return
	}
	if a.cancelPreview != nil {
		a.cancelPreview()
	}
	a.preview, a.previewID, a.previewOffset = nil, current.ID, 0
	item, generation := *current, a.generation
	job, cancel := context.WithCancel(ctx)
	a.cancelPreview = cancel
	a.workers.Go(func() {
		preview, err := a.client.Preview(job, item)
		a.send(job, message{generation: generation, preview: &preview, previewID: item.ID, err: err})
	})
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

func (a *app) act(ctx context.Context, action string) {
	ids := a.actionIDs()
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
		action := a.pendingAction
		a.pendingAction = ""
		if key.Matches("y") {
			a.act(ctx, action)
		}
		return false
	}
	if a.help {
		a.help = false
		return false
	}
	switch {
	case key.MatchesAny("q", "esc"):
		return true
	case key.Matches("?"):
		a.help = true
	case key.MatchesAny("/", "s"):
		a.edit, a.input = true, []rune(a.query.Raw)
		a.caret = len(a.input)
	case key.Matches("i"):
		a.apply(ctx, a.options.Query.Raw)
	case key.Matches("tab"):
		a.sidebar = !a.sidebar
	case key.MatchesAny("j", "down"):
		if a.sidebar {
			a.sideIndex = min(a.sideIndex+1, len(a.sideEntries())-1)
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
			a.sideIndex = len(a.sideEntries()) - 1
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
			a.previewOffset += 5
		} else {
			a.move(ctx, 10)
		}
	case key.Matches("ctrl+u"):
		if a.showPreview {
			a.previewOffset = max(0, a.previewOffset-5)
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
	case key.MatchesAny("shift+i", "I"):
		a.act(ctx, "read")
	case key.MatchesAny("shift+u", "U"):
		a.status = "Mark unread is not available through the supported public API."
	case key.Matches("e"):
		a.pendingAction = "done"
	case key.MatchesAny("shift+m", "M"):
		a.pendingAction = "unsubscribe"
	case key.MatchesAny("shift+s", "S"):
		a.pendingAction = "subscribe"
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
		}
	case key.MatchesAny("enter", "o"):
		if a.sidebar {
			a.apply(ctx, a.sideEntries()[a.sideIndex].query)
			a.sidebar = false
		} else if current := a.current(); current != nil {
			target := current.URL
			if a.preview != nil && a.previewID == current.ID {
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
	case key.MatchesAny("home", "ctrl+a"):
		a.caret = 0
	case key.MatchesAny("end", "ctrl+e"):
		a.caret = len(a.input)
	case key.MatchesAny("backspace", "ctrl+h"):
		if a.caret > 0 {
			a.input = append(a.input[:a.caret-1], a.input[a.caret:]...)
			a.caret--
		}
	case key.Matches("delete"):
		if a.caret < len(a.input) {
			a.input = append(a.input[:a.caret], a.input[a.caret+1:]...)
		}
	case key.Matches("ctrl+u"):
		a.input = a.input[a.caret:]
		a.caret = 0
	case key.Mod&^(event.KeyModShift|event.KeyModCapsLock|event.KeyModNumLock) == 0 && unicode.IsPrint(key.Code):
		a.insert(string(key.Code))
	}
	return false
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

type sideEntry struct{ label, query string }

func (a *app) sideEntries() []sideEntry {
	var scope []string
	for _, t := range a.query.Terms {
		if t.Field == "repo" || t.Field == "org" {
			scope = append(scope, t.Field+":"+t.Value)
		}
	}
	base := strings.Join(scope, " ")
	entries := []sideEntry{
		{"Inbox", base},
		{"Unread", base + " is:unread"},
		{"Review requested", base + " reason:review-requested"},
		{"Mentioned", base + " reason:mention"},
		{"Team mentioned", base + " reason:team-mention"},
		{"Assigned", base + " reason:assign"},
		{"All repositories", ""},
	}
	seen := make(map[string]bool)
	for _, t := range a.result.Items {
		owner, _, _ := strings.Cut(t.Repository, "/")
		if !seen["org:"+owner] {
			entries = append(entries, sideEntry{"org:" + owner, "org:" + owner})
			seen["org:"+owner] = true
		}
		if !seen[t.Repository] {
			entries = append(entries, sideEntry{t.Repository, "repo:" + t.Repository})
			seen[t.Repository] = true
		}
	}
	return entries
}
