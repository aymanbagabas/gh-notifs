package notifs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const previewMetadataQuery = `query($owner:String!,$repo:String!,$number:Int!){
repository(owner:$owner,name:$repo){issueOrPullRequest(number:$number){
... on Issue {
author{login}
assignees(first:20){totalCount nodes{login}}
labels(first:20){totalCount nodes{name}}
participants(first:20){totalCount nodes{login}}
milestone{title}
development:closedByPullRequestsReferences(first:20,includeClosedPrs:true){totalCount nodes{number title state repository{nameWithOwner}}}
}
... on PullRequest {
author{login} mergedBy{login} headRefName baseRefName reviewDecision
assignees(first:20){totalCount nodes{login}}
labels(first:20){totalCount nodes{name}}
participants(first:20){totalCount nodes{login}}
milestone{title}
reviewRequests(first:20){totalCount nodes{requestedReviewer{
... on User{login} ... on Bot{login} ... on Mannequin{login}
... on Team{name slug organization{login}} ... on EnterpriseTeam{name combinedSlug}
}}}
latestReviews(first:20){totalCount nodes{author{login} state}}
development:closingIssuesReferences(first:20){totalCount nodes{number title state repository{nameWithOwner}}}
commits(last:1){nodes{commit{
statusCheckRollup{state contexts(first:20){totalCount nodes{
... on CheckRun{name status conclusion}
... on StatusContext{context state}
}}}
deployments(first:20,orderBy:{field:CREATED_AT,direction:DESC}){totalCount nodes{environment state latestStatus{state}}}
}}}
}
}}}`

type previewConnection[T any] struct {
	TotalCount int
	Nodes      []T
}

type previewActor struct {
	Login, Name, Slug, CombinedSlug string
	Organization                    struct{ Login string }
}

func actorName(actor *previewActor) string {
	if actor == nil {
		return "Deleted account"
	}
	switch {
	case actor.Login != "":
		return "@" + actor.Login
	case actor.Organization.Login != "":
		return "@" + actor.Organization.Login + "/" + actor.Slug
	case actor.CombinedSlug != "":
		return "@" + actor.CombinedSlug
	default:
		return actor.Name
	}
}

type previewDetails struct {
	Author, MergedBy                         *previewActor
	HeadRefName, BaseRefName, ReviewDecision string
	Assignees, Participants                  *previewConnection[*previewActor]
	Labels                                   *previewConnection[struct{ Name string }]
	Milestone                                *struct{ Title string }
	ReviewRequests                           *previewConnection[struct{ RequestedReviewer *previewActor }]
	LatestReviews                            *previewConnection[struct {
		Author *previewActor
		State  string
	}]
	Development *previewConnection[struct {
		Number       int
		Title, State string
		Repository   struct{ NameWithOwner string }
	}]
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string
					Contexts *previewConnection[struct{ Name, Status, Conclusion, Context, State string }]
				}
				Deployments *previewConnection[struct {
					Environment, State string
					LatestStatus       *struct{ State string }
				}]
			}
		}
	}
}

func previewSection[T any](name, target string, connection *previewConnection[T], describe func(T) string) PreviewSection {
	section := PreviewSection{Name: name, URL: target, Total: -1}
	if connection != nil {
		section.Total = connection.TotalCount
		for _, node := range connection.Nodes {
			section.Items = append(section.Items, describe(node))
		}
	}
	return section
}

func (c *Client) previewMetadata(ctx context.Context, t Thread) ([]PreviewSection, error) {
	owner, repo, _ := strings.Cut(t.Repository, "/")
	payload, err := json.Marshal(map[string]any{
		"query":     previewMetadataQuery,
		"variables": map[string]any{"owner": owner, "repo": repo, "number": t.Number},
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.request(ctx, http.MethodPost, c.GraphQL, bytes.NewReader(payload), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var response struct {
		Data struct {
			Repository *struct{ IssueOrPullRequest *previewDetails }
		}
		Errors []struct{ Message string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	var apiErr error
	for _, problem := range response.Errors {
		apiErr = errors.Join(apiErr, errors.New(problem.Message))
	}
	if response.Data.Repository == nil || response.Data.Repository.IssueOrPullRequest == nil {
		return nil, errors.Join(apiErr, fmt.Errorf("GitHub did not return preview metadata for %s", t.Reference()))
	}
	d := response.Data.Repository.IssueOrPullRequest
	sections := []PreviewSection{
		{Name: "Author", Items: []string{actorName(d.Author)}, Total: -1},
		previewSection("Assignees", t.URL, d.Assignees, actorName),
		previewSection("Labels", t.URL, d.Labels, func(label struct{ Name string }) string { return label.Name }),
		previewSection("Participants", t.URL, d.Participants, actorName),
	}
	if d.Author == nil && apiErr != nil {
		sections[0].Items = nil
	}
	if d.MergedBy != nil {
		sections = append(sections, PreviewSection{Name: "Merged by", Items: []string{actorName(d.MergedBy)}, Total: -1})
	}
	if d.Milestone != nil {
		sections = append(sections, PreviewSection{Name: "Milestone", Items: []string{d.Milestone.Title}, Total: -1})
	}
	if t.Type == "PullRequest" {
		if d.ReviewDecision != "" {
			sections = append(sections, PreviewSection{Name: "Review decision", Items: []string{ReasonName(d.ReviewDecision)}, Total: -1})
		}
		sections = append(sections,
			previewSection("Review requested", t.URL, d.ReviewRequests, func(request struct{ RequestedReviewer *previewActor }) string {
				return actorName(request.RequestedReviewer)
			}),
			previewSection("Reviewers", t.URL, d.LatestReviews, func(review struct {
				Author *previewActor
				State  string
			}) string {
				return actorName(review.Author) + " (" + ReasonName(review.State) + ")"
			}),
		)
		if d.HeadRefName != "" || d.BaseRefName != "" {
			sections = append(sections, PreviewSection{Name: "Branches", Items: []string{d.HeadRefName + " → " + d.BaseRefName}, Total: -1})
		}
	}
	sections = append(sections, previewSection("Development", t.URL, d.Development, func(ref struct {
		Number       int
		Title, State string
		Repository   struct{ NameWithOwner string }
	}) string {
		return fmt.Sprintf("%s#%d %s (%s)", ref.Repository.NameWithOwner, ref.Number, ref.Title, ReasonName(ref.State))
	}))
	if t.Type == "PullRequest" && len(d.Commits.Nodes) > 0 {
		commit := d.Commits.Nodes[0].Commit
		checks := PreviewSection{Name: "Checks (head commit)", Total: 0, URL: t.URL + "/checks"}
		if rollup := commit.StatusCheckRollup; rollup != nil {
			checks = previewSection("Checks: "+ReasonName(rollup.State), t.URL+"/checks", rollup.Contexts,
				func(check struct{ Name, Status, Conclusion, Context, State string }) string {
					name, state := check.Name, check.Conclusion
					if name == "" {
						name, state = check.Context, check.State
					} else if state == "" {
						state = check.Status
					}
					return name + " (" + ReasonName(state) + ")"
				})
		} else if apiErr != nil {
			checks.Total = -1
		}
		sections = append(sections, checks, previewSection("Deployments (head commit)",
			"https://"+c.Host+"/"+t.Repository+"/deployments", commit.Deployments, func(deployment struct {
				Environment, State string
				LatestStatus       *struct{ State string }
			}) string {
				state := deployment.State
				if deployment.LatestStatus != nil {
					state = deployment.LatestStatus.State
				}
				return deployment.Environment + " (" + ReasonName(state) + ")"
			}))
	}
	return sections, apiErr
}
