package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/config"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	arrivals := flag.Bool("arrivals", false, "enable automatic refresh and a bell for a fictional arrival after one minute")
	flag.Parse()
	root, err := os.MkdirTemp("", "gh-notifs-inbox-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	cache, err := notifs.OpenCache(root, "github.com", "demo")
	if err != nil {
		return err
	}
	items := demoTransport{
		{Repository: "acme/api", Number: 42, Type: "PullRequest", Title: "Keep uploads responsive on slow connections", Author: "octocat", State: "open", Reason: "review_requested", Unread: true},
		{Repository: "acme/web", Number: 81, Type: "Issue", Title: "Fix keyboard navigation in the search dialog", Author: "mona", State: "open", Reason: "mention", Unread: true},
		{Repository: "acme/web", Number: 28, Type: "PullRequest", Title: "Add dark mode to the dashboard", Author: "sam", State: "merged", Reason: "subscribed"},
		{Repository: "acme/docs", Number: 17, Type: "PullRequest", Title: "Document the command-line workflow", Author: "alex", State: "open", Reason: "review_requested", Unread: true},
		{Repository: "acme/api", Number: 64, Type: "Issue", Title: "Handle empty search results", Author: "octocat", State: "open", Reason: "assign"},
		{Repository: "acme/docs", Number: 33, Type: "PullRequest", Title: "Publish the quick-start guide", Author: "mona", State: "closed", Reason: "state_change"},
	}
	for i := range items {
		item := &items[i]
		item.ID = fmt.Sprint(i + 1)
		item.AuthorID = "U_demo_" + item.Author
		item.UpdatedAt = time.Now().Add(-time.Duration(i*i+1) * time.Minute)
		endpoint, web := "pulls", "pull"
		if item.Type == "Issue" {
			endpoint, web = "issues", "issues"
		}
		item.SubjectURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/%d", item.Repository, endpoint, item.Number)
		item.URL = fmt.Sprintf("https://github.com/%s/%s/%d", item.Repository, web, item.Number)
	}
	cache.Put(notifs.Endpoint(notifs.Query{}, 1), notifs.Page{Items: items, CheckedAt: time.Now()})
	var transport http.RoundTripper = items
	if *arrivals {
		transport = arrivalTransport{items: items, arrival: time.Now().Add(time.Minute)}
	}
	client := &notifs.Client{HTTP: &http.Client{Transport: transport}, Host: "github.com",
		BaseURL: "https://api.github.com/", GraphQL: "https://api.github.com/graphql", Cache: cache}
	return tui.Run(context.Background(), client, tui.Options{PageSize: 6, AutoRefresh: *arrivals, Bell: *arrivals, Scopes: []config.Scope{
		{Name: "Inbox"}, {Name: "Unread", Query: "is:unread"},
		{Name: "Review requested", Query: "reason:review-requested"}, {Name: "Mentioned", Query: "reason:mention"},
	}})
}

type arrivalTransport struct {
	items   demoTransport
	arrival time.Time
}

func (t arrivalTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	items := t.items
	if !time.Now().Before(t.arrival) {
		item := items[0]
		item.ID, item.Number = "7", 65
		item.Title = "New arrival: review the notification workflow"
		item.UpdatedAt = t.arrival
		item.SubjectURL = "https://api.github.com/repos/acme/api/pulls/65"
		item.URL = "https://github.com/acme/api/pull/65"
		items = append(append(demoTransport(nil), items...), item)
	}
	return items.RoundTrip(r)
}

// All API responses are local fixtures. No credentials or network access are used.
type demoTransport []notifs.Thread

func (items demoTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var data any
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/notifications"):
		rows := make([]any, 0, len(items))
		for _, item := range items {
			if strings.HasPrefix(r.URL.Path, "/repos/") && r.URL.Path != "/repos/"+item.Repository+"/notifications" {
				continue
			}
			if r.URL.Query().Get("all") == "false" && !item.Unread {
				continue
			}
			rows = append(rows, map[string]any{
				"id": item.ID, "unread": item.Unread, "reason": item.Reason, "updated_at": item.UpdatedAt,
				"repository": map[string]any{"full_name": item.Repository},
				"subject":    map[string]any{"type": item.Type, "title": item.Title, "url": item.SubjectURL},
			})
		}
		data = rows
	case r.Method == http.MethodPost && r.URL.Path == "/graphql":
		var payload struct {
			Variables map[string]json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, err
		}
		nodes := make(map[string]any)
		for _, item := range items {
			for alias, raw := range payload.Variables {
				if !strings.HasPrefix(alias, "n") {
					continue
				}
				var number int
				if err := json.Unmarshal(raw, &number); err != nil {
					return nil, err
				}
				if number != item.Number {
					continue
				}
				detail := map[string]any{"state": strings.ToUpper(item.State), "url": item.URL,
					"author": map[string]any{"login": item.Author, "id": item.AuthorID}}
				key := "v" + strings.TrimPrefix(alias, "n")
				if alias == "number" {
					key = "repository"
					detail["headRefName"], detail["baseRefName"] = "fix/uploads", "main"
					detail["reviewDecision"] = "REVIEW_REQUIRED"
					detail["labels"] = map[string]any{"totalCount": 1, "nodes": []any{map[string]any{"name": "enhancement"}}}
					for _, field := range []string{"assignees", "participants", "reviewRequests", "latestReviews", "development"} {
						detail[field] = map[string]any{"totalCount": 0, "nodes": []any{}}
					}
				}
				nodes[key] = map[string]any{"issueOrPullRequest": detail}
			}
		}
		data = map[string]any{"data": nodes}
	case r.Method == http.MethodGet:
		for _, item := range items {
			if r.URL.String() == item.SubjectURL {
				data = map[string]any{"title": item.Title, "html_url": item.URL,
					"body": "## What changed\n\nKeep the interface responsive while requests run in the background.\n\n- [x] Preserve keyboard navigation\n- [x] Cover slow connections\n- [ ] Ready for review\n\n```go\nresult, err := client.Fetch(ctx, query, limit, progress)\n```"}
				break
			}
		}
	}
	if data == nil {
		return nil, fmt.Errorf("the offline demo does not support %s %s", r.Method, r.URL.Path)
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
}
