package notifs

import (
	"testing"
	"time"
)

func TestQuery(t *testing.T) {
	item := Thread{Repository: "acme/app", Type: "PullRequest", Reason: "review_requested",
		Title: "Fix slow uploads", Unread: true, Author: "octocat", State: "open",
		UpdatedAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"", true},
		{`org:acme repo:acme/app is:pr is:unread reason:review-requested "slow uploads"`, true},
		{"author:@me state:open since:2026-10-08 before:2026-10-10", true},
		{"is:read", false},
		{"is:issue", false},
		{"repo:acme/other", false},
		{"org:other", false},
		{"before:2026-10-09", false},
		{"reason:mention", false},
		{"is:PullRequest reason:REVIEW_REQUESTED", true},
		{`is:pull_request reason:"review requested"`, true},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}

			if got := q.Match(item, "octocat"); got != tc.want {
				t.Fatalf("Match = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocalTypeAndReasonNames(t *testing.T) {
	for _, raw := range []string{"CheckSuite", "check_suite", "CHECK-SUITE"} {
		q, err := ParseQuery("is:check-suite reason:review-requested")
		if err != nil || !q.Match(Thread{Type: raw, Reason: "REVIEW_REQUESTED"}, "") {
			t.Fatalf("local names did not match %q: %v", raw, err)
		}
		if TypeName(raw) != "check-suite" {
			t.Fatalf("display name differs for %q", raw)
		}
	}
	if ReasonName("review_\x00requested") != "review-requested" {
		t.Fatal("reason contains control characters")
	}
}

func TestQueryErrors(t *testing.T) {
	for _, input := range []string{"is:saved", "is:done", "project:foo", "since:yesterday", `text:"oops`, "repo:foo", "org:", "state:nope"} {
		if _, err := ParseQuery(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
