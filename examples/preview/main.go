package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func main() {
	repo := flag.String("repo", "", "fetch a live preview from OWNER/REPO")
	number := flag.Int("number", 0, "issue or pull request number")
	issue := flag.Bool("issue", false, "fetch an issue instead of a pull request")
	flag.Parse()
	if err := run(*repo, *number, *issue); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(repo string, number int, issue bool) error {
	p := notifs.Preview{
		Title: "Fix slow uploads", Body: "Keep uploads responsive on slow connections.",
		URL: "https://github.com/acme/app/pull/7",
		Metadata: []notifs.PreviewSection{
			{Name: "Author", Items: []string{"@octocat"}, Total: -1},
			{Name: "Assignees", Items: []string{"@mona"}, Total: 1},
			{Name: "Labels", Items: []string{"bug", "performance"}, Total: 2},
			{Name: "Review requested", Items: []string{"@acme/platform"}, Total: 1},
			{Name: "Reviewers", Items: []string{"@mona (approved)"}, Total: 1},
			{Name: "Checks: success", Items: []string{"Go tests (success)", "Lint (success)"}, Total: 2},
			{Name: "Branches", Items: []string{"fix-uploads → main"}, Total: -1},
			{Name: "Development", Items: []string{"acme/app#5 Slow uploads (open)"}, Total: 1},
			{Name: "Deployments (head commit)", Items: []string{"preview (success)"}, Total: 1},
			{Name: "Participants", Items: []string{"@octocat", "@mona"}, Total: 2},
		},
	}
	var fetchErr error
	if repo != "" || number != 0 {
		if repo == "" || number < 1 {
			return fmt.Errorf("--repo and a positive --number are required together")
		}
		client, err := notifs.NewClient("github.com")
		if err != nil {
			return err
		}
		kind, endpoint, web := "PullRequest", "pulls", "pull"
		if issue {
			kind, endpoint, web = "Issue", "issues", "issues"
		}
		p, fetchErr = client.Preview(context.Background(), notifs.Thread{
			Repository: repo, Number: number, Type: kind,
			SubjectURL: fmt.Sprintf("%srepos/%s/%s/%d", client.BaseURL, repo, endpoint, number),
			URL:        fmt.Sprintf("https://github.com/%s/%s/%d", repo, web, number),
		})
	}
	rendered, err := theme.Render(p.Markdown(), p.URL, 80, false, true)
	if err != nil {
		return err
	}
	if _, err := fmt.Print(rendered); err != nil {
		return err
	}
	return fetchErr
}
