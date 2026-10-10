package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDiscussionCommentsReviewsAndPagination(t *testing.T) {
	for _, kind := range []string{"Issue", "PullRequest"} {
		t.Run(kind, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				var payload struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				if payload.Query != discussionQuery || payload.Variables["owner"] != "acme" ||
					payload.Variables["repo"] != "app" || payload.Variables["number"] != float64(7) {
					t.Error("incorrect discussion query")
				}
				if payload.Variables["cursor"] == nil {
					fmt.Fprint(w, `{"data":{"repository":{"issueOrPullRequest":{"conversation":{
"totalCount":3,"pageInfo":{"endCursor":"next","hasNextPage":true},"nodes":[
{"author":{"login":"author"},"body":"**Comment**","createdAt":"2026-10-09T12:00:00Z","url":"https://github.com/acme/app/issues/7#issuecomment-1"},
{"author":{"login":"reviewer"},"body":"Approved message","state":"APPROVED","createdAt":"2026-10-09T12:00:00Z","submittedAt":"2026-10-09T13:00:00Z"}]
}}}}}`)
				} else {
					if payload.Variables["cursor"] != "next" {
						t.Error("pagination cursor was not forwarded")
					}
					fmt.Fprint(w, `{"data":{"repository":{"issueOrPullRequest":{"conversation":{"totalCount":3,"pageInfo":{"hasNextPage":false},"nodes":[{"body":"Last comment"}]}}}}}`)
				}
			})
			item := Thread{Repository: "acme/app", Number: 7, Type: kind}
			page, err := c.Discussion(context.Background(), item, false, "")
			if err != nil || !page.More || page.Total != 3 || len(page.Comments) != 2 {
				t.Fatalf("incorrect discussion: %+v, %v", page, err)
			}
			if page.Comments[1].State != "approved" || page.Comments[1].CreatedAt.Hour() != 13 {
				t.Fatal("review lost its state or submission time")
			}
			if !strings.Contains(page.Markdown(), "**Comment**") || !strings.Contains(page.Markdown(), "Approved message") {
				t.Fatal("discussion lost comment or review Markdown")
			}
			next, err := c.Discussion(context.Background(), item, false, page.NextCursor)
			if err != nil || next.More || next.Comments[0].Author != "Deleted account" {
				t.Fatalf("incorrect next page: %+v, %v", next, err)
			}
		})
	}
}

func TestInlineReviewThreads(t *testing.T) {
	comments := make([]map[string]any, 20)
	for i := range comments {
		comments[i] = map[string]any{"author": map[string]string{"login": "reviewer"}, "body": fmt.Sprintf("Reply %d", i),
			"url": "https://github.com/acme/app/pull/7#discussion_r1", "diffHunk": "@@ -1 +1 @@\n+```"}
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var payload struct{ Query string }
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Query != reviewThreadsQuery || !strings.Contains(payload.Query, "comments(first:20)") {
			t.Error("thread query did not cap its comments")
		}
		data, err := json.Marshal(comments)
		if err != nil {
			t.Error(err)
		}
		fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{
"totalCount":21,"pageInfo":{"endCursor":"threads-next","hasNextPage":true},
"nodes":[{"path":"upload.go","line":null,"originalLine":42,"isResolved":true,"isOutdated":true,
"comments":{"totalCount":25,"nodes":%s}}]}}}}}`, data)
	})
	page, err := c.Discussion(context.Background(), Thread{Repository: "acme/app", Number: 7, Type: "PullRequest"}, true, "")
	if err != nil || len(page.Threads) != 1 || !page.More || page.NextCursor != "threads-next" {
		t.Fatalf("incorrect thread response: %+v, %v", page, err)
	}
	thread := page.Threads[0]
	if thread.Line != 42 || !thread.Resolved || !thread.Outdated || len(thread.Comments) != 20 {
		t.Fatalf("missing thread metadata: %+v", thread)
	}
	for _, want := range []string{"upload.go:42", "resolved, outdated", "````diff", "Reply 19",
		"Showing 20 of 25 comments.", "[Open the full thread](https://github.com/acme/app/pull/7#discussion_r1)"} {
		if !strings.Contains(page.Markdown(), want) {
			t.Fatalf("thread rendering lacks %q", want)
		}
	}
}

func TestDiscussionErrorsAndUnsupportedTypes(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"repository":null},"errors":[{"message":"Permission denied"}]}`)
	})
	if _, err := c.Discussion(context.Background(), Thread{Type: "Release"}, false, ""); err == nil {
		t.Fatal("unsupported notification accepted")
	}
	if _, err := c.Discussion(context.Background(), Thread{Type: "Issue", Number: 7}, true, ""); err == nil {
		t.Fatal("issue accepted inline review threads")
	}
	if _, err := c.Discussion(context.Background(), Thread{Type: "PullRequest", Number: 7, Repository: "acme/app"}, false, ""); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("GraphQL error was hidden: %v", err)
	}
}
