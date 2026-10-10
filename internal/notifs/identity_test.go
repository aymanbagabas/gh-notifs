package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthorMeAfterAccountRename(t *testing.T) {
	var renamed atomic.Bool
	var identityCalls atomic.Int32
	var graphCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login := "oldname"
		if renamed.Load() {
			login = "newname"
		}
		switch r.URL.Path {
		case "/user":
			identityCalls.Add(1)
			fmt.Fprintf(w, `{"id":42,"node_id":"U_42","login":%q}`, login)
		case "/notifications":
			fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "mention"))
		case "/graphql":
			graphCalls.Add(1)
			fmt.Fprintf(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":%q},"url":"https://github.com/acme/app/pull/7"}}}}`, login)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	newClient := func() *Client {
		return &Client{HTTP: server.Client(), BaseURL: server.URL + "/", GraphQL: server.URL + "/graphql",
			Host: "github.com", cacheRoot: root, identityKey: cacheKey("github.com", "same-token"),
			details: make(map[string]Thread)}
	}
	client := newClient()
	query, _ := ParseQuery("author:@me")
	result, err := client.Fetch(context.Background(), query, 10, nil)
	if err != nil || len(result.Items) != 1 || result.Items[0].Author != "oldname" {
		t.Fatalf("before rename: %+v, %v", result, err)
	}
	renamed.Store(true)
	for _, tc := range []struct {
		name   string
		client *Client
	}{{"same process", client}, {"restart", newClient()}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "restart" {
				if err := tc.client.Initialize(context.Background()); err != nil {
					t.Fatal(err)
				}
				cached := tc.client.Cached(Query{}, 10)
				if len(cached.Items) != 1 || cached.Items[0].Author != "oldname" || cached.Items[0].AuthorID != "U_42" {
					t.Fatalf("restart lost the saved display identity: %+v", cached)
				}
			}
			result, err := tc.client.Fetch(context.Background(), query, 10, nil)
			if err != nil || len(result.Items) != 1 || result.Items[0].AuthorID != "U_42" {
				t.Fatalf("after rename: %+v, %v", result, err)
			}
		})
	}
	if identityCalls.Load() != 1 || graphCalls.Load() != 2 {
		t.Fatalf("stable identity caused redundant requests: user=%d graphql=%d", identityCalls.Load(), graphCalls.Load())
	}
	b, err := os.ReadFile(client.identityPath())
	if err != nil {
		t.Fatal(err)
	}
	var saved identity
	if err := json.Unmarshal(b, &saved); err != nil || saved.ID != 42 || saved.NodeID != "U_42" {
		t.Fatalf("account identity changed: %+v, %v", saved, err)
	}
}

func TestLiteralAuthorAfterAccountRename(t *testing.T) {
	var renamed atomic.Bool
	var graphCalls atomic.Int32
	var authorCalls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/notifications":
			if r.Header.Get("If-None-Match") != "" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"unchanged"`)
			fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "mention"))
		case "/graphql":
			graphCalls.Add(1)
			login := "oldname"
			if renamed.Load() {
				login = "newname"
			}
			fmt.Fprintf(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":%q},"url":"https://github.com/acme/app/pull/7"}}}}`, login)
		case "/users/oldname", "/users/newname":
			authorCalls.Add(1)
			fmt.Fprint(w, `{"node_id":"U_42"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	for _, tc := range []struct {
		query string
		count int
	}{
		{"author:oldname", 1},
		{"author:newname", 1},
		{"author:oldname", 1},
		{"author:oldname author:newname state:open", 1},
	} {
		query, err := ParseQuery(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		original, _ := ParseQuery(tc.query)
		result, err := client.Fetch(ctx, query, 10, nil)
		if err != nil || len(result.Items) != tc.count {
			t.Fatalf("%s: got %+v, %v; want %d items", tc.query, result, err, tc.count)
		}
		before := graphCalls.Load()
		if err := client.Enrich(ctx, result.Items); err != nil {
			t.Fatal(err)
		}
		if graphCalls.Load() != before {
			t.Fatal("display enrichment did not reuse the refreshed author details")
		}
		if !reflect.DeepEqual(query, original) {
			t.Fatal("author resolution mutated the caller's query")
		}
		if len(result.Items) != 0 && result.Items[0].AuthorID != "U_42" {
			t.Fatal("author identity was not preserved in cached details")
		}
		renamed.Store(true)
	}
	if graphCalls.Load() != 1 || authorCalls.Load() != 2 {
		t.Fatalf("expected one detail fetch and one lookup per name: graphql=%d users=%d", graphCalls.Load(), authorCalls.Load())
	}
}

func TestIdentityIsResolvedOncePerCredential(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/user" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		id := 42
		if r.Header.Get("Authorization") == "Bearer other" {
			id = 99
		}
		fmt.Fprintf(w, `{"id":%d,"node_id":"U_%d","login":"octocat"}`, id, id)
	}))
	defer server.Close()
	root := t.TempDir()
	newClient := func(token string) *Client {
		return &Client{HTTP: &http.Client{Transport: identityTransport{token: token, base: server.Client().Transport}},
			BaseURL: server.URL + "/", Host: "github.com", cacheRoot: root, identityKey: cacheKey("github.com", token)}
	}
	first := newClient("first")
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := first.Initialize(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	first.Cache.Put("page", Page{Items: []Thread{{ID: "123"}}, CheckedAt: time.Now()})
	if err := first.Cache.Save(); err != nil {
		t.Fatal(err)
	}
	same := newClient("first")
	if err := same.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("known credential caused another identity request: %d", calls.Load())
	}
	rotated := newClient("rotated")
	if err := rotated.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || rotated.Cache.path != first.Cache.path {
		t.Fatal("rotation did not resolve once and retain the account cache")
	}
	if page, ok := rotated.Cache.Get("page"); !ok || page.Items[0].ID != "123" {
		t.Fatal("rotation lost cached notifications")
	}
	other := newClient("other")
	if err := other.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if other.Cache.path == first.Cache.path {
		t.Fatal("different users share the account cache")
	}
	b, err := os.ReadFile(first.identityPath())
	if err != nil || strings.Contains(string(b), "first") {
		t.Fatalf("identity mapping contains credential data or cannot be read: %v", err)
	}
	query, _ := ParseQuery("author:@me")
	if authors, err := same.resolveAuthors(context.Background(), query); err != nil || authors["@me"] != "U_42" || calls.Load() != 3 {
		t.Fatal("author:@me did not reuse the stable account identity")
	}
}

type identityTransport struct {
	token string
	base  http.RoundTripper
}

func (t identityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

func TestUnverifiedIdentityDoesNotOpenCache(t *testing.T) {
	for _, response := range []string{`{"id":0,"login":"octocat"}`, `{"id":42}`, `{broken`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			c := &Client{HTTP: server.Client(), BaseURL: server.URL + "/", Host: "github.com",
				cacheRoot: t.TempDir(), identityKey: "unknown"}
			if err := c.Initialize(context.Background()); err == nil || c.Cache != nil {
				t.Fatal("unverified identity opened an account cache")
			}
			if len(c.Cached(Query{}, 10).Items) != 0 {
				t.Fatal("unverified identity returned cached data")
			}
		})
	}
}

func TestAuthorResolutionIsConcurrentAndSessionScoped(t *testing.T) {
	var calls atomic.Int32
	var reassigned atomic.Bool
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/someone" {
			t.Errorf("unexpected author lookup: %s", r.URL)
		}
		calls.Add(1)
		id := "U_original"
		if reassigned.Load() {
			id = "U_new_owner"
		}
		fmt.Fprintf(w, `{"node_id":%q}`, id)
	})
	query, _ := ParseQuery("author:Someone author:SOMEONE")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ids, err := client.resolveAuthors(context.Background(), query)
			if err != nil || ids["someone"] != "U_original" {
				t.Errorf("resolve: %v, %v", ids, err)
				return
			}
			ids["someone"] = "caller-change"
		})
	}
	wg.Wait()
	reassigned.Store(true)
	ids, err := client.resolveAuthors(context.Background(), query)
	if err != nil || ids["someone"] != "U_original" || calls.Load() != 1 {
		t.Fatalf("session did not retain its author identity: %v, %v, calls=%d", ids, err, calls.Load())
	}
	restarted := &Client{HTTP: client.HTTP, BaseURL: client.BaseURL}
	ids, err = restarted.resolveAuthors(context.Background(), query)
	if err != nil || ids["someone"] != "U_new_owner" || calls.Load() != 2 {
		t.Fatalf("new client reused a stale name binding: %v, %v, calls=%d", ids, err, calls.Load())
	}
}

func TestAuthorLookupErrorsAreNotCached(t *testing.T) {
	for _, response := range []struct {
		name, body string
		status     int
	}{
		{"missing node ID", `{}`, http.StatusOK},
		{"invalid JSON", `{broken`, http.StatusOK},
		{"unauthorized", `{"message":"Bad credentials"}`, http.StatusUnauthorized},
		{"unknown author", `{"message":"Not Found"}`, http.StatusNotFound},
	} {
		t.Run(response.name, func(t *testing.T) {
			var calls atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/users/someone" {
					t.Errorf("unexpected endpoint: %s", r.URL)
				}

				w.WriteHeader(response.status)
				fmt.Fprint(w, response.body)
			})
			cache := client.Cache
			query, _ := ParseQuery("author:someone")
			for range 2 {
				if authors, err := client.resolveAuthors(context.Background(), query); err == nil || authors != nil {
					t.Fatalf("identity error was hidden: %v, %v", authors, err)
				}
			}
			if client.Cache != cache || len(client.authors) != 0 || calls.Load() != 2 {
				t.Fatal("failed identity lookup changed the account cache or cached a failure")
			}
		})
	}
}
