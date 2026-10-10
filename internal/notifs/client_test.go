package notifs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	for _, tc := range []struct {
		query, path, all, perPage, participating string
	}{
		{"", "notifications", "true", "50", ""},
		{"reason:participating", "notifications", "true", "50", "true"},
		{"reason:author", "notifications", "true", "50", "true"},
		{"reason:comment", "notifications", "true", "50", "true"},
		{"reason:MENTION", "notifications", "true", "50", "true"},
		{"reason:subscribed", "notifications", "true", "50", ""},
		{"reason:review-requested", "notifications", "true", "50", ""},
		{"reason:mention reason:comment", "notifications", "true", "50", "true"},
		{"reason:mention reason:subscribed", "notifications", "true", "50", ""},
		{"is:read is:unread", "notifications", "true", "50", ""},
		{"is:unread is:read", "notifications", "true", "50", ""},
		{"author:@me", "notifications", "true", "50", ""},
		{"org:acme is:unread", "notifications", "false", "50", ""},
		{"repo:acme/app", "repos/acme/app/notifications", "true", "100", ""},
		{"repo:acme/app reason:mention is:unread", "repos/acme/app/notifications", "false", "100", "true"},
		{"reason:participating is:read repo:acme/app", "repos/acme/app/notifications", "true", "100", "true"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query + " since:2026-10-01 before:2026-11-01")
			if err != nil {
				t.Fatal(err)
			}
			endpoint, err := url.Parse(Endpoint(q, 2))
			if err != nil {
				t.Fatal(err)
			}
			if endpoint.Path != tc.path {
				t.Fatalf("path: %s, want %s", endpoint.Path, tc.path)
			}
			for key, want := range map[string]string{
				"all": tc.all, "per_page": tc.perPage, "participating": tc.participating,
				"page": "2", "since": "2026-10-01T00:00:00Z", "before": "2026-11-01T00:00:00Z",
			} {
				if got := endpoint.Query().Get(key); got != want {
					t.Errorf("%s: %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestEndpointUsesStrictestDates(t *testing.T) {
	q, err := ParseQuery("since:2026-10-02T01:00:00+01:00 since:2026-10-01 before:2026-10-09 before:2026-10-10")
	if err != nil {
		t.Fatal(err)
	}
	want := Endpoint(q, 1)
	slices.Reverse(q.Terms)
	if Endpoint(q, 1) != want {
		t.Fatal("date order changed the endpoint")
	}
	u, err := url.Parse(want)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("since") != "2026-10-02T00:00:00Z" || u.Query().Get("before") != "2026-10-09T00:00:00Z" {
		t.Fatalf("wrong bounds: %s", want)
	}
}

func TestConcurrentRepositoriesMergeAndCache(t *testing.T) {
	bothStarted := make(chan struct{})
	var started, calls, callbacks atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/notifications")
		if repo != "acme/app" && repo != "acme/other" {
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.Error(w, "wrong endpoint", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Error("repository endpoint must use per_page=100")
		}
		if r.URL.Query().Get("page") == "1" {
			if started.Add(1) == 2 {
				close(bothStarted)
			}
			select {
			case <-bothStarted:
			case <-r.Context().Done():
				return
			}
		}
		item := func(id, kind, date string) string {
			raw := strings.ReplaceAll(threadJSON(id, kind, "subscribed"), "acme/app", repo)
			return strings.ReplaceAll(raw, "2026-10-09", date)
		}
		if repo == "acme/app" && r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `</?page=2>; rel="next"`)
			fmt.Fprintf(w, "[%s]", item("skip", "Release", "2026-10-11"))
		} else if repo == "acme/app" {
			fmt.Fprintf(w, "[%s]", item("old", "PullRequest", "2026-10-08"))
		} else {
			w.Header().Set("Link", `</?page=2>; rel="next"`)
			fmt.Fprintf(w, "[%s,%s,%s]", item("new1", "Issue", "2026-10-10"), item("new1", "Issue", "2026-10-10"), item("new2", "PullRequest", "2026-10-09"))
		}
	})
	q, err := ParseQuery("repo:acme/app repo:acme/other repo:ACME/APP is:pr is:issue is:unread")
	if err != nil {
		t.Fatal(err)
	}
	original := Query{Raw: q.Raw, Terms: slices.Clone(q.Terms)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var snapshots []Result
	result, err := c.Fetch(ctx, q, 2, func(result Result) {
		if callbacks.Add(1) != 1 {
			t.Error("progress callbacks overlap")
		}
		defer callbacks.Add(-1)
		if len(result.Items) > 2 {
			t.Error("progress exceeded the global limit")
		}
		snapshots = append(snapshots, result)
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || result.Pages != 3 || !result.More || len(result.Items) != 2 ||
		result.Items[0].ID != "new1" || result.Items[1].ID != "new2" || result.CheckedAt.IsZero() || result.FromCache {
		t.Fatalf("incorrect merged result: %+v (%d requests)", result, calls.Load())
	}
	cached := c.Cached(q, 2)
	if !reflect.DeepEqual(cached.Items, result.Items) || cached.Pages != result.Pages || cached.More != result.More ||
		!cached.FromCache || cached.CheckedAt.IsZero() {
		t.Fatalf("cache differs: %+v", cached)
	}
	if !reflect.DeepEqual(q, original) || len(snapshots) == 0 {
		t.Fatal("query mutated or progress missing")
	}
	result.Items[0].Title = "changed"
	if snapshots[len(snapshots)-1].Items[0].Title == "changed" {
		t.Fatal("progress shares result storage")
	}
}

func TestMixedParticipationRequests(t *testing.T) {
	for _, scope := range []string{"", "repo:acme/app repo:acme/other "} {
		t.Run(scope, func(t *testing.T) {
			var calls atomic.Int32
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				repo := "acme/app"
				if strings.Contains(r.URL.Path, "/acme/other/") {
					repo = "acme/other"
				}
				item := func(id, reason string) string {
					return strings.ReplaceAll(threadJSON(repo+id, "PullRequest", reason), "acme/app", repo)
				}
				if r.URL.Query().Get("participating") == "true" {
					fmt.Fprintf(w, "[%s,%s]", item("1", "state_change"), item("3", "mention"))
				} else {
					fmt.Fprintf(w, "[%s,%s,%s,%s]", item("1", "state_change"), item("2", "subscribed"), item("3", "mention"), item("4", "review_requested"))
				}
			})
			q, err := ParseQuery(scope + "reason:participating reason:subscribed reason:mention")
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Fetch(context.Background(), q, 20, nil)
			want := 3
			if scope != "" {
				want = 6
			}
			if err != nil || len(result.Items) != want || result.More || int(calls.Load()) != want/3*2 {
				t.Fatalf("mixed participation: %+v, %v (%d requests)", result, err, calls.Load())
			}
			for _, item := range result.Items {
				if item.Reason == "review_requested" {
					t.Fatal("unrestricted response was treated as participation evidence")
				}
			}
			root := filepath.Dir(filepath.Dir(filepath.Dir(c.Cache.path)))
			cache, err := OpenCache(root, "github.com", "test-account")
			if err != nil {
				t.Fatal(err)
			}
			c.Cache = cache
			cached := c.Cached(q, 20)
			if !reflect.DeepEqual(cached.Items, result.Items) || cached.More || cached.Pages != result.Pages {
				t.Fatalf("mixed participation cache differs: %+v", cached)
			}
		})
	}
}

func TestConcurrentRequestFailureCancelsOthers(t *testing.T) {
	bothStarted := make(chan struct{})
	canceled := make(chan struct{})
	var started atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if started.Add(1) == 2 {
			close(bothStarted)
		}
		select {
		case <-bothStarted:
		case <-r.Context().Done():
			return
		}
		if strings.Contains(r.URL.Path, "/acme/app/") {
			http.Error(w, `{"message":"repository request failed"}`, http.StatusInternalServerError)
		} else {
			<-r.Context().Done()
			close(canceled)
		}
	})
	q, _ := ParseQuery("repo:acme/app repo:acme/other")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Fetch(ctx, q, 10, nil); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("repository failure was lost: %v", err)
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("other repository request did not stop")
	}
}

func TestFetchAuthorStateAndReadAlternatives(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"login":"octocat"}`)
		case "/notifications":
			if r.URL.Query().Get("all") != "true" {
				t.Error("read/unread alternatives excluded read notifications")
			}
			read := strings.ReplaceAll(threadJSON("2", "Issue", "subscribed"), `"unread":true`, `"unread":false`)
			read = strings.ReplaceAll(read, "/pulls/7", "/issues/8")
			fmt.Fprintf(w, "[%s,%s]", threadJSON("1", "PullRequest", "mention"), read)
		case "/graphql":
			fmt.Fprint(w, `{"data":{
				"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"login":"octocat"}}},
				"v1":{"issueOrPullRequest":{"state":"CLOSED","author":{"login":"someone"}}}
			}}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	q, err := ParseQuery("author:@ME author:someone state:open state:closed is:issue is:pr is:read is:unread")
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Fetch(context.Background(), q, 10, nil)
	if err != nil || len(result.Items) != 2 {
		t.Fatalf("detail alternatives: %+v, %v", result, err)
	}
	if result.Items[0].Author != "octocat" || result.Items[0].State != "open" ||
		result.Items[1].Author != "someone" || result.Items[1].State != "closed" || result.Items[1].Unread {
		t.Fatalf("incorrect details: %+v", result.Items)
	}
}

func TestCachedRepositoriesReportMissingBranch(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("cache made a request") })
	q, _ := ParseQuery("repo:acme/app repo:acme/other")
	c.Cache.Put(Endpoint(q.Requests()[0], 1), Page{
		Items: []Thread{{ID: "1", Repository: "acme/app"}}, CheckedAt: time.Now(),
	})
	result := c.Cached(q, 10)
	if len(result.Items) != 1 || !result.More || !result.FromCache || result.Pages != 1 {
		t.Fatalf("missing branch was treated as complete: %+v", result)
	}
}

func TestParticipationFetchAndCache(t *testing.T) {
	for _, scope := range []string{"", "repo:acme/app "} {
		t.Run(scope, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("participating") == "true" {
					fmt.Fprintf(w, "[%s,%s]", threadJSON("2", "PullRequest", "mention"), threadJSON("3", "PullRequest", "comment"))
				} else {
					fmt.Fprintf(w, "[%s]", threadJSON("1", "Release", "subscribed"))
				}
			})
			all, err := ParseQuery(scope)
			if err != nil {
				t.Fatal(err)
			}
			participating, err := ParseQuery(scope + "reason:mention is:unread is:pr")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Fetch(context.Background(), all, 10, nil); err != nil {
				t.Fatal(err)
			}
			if got := c.Cached(participating, 10); len(got.Items) != 0 {
				t.Fatal("participating query reused an unrestricted cached page")
			}
			result, err := c.Fetch(context.Background(), participating, 10, nil)
			if err != nil || len(result.Items) != 1 || result.Items[0].ID != "2" {
				t.Fatalf("participating fetch: %+v, %v", result, err)
			}
			for _, tc := range []struct {
				query Query
				id    string
			}{{all, "1"}, {participating, "2"}} {
				cached := c.Cached(tc.query, 10)
				if len(cached.Items) != 1 || cached.Items[0].ID != tc.id {
					t.Fatalf("incorrect cached results: %+v", cached)
				}
			}
			aggregate, err := ParseQuery(scope + "reason:participating is:unread")
			if err != nil {
				t.Fatal(err)
			}
			if cached := c.Cached(aggregate, 10); len(cached.Items) != 2 {
				t.Fatalf("aggregate reason rejected the server's participation results: %+v", cached)
			}
		})
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

func TestRepositoryHistoryIncludesFilteredOutNotifications(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "[%s]", threadJSON("1", "Release", "subscribed"))
	})
	q, err := ParseQuery("is:pr")
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Fetch(context.Background(), q, 10, nil)
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("filtered fetch: %+v, %v", result, err)
	}
	if repos := c.RecentRepositories(); len(repos) != 1 || repos[0].Name != "acme/app" {
		t.Fatalf("filtered-out notification did not update history: %+v", repos)
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
