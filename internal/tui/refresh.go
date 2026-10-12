package tui

import (
	"context"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func (a *app) refreshIfDue(ctx context.Context, now time.Time) {
	if !a.autoRefresh || a.loading || a.acting || a.edit || a.pasting ||
		a.pendingAction != "" || a.nextRefresh.IsZero() || now.Before(a.refreshDeadline()) {
		return
	}
	a.startFetch(ctx, false, true)
}

func (a *app) refreshDeadline() time.Time {
	deadline := a.nextRefresh
	if !deadline.IsZero() {
		if allowed := a.client.PollAfter(); allowed.After(deadline) {
			deadline = allowed
		}
	}
	return deadline
}

func (a *app) observeActivity(items []notifs.Thread) bool {
	baseline := a.activity == nil
	if baseline {
		a.activity = make(map[string]time.Time)
	}
	arrived := false
	latest := a.latestActivity
	for _, item := range items {
		previous, seen := a.activity[item.ID]
		if !baseline && item.Unread &&
			(seen && item.UpdatedAt.After(previous) || !seen && !item.UpdatedAt.Before(a.latestActivity)) {
			arrived = true
		}
		if !seen || item.UpdatedAt.After(previous) {
			a.activity[item.ID] = item.UpdatedAt
		}
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	a.latestActivity = latest
	return arrived
}
