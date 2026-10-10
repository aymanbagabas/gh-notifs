package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const metadataFixture = `{
"author":{"login":"author"},"mergedBy":{"login":"merger"},
"assignees":{"totalCount":1,"nodes":[{"login":"assignee"}]},
"labels":{"totalCount":0,"nodes":[]},"participants":{"totalCount":1,"nodes":[{"login":"participant"}]},
"milestone":{"title":"Next release"},"headRefName":"feature","baseRefName":"main","reviewDecision":"APPROVED",
"reviewRequests":{"totalCount":2,"nodes":[{"requestedReviewer":{"login":"reviewer"}},{"requestedReviewer":{"slug":"core","organization":{"login":"acme"}}}]},
"latestReviews":{"totalCount":1,"nodes":[{"author":{"login":"reviewer"},"state":"APPROVED"}]},
"development":{"totalCount":1,"nodes":[{"number":3,"title":"Fix uploads","state":"OPEN","repository":{"nameWithOwner":"acme/app"}}]},
"commits":{"nodes":[{"commit":{"oid":"abc","statusCheckRollup":{"state":"SUCCESS","contexts":{"totalCount":2,"nodes":[{"name":"Tests","status":"COMPLETED","conclusion":"SUCCESS"},{"context":"CI","state":"PENDING"}]}},"deployments":{"totalCount":1,"nodes":[{"environment":"preview","state":"ACTIVE","latestStatus":{"state":"SUCCESS"}}]}}}]}
}`

func TestPreviewMetadata(t *testing.T) {
	for _, kind := range []string{"Issue", "PullRequest"} {
		t.Run(kind, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/graphql" {
					fmt.Fprint(w, `{"title":"Title","body":"Original body","html_url":"https://github.com/acme/app/pull/7"}`)
					return
				}
				var payload struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Variables["owner"] != "acme" || payload.Variables["repo"] != "app" ||
					payload.Variables["number"] != float64(7) || payload.Query != previewMetadataQuery {
					t.Errorf("incorrect metadata query: %+v", payload)
				}
				fmt.Fprintf(w, `{"data":{"repository":{"issueOrPullRequest":%s}}}`, metadataFixture)
			})
			p, err := c.Preview(context.Background(), Thread{Type: kind, Repository: "acme/app", Number: 7, SubjectURL: c.BaseURL + "subject"})
			if err != nil {
				t.Fatal(err)
			}
			markdown := p.Markdown()
			for _, want := range []string{"@author", "@assignee", "@participant", "Next release", `acme/app\#3`, "Original body"} {
				if !strings.Contains(markdown, want) {
					t.Errorf("missing %q", want)
				}
			}
			if kind == "PullRequest" {
				for _, want := range []string{"@acme/core", "@reviewer (approved)", "Tests (success)", "CI (pending)", "preview (success)", "feature → main", "@merger"} {
					if !strings.Contains(markdown, want) {
						t.Errorf("missing %q", want)
					}
				}
			} else if strings.Contains(markdown, "**Checks") || strings.Contains(markdown, "**Reviewers") {
				t.Fatal("issue shows pull request metadata")
			}
			if strings.Index(markdown, "Original body") < strings.Index(markdown, "**Development") {
				t.Fatal("metadata must precede the body")
			}
		})
	}
}

func TestPreviewPartialMetadataErrorsPreserveBody(t *testing.T) {
	for _, response := range []string{
		`{"data":{"repository":null},"errors":[{"message":"Permission denied"}]}`,
		`{"data":{"repository":{"issueOrPullRequest":{"author":{"login":"author"},"assignees":null}}},"errors":[{"message":"Permission denied"}]}`,
	} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/graphql" {
				fmt.Fprint(w, response)
			} else {
				fmt.Fprint(w, `{"title":"Title","body":"Keep this body"}`)
			}
		})
		p, err := c.Preview(context.Background(), Thread{Type: "Issue", Repository: "acme/app", Number: 7, SubjectURL: c.BaseURL + "subject"})
		if err == nil || !strings.Contains(err.Error(), "Permission denied") || p.Body != "Keep this body" {
			t.Fatalf("partial metadata failure lost body or error: %+v, %v", p, err)
		}
		if len(p.Metadata) > 0 && !strings.Contains(p.Markdown(), "**Assignees:** Unavailable") {
			t.Fatal("unavailable metadata was reported as empty")
		}
	}
}

func TestPreviewOtherSubjectsDoNotFetchGraphQL(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			t.Error("release should not request issue metadata")
		}
		fmt.Fprint(w, `{"body":"Release notes","author":{"login":"author"}}`)
	})
	p, err := c.Preview(context.Background(), Thread{Type: "Release", SubjectURL: c.BaseURL + "subject"})
	if err != nil || !strings.Contains(p.Markdown(), "@author") {
		t.Fatalf("release author missing: %+v, %v", p, err)
	}
}

func TestPreviewMetadataLimitAndEscaping(t *testing.T) {
	var labels []string
	for i := range 20 {
		labels = append(labels, fmt.Sprintf(`{"name":"label-%d"}`, i))
	}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			fmt.Fprintf(w, `{"data":{"repository":{"issueOrPullRequest":{"author":{"login":"a"},"labels":{"totalCount":25,"nodes":[%s]}}}}}`, strings.Join(labels, ","))
		} else {
			fmt.Fprint(w, `{"body":"Body","html_url":"https://github.com/acme/app/issues/7"}`)
		}
	})
	p, err := c.Preview(context.Background(), Thread{Type: "Issue", Repository: "acme/app", Number: 7, SubjectURL: c.BaseURL + "subject"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Markdown(), "showing 20 of 25 · [View all](https://github.com/acme/app/issues/7)") {
		t.Fatal("metadata lacks its total or overflow link")
	}
	p.Title = "[title](https://bad.example)"
	p.Metadata = []PreviewSection{{Name: "Labels", Items: []string{"[bad](https://bad.example)\x1b[31m"}, Total: 2, URL: "javascript:bad"}}
	markdown := p.Markdown()
	if strings.Contains(markdown, "[bad](") || strings.Contains(markdown, "[title](") ||
		strings.Contains(markdown, "\x1b") || strings.Contains(markdown, "javascript:") {
		t.Fatal("metadata or title emitted unsafe formatting")
	}
}
