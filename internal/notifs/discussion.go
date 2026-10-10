package notifs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const discussionQuery = `query($owner:String!,$repo:String!,$number:Int!,$cursor:String){
repository(owner:$owner,name:$repo){issueOrPullRequest(number:$number){
... on Issue {conversation:timelineItems(first:20,after:$cursor,itemTypes:[ISSUE_COMMENT]){
totalCount:filteredCount pageInfo{endCursor hasNextPage} nodes{
... on IssueComment{id author{login} createdAt body url}
}}}
... on PullRequest {conversation:timelineItems(first:20,after:$cursor,itemTypes:[ISSUE_COMMENT,PULL_REQUEST_REVIEW]){
totalCount:filteredCount pageInfo{endCursor hasNextPage} nodes{
... on IssueComment{id author{login} createdAt body url}
... on PullRequestReview{id author{login} createdAt submittedAt body url state}
}}}
}}}`

const reviewThreadsQuery = `query($owner:String!,$repo:String!,$number:Int!,$cursor:String){
repository(owner:$owner,name:$repo){pullRequest(number:$number){
reviewThreads(first:20,after:$cursor){totalCount pageInfo{endCursor hasNextPage} nodes{
id path line originalLine isResolved isOutdated
comments(first:20){totalCount nodes{id author{login} createdAt body url diffHunk}}
}}
}}}`

type DiscussionComment struct {
	Author, Body, URL, State string
	CreatedAt                time.Time
}

type ReviewThread struct {
	Path, DiffHunk, URL string
	Line, CommentTotal  int
	Resolved, Outdated  bool
	Comments            []DiscussionComment
}

type DiscussionPage struct {
	Comments   []DiscussionComment
	Threads    []ReviewThread
	Total      int
	NextCursor string
	More       bool
}

type discussionCommentData struct {
	Author                     *previewActor
	Body, URL, State, DiffHunk string
	CreatedAt, SubmittedAt     time.Time
}

func (d discussionCommentData) comment() DiscussionComment {
	created := d.CreatedAt
	if !d.SubmittedAt.IsZero() {
		created = d.SubmittedAt
	}
	return DiscussionComment{Author: actorName(d.Author), Body: d.Body, URL: d.URL,
		State: ReasonName(d.State), CreatedAt: created}
}

func (c *Client) Discussion(ctx context.Context, t Thread, threads bool, cursor string) (DiscussionPage, error) {
	var page DiscussionPage
	if t.Number < 1 || (t.Type != "Issue" && t.Type != "PullRequest") || (threads && t.Type != "PullRequest") {
		return page, fmt.Errorf("this notification has no supported discussion endpoint")
	}
	query := discussionQuery
	if threads {
		query = reviewThreadsQuery
	}
	owner, repo, _ := strings.Cut(t.Repository, "/")
	var after *string
	if cursor != "" {
		after = &cursor
	}
	payload, err := json.Marshal(map[string]any{"query": query, "variables": map[string]any{
		"owner": owner, "repo": repo, "number": t.Number, "cursor": after,
	}})
	if err != nil {
		return page, err
	}
	resp, err := c.request(ctx, http.MethodPost, c.GraphQL, bytes.NewReader(payload), nil)
	if err != nil {
		return page, err
	}
	defer resp.Body.Close()
	type pageInfo struct {
		EndCursor   string
		HasNextPage bool
	}
	var response struct {
		Data struct {
			Repository *struct {
				IssueOrPullRequest *struct {
					Conversation *struct {
						TotalCount int
						PageInfo   pageInfo
						Nodes      []*discussionCommentData
					}
				}
				PullRequest *struct {
					ReviewThreads *struct {
						TotalCount int
						PageInfo   pageInfo
						Nodes      []*struct {
							Path, ID               string
							Line, OriginalLine     int
							IsResolved, IsOutdated bool
							Comments               *previewConnection[*discussionCommentData]
						}
					}
				}
			}
		}
		Errors []struct{ Message string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return page, err
	}
	for _, problem := range response.Errors {
		err = errors.Join(err, errors.New(problem.Message))
	}
	if err != nil {
		return page, fmt.Errorf("load discussion: %w", err)
	}
	repository := response.Data.Repository
	if threads && repository != nil && repository.PullRequest != nil && repository.PullRequest.ReviewThreads != nil {
		connection := repository.PullRequest.ReviewThreads
		page.Total, page.More, page.NextCursor = connection.TotalCount, connection.PageInfo.HasNextPage, connection.PageInfo.EndCursor
		for _, node := range connection.Nodes {
			if node == nil {
				continue
			}
			thread := ReviewThread{Path: node.Path, Line: node.Line, Resolved: node.IsResolved, Outdated: node.IsOutdated}
			if thread.Line == 0 {
				thread.Line = node.OriginalLine
			}
			if node.Comments != nil {
				thread.CommentTotal = node.Comments.TotalCount
				for _, comment := range node.Comments.Nodes {
					if comment != nil {
						if len(thread.Comments) == 0 {
							thread.DiffHunk, thread.URL = comment.DiffHunk, comment.URL
						}
						thread.Comments = append(thread.Comments, comment.comment())
					}
				}
			}
			page.Threads = append(page.Threads, thread)
		}
		return page, nil
	}
	if !threads && repository != nil && repository.IssueOrPullRequest != nil && repository.IssueOrPullRequest.Conversation != nil {
		connection := repository.IssueOrPullRequest.Conversation
		page.Total, page.More, page.NextCursor = connection.TotalCount, connection.PageInfo.HasNextPage, connection.PageInfo.EndCursor
		for _, comment := range connection.Nodes {
			if comment != nil {
				page.Comments = append(page.Comments, comment.comment())
			}
		}
		return page, nil
	}
	return page, fmt.Errorf("GitHub did not return the discussion for %s", t.Reference())
}

func (c DiscussionComment) Markdown() string {
	title := c.Author
	if c.State != "" {
		title += " · " + c.State + " review"
	}
	body := c.Body
	if body == "" {
		body = "No written message."
	}
	p := Preview{Title: title, Body: body}
	if !c.CreatedAt.IsZero() {
		p.Metadata = []PreviewSection{{Name: "Posted", Items: []string{c.CreatedAt.UTC().Format(time.RFC3339)}, Total: -1}}
	}
	return p.Markdown() + "\n\n" + markdownLink("Open on GitHub", c.URL) + "\n\n---\n\n"
}

func (p DiscussionPage) Markdown() string {
	var out strings.Builder
	for _, comment := range p.Comments {
		out.WriteString(comment.Markdown())
	}
	for _, thread := range p.Threads {
		location := thread.Path
		if thread.Line > 0 {
			location += fmt.Sprintf(":%d", thread.Line)
		}
		state := "unresolved"
		if thread.Resolved {
			state = "resolved"
		}
		if thread.Outdated {
			state += ", outdated"
		}
		title := Preview{Title: location, Metadata: []PreviewSection{{Name: "Thread", Items: []string{state}, Total: -1}}}
		out.WriteString(title.Markdown())
		if thread.DiffHunk != "" {
			longest, run := 0, 0
			for _, r := range thread.DiffHunk {
				if r == '`' {
					run++
					longest = max(longest, run)
				} else {
					run = 0
				}
			}
			fence := strings.Repeat("`", max(3, longest+1))
			fmt.Fprintf(&out, "%sdiff\n%s\n%s\n\n", fence, thread.DiffHunk, fence)
		}
		for _, comment := range thread.Comments {
			out.WriteString(comment.Markdown())
		}
		if thread.CommentTotal > len(thread.Comments) {
			fmt.Fprintf(&out, "Showing %d of %d comments. %s\n\n", len(thread.Comments), thread.CommentTotal,
				markdownLink("Open the full thread", thread.URL))
		}
	}
	if out.Len() == 0 {
		return "No comments or reviews on this page."
	}
	return out.String()
}
