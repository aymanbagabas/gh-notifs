package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cache, err := OpenCache(t.TempDir(), "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	return &Client{HTTP: server.Client(), BaseURL: server.URL + "/", GraphQL: server.URL + "/graphql",
		Host: "github.com", Cache: cache, details: make(map[string]Thread)}
}

func threadJSON(id, kind, reason string) string {
	return fmt.Sprintf(`{"id":%q,"unread":true,"reason":%q,"updated_at":"2026-10-09T12:00:00Z",
"last_read_at":null,"repository":{"full_name":"acme/app"},
"subject":{"type":%q,"title":"Fix uploads","url":"https://api.github.com/repos/acme/app/pulls/7"}}`, id, reason, kind)
}

func TestConditionalRefresh(t *testing.T) {
	var requests atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == `"inbox"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"inbox"`)
		fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "review_requested"))
	})
	q, _ := ParseQuery("")
	for range 2 {
		result, err := c.Fetch(context.Background(), q, 50, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 1 || result.Items[0].Number != 7 || result.More {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if requests.Load() != 2 {
		t.Fatal("unexpected request count")
	}
	cached := c.Cached(q, 50)
	if len(cached.Items) != 1 || !cached.FromCache {
		t.Fatalf("missing cache: %+v", cached)
	}
	info, err := os.Stat(c.Cache.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v, %v", info, err)
	}
}

func TestFilterScansPagesAndReconciles(t *testing.T) {
	var empty atomic.Bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if empty.Load() {
			fmt.Fprint(w, "[]")
			return
		}
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<https://api.github.com/notifications?page=2>; rel="next"`)
			fmt.Fprintf(w, "[%s]", threadJSON("1", "Release", "subscribed"))
		} else {
			fmt.Fprintf(w, "[%s]", threadJSON("2", "PullRequest", "review_requested"))
		}
	})
	q, _ := ParseQuery("reason:review-requested")
	result, err := c.Fetch(context.Background(), q, 1, nil)
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != "2" || result.Pages != 2 {
		t.Fatalf("filtered pages: %+v, %v", result, err)
	}
	empty.Store(true)
	result, err = c.Fetch(context.Background(), q, 1, nil)
	if err != nil || len(result.Items) != 0 || len(c.Cached(q, 1).Items) != 0 {
		t.Fatalf("removed item retained: %+v, %v", result, err)
	}
}

func TestRateLimitAndCancellation(t *testing.T) {
	var requests atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"rate limit"}`)
	})
	q, _ := ParseQuery("")
	for range 2 {
		if _, err := c.Fetch(context.Background(), q, 1, nil); err == nil {
			t.Fatal("rate limit not reported")
		}
	}
	if requests.Load() != 1 {
		t.Fatal("requested again during rate limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Fetch(ctx, q, 1, nil); err == nil {
		t.Fatal("cancellation not reported")
	}
}

func TestGraphQLDetailsAreBatchedAndReused(t *testing.T) {
	var requests atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var payload struct {
			Query     string
			Variables map[string]any
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if !strings.Contains(payload.Query, "issueOrPullRequest") || payload.Variables["n0"] != float64(7) {
			t.Errorf("incorrect GraphQL request: %+v", payload)
		}
		fmt.Fprint(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","isDraft":true,"author":{"login":"octocat"},"url":"https://github.com/acme/app/pull/7"}}}}`)
	})
	items := []Thread{{ID: "1", Repository: "acme/app", Type: "PullRequest", Number: 7, Unread: true}}
	for range 2 {
		if err := c.Enrich(context.Background(), items); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 1 || items[0].Author != "octocat" || items[0].State != "draft" || !items[0].Unread {
		t.Fatalf("incorrect enrichment: %+v, %d requests", items, requests.Load())
	}
}

func TestActionsValidateAndInvalidate(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/notifications/threads/42" {
			t.Errorf("incorrect action: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusResetContent)
	})
	c.Cache.Put("test", Page{CheckedAt: time.Now(), Items: []Thread{{ID: "42"}}})
	if err := c.Action(context.Background(), "../other", "read"); err == nil {
		t.Fatal("accepted invalid thread ID")
	}
	if err := c.Action(context.Background(), "42", "read"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Cache.Get("test"); ok {
		t.Fatal("action left stale cache")
	}
}

func TestCacheAccountIsolation(t *testing.T) {
	root := t.TempDir()
	a, _ := OpenCache(root, "github.com", "account-a")
	b, _ := OpenCache(root, "github.com", "account-b")
	c, _ := OpenCache(root, "enterprise.example", "account-a")
	if a.path == b.path || a.path == c.path {
		t.Fatal("accounts or hosts share a cache")
	}
}

func TestActionInvalidatesInflightPage(t *testing.T) {
	cache, err := OpenCache(t.TempDir(), "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	generation := cache.Generation()
	if err := cache.Invalidate(); err != nil {
		t.Fatal(err)
	}
	cache.PutIfCurrent("page", Page{Items: []Thread{{ID: "stale"}}}, generation)
	if _, ok := cache.Get("page"); ok {
		t.Fatal("inflight request restored invalidated data")
	}
}

func TestRejectsForeignSubjectURL(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	if _, err := c.Preview(context.Background(), Thread{SubjectURL: "https://other.example/private"}); err == nil {
		t.Fatal("accepted another API host")
	}
}

func TestEndpoint(t *testing.T) {
	q, _ := ParseQuery("repo:acme/app is:unread since:2026-10-01")
	endpoint := Endpoint(q, 2)
	if !strings.HasPrefix(endpoint, "repos/acme/app/notifications?") ||
		!strings.Contains(endpoint, "all=false") || !strings.Contains(endpoint, "page=2") {
		t.Fatal(endpoint)
	}
}

func TestProgressArrivesBeforeDetails(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			t.Error("Fetch should not request optional details before exposing rows")
		}
		fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "review_requested"))
	})
	q, _ := ParseQuery("")
	called := false
	_, err := c.Fetch(context.Background(), q, 10, func(result Result) {
		called = true
		if len(result.Items) != 1 {
			t.Errorf("progress has no rows: %+v", result)
		}
	})
	if err != nil || !called {
		t.Fatalf("progress not delivered: %v", err)
	}
}

func BenchmarkCachedInbox(b *testing.B) {
	cache, err := OpenCache(b.TempDir(), "github.com", "account")
	if err != nil {
		b.Fatal(err)
	}
	c := &Client{Cache: cache}
	q, _ := ParseQuery("")
	items := make([]Thread, 50)
	for i := range items {
		items[i] = Thread{ID: fmt.Sprint(i), Title: "Fix slow uploads", Unread: true, UpdatedAt: time.Now()}
	}
	cache.Put(Endpoint(q, 1), Page{Items: items, CheckedAt: time.Now()})
	for b.Loop() {
		c.Cached(q, 50)
	}
}
