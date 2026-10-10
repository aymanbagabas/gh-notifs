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
		{`org:acme repo:acme/app is:issue-or-pull-request is:unread reason:review-requested "slow uploads"`, true},
		{"author:@me state:open since:2026-10-08 before:2026-10-10", true},
		{"is:read", false},
		{"reason:participating is:unread is:issue-or-pull-request", false},
		{"reason:participating is:read", false},
		{"reason:participating is:issue-or-pull-request", false},
		{"is:issue-or-pull-request", true},
		{"is:issue", false},
		{"is:pr", true},
		{"is:pull-request", true},
		{"repo:acme/other", false},
		{"org:other", false},
		{"before:2026-10-09", false},
		{"reason:mention", false},
		{"is:ISSUE-OR-PULL-REQUEST reason:REVIEW_REQUESTED", true},
		{`is:issue_or_pull_request reason:"review requested"`, true},
		{"repo:acme/other repo:ACME/APP", true},
		{"org:other org:ACME repo:acme/app", true},
		{"repo:acme/other repo:acme/app org:other", false},
		{"author:someone author:@me state:closed state:OPEN", true},
		{"reason:mention reason:review_requested", true},
		{"reason:participating reason:review-requested", true},
		{"reason:participating reason:subscribed", false},
		{"is:release is:issue-or-pull-request is:unread", true},
		{"is:issue-or-pull-request is:release is:read", false},
		{"is:issue is:pr is:unread", true},
		{"is:issue is:pr is:read", false},
		{"is:read is:unread", true},
		{"is:unread is:read", true},
		{"is:issue-or-pull-request is:issue_or_pull_request", true},
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

func TestResolvedAuthorsSurviveRequestSplits(t *testing.T) {
	q, err := ParseQuery("repo:acme/app repo:acme/other reason:participating reason:subscribed author:newname author:someone")
	if err != nil {
		t.Fatal(err)
	}
	q.authorIDs = map[string]string{"newname": "U_42", "someone": "U_99"}
	requests := q.Requests()
	if len(requests) != 4 {
		t.Fatalf("expected repository and reason splits, got %d", len(requests))
	}
	for _, request := range requests {
		item := Thread{Repository: request.values("repo")[0], Reason: "subscribed", Author: "oldname", AuthorID: "U_42"}
		if !request.match(item, "", true) {
			t.Fatal("request split lost resolved author identity")
		}
		item.Author, item.AuthorID = "newname", "U_other"
		if request.match(item, "", true) {
			t.Fatal("matching login overrode a different author ID")
		}
		item.AuthorID = ""
		if request.match(item, "", true) {
			t.Fatal("missing author identity fell back to display text")
		}
	}
}

func TestNotificationTypeFilters(t *testing.T) {
	for _, tc := range []struct {
		filter string
		types  []string
	}{
		{"is:issue", []string{"Issue"}},
		{"is:ISSUE", []string{"Issue"}},
		{"is:pr", []string{"PullRequest"}},
		{"is:PR", []string{"PullRequest"}},
		{"is:pull-request", []string{"PullRequest"}},
		{"is:pull_request", []string{"PullRequest"}},
		{"is:PullRequest", []string{"PullRequest"}},
		{"is:issue-or-pull-request", []string{"Issue", "PullRequest"}},
		{"is:issue is:pr", []string{"Issue", "PullRequest"}},
	} {
		q, err := ParseQuery(tc.filter)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"Issue", "PullRequest", "Release"} {
			if got, want := q.Match(Thread{Type: kind}, ""), slices.Contains(tc.types, kind); got != want {
				t.Errorf("%s with %s: got %t, want %t", tc.filter, kind, got, want)
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
	q, err := ParseQuery("repo:ACME/app repo:acme/other repo:acme/APP reason:participating reason:subscribed is:issue-or-pull-request is:unread")
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
