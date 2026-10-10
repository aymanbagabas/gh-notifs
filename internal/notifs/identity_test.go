package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthorMeAfterAccountRename(t *testing.T) {
	var renamed atomic.Bool
	var identityCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login := "oldname"
		if renamed.Load() {
			login = "newname"
		}
		switch r.URL.Path {
		case "/user":
			identityCalls.Add(1)
			fmt.Fprintf(w, `{"id":42,"login":%q}`, login)
		case "/notifications":
			fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "mention"))
		case "/graphql":
			fmt.Fprintf(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"login":%q},"url":"https://github.com/acme/app/pull/7"}}}}`, login)
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
			result, err := tc.client.Fetch(context.Background(), query, 10, nil)
			if err != nil || len(result.Items) != 1 || result.Items[0].Author != "newname" {
				t.Fatalf("after rename: %+v, %v", result, err)
			}
		})
	}
	if identityCalls.Load() < 2 {
		t.Fatal("current account was never checked after the rename")
	}
	b, err := os.ReadFile(client.identityPath())
	if err != nil {
		t.Fatal(err)
	}
	var saved identity
	if err := json.Unmarshal(b, &saved); err != nil || saved.ID != 42 {
		t.Fatalf("account identity changed: %+v, %v", saved, err)
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
		fmt.Fprintf(w, `{"id":%d,"login":"octocat"}`, id)
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
	if login, err := same.Login(context.Background()); err != nil || login != "octocat" || calls.Load() != 4 {
		t.Fatal("login did not check the current account name")
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

func TestLoginDoesNotFallBackToCachedName(t *testing.T) {
	for _, response := range []struct {
		name, body string
		status     int
	}{
		{"missing login", `{}`, http.StatusOK},
		{"invalid JSON", `{broken`, http.StatusOK},
		{"unauthorized", `{"message":"Bad credentials"}`, http.StatusUnauthorized},
	} {
		t.Run(response.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/user" {
					t.Errorf("unexpected endpoint: %s", r.URL)
				}
				w.WriteHeader(response.status)
				fmt.Fprint(w, response.body)
			})
			client.login = "oldname"
			cache := client.Cache
			if login, err := client.Login(context.Background()); err == nil || login != "" {
				t.Fatalf("identity error used a cached name: %q, %v", login, err)
			}
			if client.Cache != cache || client.login != "oldname" {
				t.Fatal("failed identity lookup changed the account cache or login")
			}
		})
	}
}
