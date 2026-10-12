package tui

import (
	"context"
	"unicode/utf8"

	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
)

type hit struct {
	rect  layout.Rectangle
	kind  string
	index int
}

func (a *app) target(rect layout.Rectangle, kind string, index int) {
	a.hits = append(a.hits, hit{rect: rect, kind: kind, index: index})
}

func (a *app) mouse(ctx context.Context, ev event.Event, sc *screen.Screen) bool {
	m, ok := event.AsMouse(ev)
	if !ok {
		return false
	}
	if _, released := ev.(event.MouseReleaseEvent); released {
		a.dragging = ""
		return false
	}
	if _, moved := ev.(event.MouseMoveEvent); moved {
		a.hoverID, a.hoverSelect = "", false
		a.dialogHover = 0
		switch a.dragging {
		case "side":
			a.sideWidth = max(18, min(m.X+1, sc.Size().Width-30))
			return false
		case "metadata":
			if !a.previewNarrow {
				a.metadataWidth = max(24, min(sc.Size().Width-m.X-1, a.previewBounds.Dx()-38))
			}
			return false
		case "dialog":
			a.dialogPosition = layout.Pos(
				max(0, min(m.X-a.dragOffset.X, sc.Size().Width-a.dialogRect.Dx())),
				max(0, min(m.Y-a.dragOffset.Y, sc.Size().Height-a.dialogRect.Dy())),
			)
			return false
		}
		if a.pendingAction != "" {
			for i := len(a.hits) - 1; i >= 0; i-- {
				h := a.hits[i]
				if layout.Pos(m.X, m.Y).In(h.rect) {
					if h.kind == "confirm" || h.kind == "cancel" {
						a.dialogHover = h.index
					}
					break
				}
			}
		} else {
			for i := len(a.hits) - 1; i >= 0; i-- {
				h := a.hits[i]
				if !layout.Pos(m.X, m.Y).In(h.rect) {
					continue
				}
				if (h.kind == "item" || h.kind == "checkbox") && h.index < len(a.result.Items) {
					a.hoverID = a.result.Items[h.index].ID
					a.hoverSelect = h.kind == "checkbox"
				}
				break
			}
		}
		return false
	}
	for i := len(a.hits) - 1; i >= 0; i-- {
		h := a.hits[i]
		if !layout.Pos(m.X, m.Y).In(h.rect) {
			continue
		}
		if a.pendingAction != "" && h.kind != "confirm" && h.kind != "cancel" && h.kind != "dialog-title" && h.kind != "dialog" {
			return false
		}
		if _, wheel := ev.(event.MouseWheelEvent); wheel {
			delta := 3
			if m.Button == event.MouseWheelUp {
				delta = -3
			} else if m.Button != event.MouseWheelDown {
				return false
			}
			if a.help {
				for _, area := range a.hits {
					if area.kind == "help" && layout.Pos(m.X, m.Y).In(area.rect) {
						a.helpOffset = max(0, a.helpOffset+delta)
						return false
					}
				}
			}
			switch h.kind {
			case "side", "scope":
				a.sidebar = true
				a.sideIndex = max(0, min(a.sideIndex+delta, len(a.sideEntries())-1))
			case "preview":
				a.metadataFocused = false
				a.scrollPreview(delta)
			case "metadata":
				a.metadataFocused = true
				a.metadataOffset = max(0, a.metadataOffset+delta)
			case "completion":
				values, _, _ := a.suggestions()
				a.completionIndex = max(0, min(a.completionIndex+delta, len(values)-1))
			case "item", "checkbox", "list":
				a.sidebar = false
				a.move(ctx, delta)
			}
			return false
		}
		if m.Button != event.MouseLeft {
			return false
		}
		switch h.kind {
		case "dismiss-error":
			a.dismissError()
		case "query":
			if !a.edit {
				a.edit, a.input = true, []rune(a.query.Raw)
				a.caret = len(a.input)
				a.completionKey = ""
			}
			visible, _ := queryViewport(sc, a.input, a.caret, sc.Size().Width-10)
			a.caret = len(a.input) - len([]rune(visible))
			column := 9
			for cluster, cells := range sc.GraphemeCells(visible) {
				if column+cells > m.X {
					break
				}
				column += cells
				a.caret += utf8.RuneCountInString(cluster)
			}
		case "completion":
			values, start, end := a.suggestions()
			if h.index < len(values) {
				a.complete(values[h.index], start, end)
			}
		case "divider":
			a.dragging = "side"
		case "metadata-divider":
			a.dragging = "metadata"
		case "metadata-toggle":
			a.toggleMetadata()
		case "preview":
			a.metadataFocused = false
		case "preview-body":
			a.selectPreviewTab(ctx, 0)
		case "preview-tab":
			a.selectPreviewTab(ctx, h.index)
		case "discussion-prev":
			a.changeDiscussionPage(ctx, -1)
		case "discussion-next":
			a.changeDiscussionPage(ctx, 1)
		case "metadata":
			a.metadataFocused = true
		case "dialog-title":
			a.dragging, a.dialogMoved = "dialog", true
			a.dragOffset = layout.Pos(m.X, m.Y).Sub(a.dialogRect.Min)
		case "dialog":
			return false
		case "toggle":
			a.sideHidden = !a.sideHidden
			if a.sideHidden {
				a.sidebar = false
			}
		case "scope":
			entries := a.sideEntries()
			if h.index < len(entries) {
				a.sideIndex, a.sidebar = h.index, true
				if a.apply(ctx, entries[h.index].query) {
					a.edit = false
				}
			}
		case "item", "checkbox":
			previous := a.cursor
			a.edit, a.sidebar, a.cursor = false, false, h.index
			if m.Mod&event.KeyModShift != 0 {
				for _, item := range a.result.Items[min(previous, h.index) : max(previous, h.index)+1] {
					a.selected[item.ID] = true
				}
			} else if h.kind == "checkbox" {
				item := a.result.Items[h.index]
				a.selected[item.ID] = !a.selected[item.ID]
			}
			if a.showPreview && a.cursor != previous {
				a.loadPreview(ctx)
			}
		case "help":
			return false
		case "confirm", "cancel":
			key := 'n'
			if h.kind == "confirm" {
				key = 'y'
			}
			return a.key(ctx, event.Key{Code: key})
		default:
			if h.kind != "side" && h.kind != "list" && h.kind != "preview" {
				a.edit = false
				key, err := event.ParseKey(h.kind)
				if err == nil {
					return a.key(ctx, key)
				}
			}
		}
		return false
	}
	return false
}
