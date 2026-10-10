package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func main() {
	repo := flag.String("repo", "", "fetch from OWNER/REPO instead of the sample")
	number := flag.Int("number", 0, "issue or pull request number")
	issue := flag.Bool("issue", false, "fetch issue comments")
	threads := flag.Bool("threads", false, "show inline review threads")
	cursor := flag.String("cursor", "", "fetch the next page from this cursor")
	flag.Parse()
	if err := run(*repo, *number, *issue, *threads, *cursor); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(repo string, number int, issue, threads bool, cursor string) error {
	posted := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	page := notifs.DiscussionPage{Total: 2, Comments: []notifs.DiscussionComment{
		{Author: "@octocat", Body: "Please review the **cancellation** behavior.", CreatedAt: posted},
		{Author: "@mona", Body: "The tests look good.", State: "approved", CreatedAt: posted.Add(time.Hour)},
	}}
	if threads {
		page = notifs.DiscussionPage{Total: 1, Threads: []notifs.ReviewThread{{
			Path: "upload.go", Line: 42, Resolved: true, CommentTotal: 2,
			DiffHunk: "@@ -41,1 +41,1 @@\n-return send()\n+return send(ctx)",
			Comments: []notifs.DiscussionComment{
				{Author: "@mona", Body: "Please pass `ctx` through here.", CreatedAt: posted},
				{Author: "@octocat", Body: "Done; cancellation now reaches the request.", CreatedAt: posted.Add(time.Hour)},
			},
		}}}
	}
	if repo != "" || number != 0 {
		if repo == "" || number < 1 {
			return fmt.Errorf("--repo and a positive --number are required together")
		}
		client, err := notifs.NewClient("github.com")
		if err != nil {
			return err
		}
		kind := "PullRequest"
		if issue {
			kind = "Issue"
		}
		page, err = client.Discussion(context.Background(), notifs.Thread{Repository: repo, Number: number, Type: kind}, threads, cursor)
		if err != nil {
			return err
		}
	}
	rendered, err := theme.Render(page.Markdown(), "", 80, false, false)
	if err != nil {
		return err
	}
	if _, err := fmt.Printf("Entries: %d; total: %d; more: %t\n%s", len(page.Comments)+len(page.Threads), page.Total, page.More, rendered); err != nil {
		return err
	}
	if page.More {
		_, err = fmt.Fprintf(os.Stderr, "Next page: --cursor %q\n", page.NextCursor)
	}
	return err
}
