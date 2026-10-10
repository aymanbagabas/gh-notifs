package main

import (
	"fmt"
	"os"

	"github.com/aymanbagabas/gh-notifs/internal/cli"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func main() {
	err := cli.WriteTSV(os.Stdout, []notifs.Thread{{
		ID: "42", Repository: "acme/app", Number: 7, Type: "PullRequest",
		Reason: "review_requested", Unread: true, Title: "Fix slow uploads",
		URL: "https://github.com/acme/app/pull/7",
	}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
