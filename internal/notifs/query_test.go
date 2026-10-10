package notifs

import (
	"reflect"
	"slices"
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
		{"reason:participating is:unread is:pr", false},
		{"reason:participating is:read", false},
		{"reason:participating is:issue", false},
		{"is:issue", true},
		{"is:pr", true},
		{"is:pull-request", true},
		{"repo:acme/other", false},
		{"org:other", false},
		{"before:2026-10-09", false},
		{"reason:mention", false},
		{"is:PullRequest reason:REVIEW_REQUESTED", true},
		{`is:pull_request reason:"review requested"`, true},
		{"repo:acme/other repo:ACME/APP", true},
		{"org:other org:ACME repo:acme/app", true},
		{"repo:acme/other repo:acme/app org:other", false},
		{"author:someone author:@me state:closed state:OPEN", true},
		{"reason:mention reason:review_requested", true},
		{"reason:participating reason:review-requested", true},
		{"reason:participating reason:subscribed", false},
		{"is:issue is:pr is:unread", true},
		{"is:pr is:issue is:read", false},
		{"is:read is:unread", true},
		{"is:unread is:read", true},
		{"is:issue is:issue-or-pull-request", true},
		{"is:pr is:pull_request is:PullRequest", true},
		{"slow uploads", true},
		{"slow missing", false},
		{"slow OR uploads", false},
		{"slow AND uploads", false},
		{"since:2026-10-10 since:2026-10-01", false},
		{"before:2026-10-09 before:2026-10-11", false},
		{"since:2026-10-09 before:2026-10-10 since:2026-10-08 before:2026-10-11", true},
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

func TestWebsiteIssueAliases(t *testing.T) {
	for _, alias := range []string{"issue", "pr", "pull-request", "pull_request", "PullRequest", "issue-or-pull-request"} {
		q, err := ParseQuery("is:" + alias)
		if err != nil {
			t.Fatal(err)
		}
		if q.Terms[0].Value != "issue-or-pull-request" {
			t.Fatalf("%s did not normalize to the website's combined type", alias)
		}
		for _, kind := range []string{"Issue", "PullRequest", "Release"} {
			if got, want := q.Match(Thread{Type: kind}, ""), kind != "Release"; got != want {
				t.Errorf("is:%s with %s: got %t, want %t", alias, kind, got, want)
			}
		}
	}
}

func TestParticipationRequiresRequestMembership(t *testing.T) {
	q, err := ParseQuery("reason:participating reason:subscribed")
	if err != nil {
		t.Fatal(err)
	}
	item := Thread{Reason: "state_change"}
	if q.Match(item, "") || !q.match(item, "", true) {
		t.Fatal("aggregate participation must use API membership, not the reason")
	}
}

func TestRequestAlternativesAndImmutability(t *testing.T) {
	q, err := ParseQuery("repo:ACME/app repo:acme/other repo:acme/APP reason:participating reason:subscribed is:pr is:unread")
	if err != nil {
		t.Fatal(err)
	}
	original := Query{Raw: q.Raw, Terms: slices.Clone(q.Terms)}
	requests := q.Requests()
	if len(requests) != 4 {
		t.Fatalf("got %d requests, want 4", len(requests))
	}
	var endpoints []string
	for _, request := range requests {
		if len(request.values("repo")) != 1 {
			t.Fatalf("request has multiple repositories: %+v", request)
		}
		endpoints = append(endpoints, Endpoint(request, 1))
	}
	reversed := Query{Terms: slices.Clone(q.Terms)}
	slices.Reverse(reversed.Terms)
	var reversedEndpoints []string
	for _, request := range reversed.Requests() {
		reversedEndpoints = append(reversedEndpoints, Endpoint(request, 1))
	}
	slices.Sort(endpoints)
	slices.Sort(reversedEndpoints)
	if !slices.Equal(endpoints, reversedEndpoints) {
		t.Fatalf("term order changed requests: %v vs %v", endpoints, reversedEndpoints)
	}
	otherTerms := slices.Clone(requests[1].Terms)
	requests[0].Terms[0].Value = "changed"
	if !reflect.DeepEqual(q, original) || !slices.Equal(requests[1].Terms, otherTerms) {
		t.Fatal("request terms share caller or sibling storage")
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
	for _, input := range []string{"is:participating", "is:saved", "is:done", "project:foo", "since:yesterday", `text:"oops`, "repo:foo", "org:", "state:nope"} {
		if _, err := ParseQuery(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
