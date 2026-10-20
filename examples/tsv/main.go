package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/cli"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func main() {
	err := cli.WriteList(context.Background(), os.Stdout, os.Stderr, []notifs.Thread{{
		ID: "42", Repository: "acme/app", Number: 7, Type: "PullRequest",
		Reason: "review_requested", Unread: true, Title: "Fix slow uploads", State: "open",
		UpdatedAt: time.Now().Add(-2 * time.Hour),
		URL:       "https://github.com/acme/app/pull/7",
	}, {
		ID: "43", Repository: "acme/app", Number: 8, Type: "Issue",
		Reason: "mention", Title: "Document the command-line workflow", State: "closed",
		UpdatedAt: time.Now().Add(-24 * time.Hour),
		URL:       "https://github.com/acme/app/issues/8",
	}}, false, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
