package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func main() {
	for _, input := range []string{
		"",
		"org:acme",
		"reason:mention is:unread",
		"repo:cli/cli reason:comment is:unread",
		"reason:participating",
		"reason:subscribed",
		"reason:mention reason:comment is:read is:unread",
		"repo:cli/cli repo:cli/go-gh is:issue-or-pull-request is:discussion is:unread",
		"repo:cli/cli repo:cli/go-gh reason:participating reason:subscribed",
		"since:2026-10-02 since:2026-10-01 before:2026-10-09 before:2026-10-10",
	} {
		query, err := notifs.ParseQuery(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		requests := query.Requests()
		fmt.Printf("Query: %q\n%d request(s); multiple requests run concurrently:\n", input, len(requests))
		for _, request := range requests {
			fmt.Printf("GET %s\n", notifs.Endpoint(request, 1))
		}
		fmt.Println()
	}
	input := "repo:cli/cli repo:cli/go-gh reason:mention reason:comment is:issue-or-pull-request is:discussion is:unread"
	query, err := notifs.ParseQuery(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Local filter: %s\n", input)
	for _, item := range []notifs.Thread{
		{ID: "1", Repository: "cli/cli", Type: "Issue", Reason: "mention", Unread: true},
		{ID: "2", Repository: "cli/go-gh", Type: "PullRequest", Reason: "comment", Unread: true},
		{ID: "3", Repository: "cli/cli", Type: "Issue", Reason: "mention", Unread: false},
		{ID: "4", Repository: "cli/cli", Type: "Release", Reason: "mention", Unread: true},
		{ID: "5", Repository: "cli/cli", Type: "Discussion", Reason: "mention", Unread: true},
	} {
		fmt.Printf("Notification %s (%s, %s, unread=%t): match=%t\n",
			item.ID, item.Repository, item.Type, item.Unread, query.Match(item, ""))
	}
	for _, filter := range []string{"issue", "pr", "pull-request", "issue-or-pull-request"} {
		query, err := notifs.ParseQuery("is:" + filter)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("is:%s: issue=%t pull-request=%t release=%t\n", filter,
			query.Match(notifs.Thread{Type: "Issue"}, ""),
			query.Match(notifs.Thread{Type: "PullRequest"}, ""),
			query.Match(notifs.Thread{Type: "Release"}, ""))
	}
	if err := authorExample(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func authorExample() error {
	root, err := os.MkdirTemp("", "gh-notifs-authors-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	cache, err := notifs.OpenCache(root, "github.com", "example")
	if err != nil {
		return err
	}
	var lookups, details, requests atomic.Int32
	var renamed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/users/oldname", "/users/newname":
			lookups.Add(1)
			fmt.Fprint(w, `{"node_id":"U_example"}`)
		case "/notifications":
			fmt.Fprint(w, `[{"id":"1","updated_at":"2026-10-10T12:00:00Z","repository":{"full_name":"acme/app"},
"subject":{"type":"Issue","title":"Example","url":"https://api.github.com/repos/acme/app/issues/1"}}]`)
		case "/graphql":
			details.Add(1)
			name := "oldname"
			if renamed.Load() {
				name = "newname"
			}
			fmt.Fprintf(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_example","login":%q},"url":"https://github.com/acme/app/issues/1"}}}}`, name)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &notifs.Client{HTTP: server.Client(), BaseURL: server.URL + "/", GraphQL: server.URL + "/graphql",
		Host: "github.com", Cache: cache}
	fmt.Println("\nStable author identity (offline):")
	for _, input := range []string{"author:oldname", "author:newname", "author:newname"} {
		query, err := notifs.ParseQuery(input)
		if err != nil {
			return err
		}
		result, err := client.Fetch(context.Background(), query, 10, nil)
		if err != nil {
			return err
		}
		if len(result.Items) != 1 || result.Items[0].AuthorID != "U_example" {
			return fmt.Errorf("%s lost the author identity", input)
		}
		fmt.Printf("%s: ID=%s cached-name=%s profile-lookups=%d detail-fetches=%d\n",
			input, result.Items[0].AuthorID, result.Items[0].Author, lookups.Load(), details.Load())
	}
	reopened, err := notifs.OpenCache(root, "github.com", "example")
	if err != nil {
		return err
	}
	restarted := &notifs.Client{HTTP: server.Client(), BaseURL: client.BaseURL, GraphQL: client.GraphQL,
		Host: "github.com", Cache: reopened}
	fmt.Println("\nCached authors after restart (offline):")
	before := requests.Load()
	cached := restarted.Cached(notifs.Query{}, 10)
	if len(cached.Items) != 1 || cached.Items[0].Author != "oldname" || cached.Items[0].AuthorID != "U_example" ||
		requests.Load() != before {
		return fmt.Errorf("restart did not display the saved author without network requests")
	}
	fmt.Printf("startup: author=@%s ID=%s requests=%d\n",
		cached.Items[0].Author, cached.Items[0].AuthorID, requests.Load()-before)
	renamed.Store(true)
	result, err := restarted.Fetch(context.Background(), notifs.Query{}, 10, nil)
	if err != nil {
		return err
	}
	before = details.Load()
	if err := restarted.Enrich(context.Background(), result.Items); err != nil {
		return err
	}
	if len(result.Items) != 1 || result.Items[0].Author != "newname" || result.Items[0].AuthorID != "U_example" ||
		details.Load()-before != 1 {
		return fmt.Errorf("refresh did not update the author with one detail request")
	}
	fmt.Printf("refresh: author=@%s ID=%s detail-fetches=%d\n",
		result.Items[0].Author, result.Items[0].AuthorID, details.Load()-before)
	return nil
}
