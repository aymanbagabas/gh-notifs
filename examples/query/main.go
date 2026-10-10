package main

import (
	"fmt"
	"os"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func main() {
	for _, input := range []string{
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
		fmt.Printf("%s\n%d request(s); multiple requests run concurrently:\n", input, len(requests))
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
	for _, alias := range []string{"issue", "pr", "pull-request", "issue-or-pull-request"} {
		query, err := notifs.ParseQuery("is:" + alias)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("is:%s: issue=%t pull-request=%t release=%t\n", alias,
			query.Match(notifs.Thread{Type: "Issue"}, ""),
			query.Match(notifs.Thread{Type: "PullRequest"}, ""),
			query.Match(notifs.Thread{Type: "Release"}, ""))
	}
}
