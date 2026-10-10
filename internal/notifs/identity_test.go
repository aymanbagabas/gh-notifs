package notifs

import (
	"context"
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
	if login, err := same.Login(context.Background()); err != nil || login != "octocat" || calls.Load() != 3 {
		t.Fatal("cached identity did not supply the login")
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
