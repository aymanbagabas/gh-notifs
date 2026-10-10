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

func TestAuthorDetailsPersistAcrossRestartAndRefresh(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotModified} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests, graphCalls atomic.Int32
			var renamed, failDetails, changed atomic.Bool
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path == "/graphql" {
					graphCalls.Add(1)
					if failDetails.Load() {
						http.Error(w, "details unavailable", http.StatusServiceUnavailable)
						return
					}
					name := "oldname"
					if renamed.Load() {
						name = "newname"
					}
					fmt.Fprintf(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":%q},"url":"https://github.com/acme/app/pull/7"}}}}`, name)
					return
				}
				if status == http.StatusNotModified && r.Header.Get("If-None-Match") != "" {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("ETag", `"inbox"`)
				item := threadJSON("1", "PullRequest", "mention")
				if changed.Load() {
					item = strings.ReplaceAll(item, "2026-10-09T12:00:00Z", "2026-10-10T12:00:00Z")
				}
				fmt.Fprintf(w, "[%s]", item)
			})
			root := filepath.Dir(filepath.Dir(filepath.Dir(client.Cache.path)))
			restart := func() *Client {
				t.Helper()
				cache, err := OpenCache(root, "github.com", "test-account")
				if err != nil {
					t.Fatal(err)
				}
				return &Client{HTTP: client.HTTP, BaseURL: client.BaseURL, GraphQL: client.GraphQL,
					Host: client.Host, Cache: cache}
			}
			check := func(result Result, name string) {
				t.Helper()
				if len(result.Items) != 1 || result.Items[0].Author != name || result.Items[0].AuthorID != "U_42" ||
					result.Items[0].State != "open" || result.Items[0].URL != "https://github.com/acme/app/pull/7" {
					t.Fatalf("want cached details for %s, got %+v", name, result)
				}
			}
			query, _ := ParseQuery("")
			ctx := context.Background()
			result, err := client.Fetch(ctx, query, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Enrich(ctx, result.Items); err != nil {
				t.Fatal(err)
			}
			client = restart()
			before := requests.Load()
			check(client.Cached(query, 10), "oldname")
			if requests.Load() != before {
				t.Fatal("cached startup made a network request")
			}
			renamed.Store(true)
			failDetails.Store(true)
			result, err = client.Fetch(ctx, query, 10, func(progress Result) { check(progress, "oldname") })
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Enrich(ctx, result.Items); err == nil {
				t.Fatal("detail fetch failure was not reported")
			}
			check(result, "oldname")
			check(restart().Cached(query, 10), "oldname")
			failDetails.Store(false)
			if err := client.Enrich(ctx, result.Items); err != nil {
				t.Fatal(err)
			}
			check(result, "newname")
			check(restart().Cached(query, 10), "newname")
			result, err = client.Fetch(ctx, query, 10, func(progress Result) { check(progress, "newname") })
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Enrich(ctx, result.Items); err != nil {
				t.Fatal(err)
			}
			if graphCalls.Load() != 3 {
				t.Fatalf("want initial, failed, and successful detail requests; got %d", graphCalls.Load())
			}
			if status == http.StatusOK {
				changed.Store(true)
				result, err = client.Fetch(ctx, query, 10, func(progress Result) {
					if len(progress.Items) != 1 || progress.Items[0].Author != "" || progress.Items[0].State != "" {
						t.Fatalf("changed notification reused old details: %+v", progress)
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := client.Enrich(ctx, result.Items); err != nil {
					t.Fatal(err)
				}
				if graphCalls.Load() != 4 {
					t.Fatal("changed notification did not request fresh details")
				}
			}
		})
	}
}

func TestInflightEnrichmentDoesNotRestoreInvalidatedCache(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/graphql" {
			close(started)
			<-release
			fmt.Fprint(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":"octocat"}}}}}`)
			return
		}
		fmt.Fprintf(w, "[%s]", threadJSON("1", "PullRequest", "mention"))
	})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	query, _ := ParseQuery("")
	result, err := client.Fetch(context.Background(), query, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- client.Enrich(context.Background(), result.Items) }()
	<-started
	root := filepath.Dir(filepath.Dir(filepath.Dir(client.Cache.path)))
	other, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Invalidate(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get(Endpoint(query, 1)); ok {
		t.Fatal("the late GraphQL response restored the invalidated page")
	}
}

func TestCancelledEnrichmentDoesNotSaveDetails(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("cancelled enrichment made a network request")
	})
	query, _ := ParseQuery("")
	item := Thread{ID: "1", Repository: "acme/app", Type: "Issue", Number: 7,
		Author: "oldname", AuthorID: "U_42", State: "open"}
	client.Cache.Put(Endpoint(query, 1), Page{Items: []Thread{item}, CheckedAt: time.Now()})
	if err := client.Cache.Save(); err != nil {
		t.Fatal(err)
	}
	detail := item
	detail.Author = "newname"
	client.details[detailKey(item)] = detail
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Enrich(ctx, []Thread{item}); err != context.Canceled {
		t.Fatalf("cancellation was not reported: %v", err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(client.Cache.path)))
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	page, ok := reopened.Get(Endpoint(query, 1))
	if !ok || page.Items[0].Author != "oldname" {
		t.Fatal("cancelled enrichment changed the persisted author")
	}
}

func TestLateDetailsAfterAnotherClientsRefresh(t *testing.T) {
	for _, change := range []string{"removed", "updated"} {
		t.Run(change, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var refreshed atomic.Bool
			first := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/graphql" {
					close(started)
					<-release
					fmt.Fprint(w, `{"data":{"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_42","login":"octocat"}}}}}`)
					return
				}
				item := threadJSON("1", "PullRequest", "mention")
				if refreshed.Load() {
					if change == "removed" {
						fmt.Fprint(w, "[]")
						return
					}
					item = strings.NewReplacer("2026-10-09T12:00:00Z", "2026-10-10T12:00:00Z",
						`"unread":true`, `"unread":false`, "Fix uploads", "Updated title").Replace(item)
					w.Header().Set("ETag", `"updated"`)
				}
				fmt.Fprintf(w, "[%s]", item)
			})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			query := Query{}
			result, err := first.Fetch(context.Background(), query, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- first.Enrich(context.Background(), result.Items) }()
			<-started
			root := filepath.Dir(filepath.Dir(filepath.Dir(first.Cache.path)))
			cache, err := OpenCache(root, "github.com", "test-account")
			if err != nil {
				t.Fatal(err)
			}
			second := &Client{HTTP: first.HTTP, BaseURL: first.BaseURL, Host: first.Host, Cache: cache}
			refreshed.Store(true)
			if _, err := second.Fetch(context.Background(), query, 10, nil); err != nil {
				t.Fatal(err)
			}
			saved, err := second.Cache.readPages()
			if err != nil {
				t.Fatal(err)
			}
			want := saved.Pages[Endpoint(query, 1)]
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			for _, saveAgain := range []bool{false, true} {
				if saveAgain {
					if err := first.Cache.Save(); err != nil {
						t.Fatal(err)
					}
				}
				reopened, err := OpenCache(root, "github.com", "test-account")
				if err != nil {
					t.Fatal(err)
				}
				got, ok := reopened.Get(Endpoint(query, 1))
				if !ok || !reflect.DeepEqual(got, want) {
					t.Fatalf("saveAgain=%t: late enrichment replaced the newer page:\ngot %+v\nwant %+v", saveAgain, got, want)
				}
			}
		})
	}
}

func TestCachedDetailsFollowReorderedPages(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		id := "1"
		if r.URL.Query().Get("page") == "1" {
			id = "2"
			w.Header().Set("Link", `<https://api.github.com/notifications?page=2>; rel="next"`)
		}
		item := strings.ReplaceAll(threadJSON(id, "PullRequest", "mention"), "/pulls/7", "/pulls/"+id)
		fmt.Fprintf(w, "[%s]", item)
	})
	query, _ := ParseQuery("")
	for i := 1; i <= 2; i++ {
		item := Thread{ID: fmt.Sprint(i), Repository: "acme/app", Type: "PullRequest", Number: i,
			SubjectURL: fmt.Sprintf("https://api.github.com/repos/acme/app/pulls/%d", i),
			UpdatedAt:  time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
			Author:     fmt.Sprintf("author%d", i), AuthorID: fmt.Sprintf("U_%d", i), State: "open"}
		client.Cache.Put(Endpoint(query, i), Page{Items: []Thread{item}, Next: i == 1, CheckedAt: time.Now()})
	}
	result, err := client.Fetch(context.Background(), query, 2, func(progress Result) {
		for _, item := range progress.Items {
			if item.Author != "author"+item.ID || item.AuthorID != "U_"+item.ID {
				t.Errorf("page reorder lost known details: %+v", item)
			}
		}
	})
	if err != nil || len(result.Items) != 2 {
		t.Fatalf("reordered fetch: %+v, %v", result, err)
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

func TestFetchRefreshesAnotherClientsInvalidation(t *testing.T) {
	var acted atomic.Bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			acted.Store(true)
			w.WriteHeader(http.StatusResetContent)
			return
		}
		if acted.Load() && (r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "") {
			t.Error("fetch reused validators from another client's invalidated page")
		}
		w.Header().Set("ETag", `"inbox"`)
		raw := threadJSON("42", "Release", "subscribed")
		if acted.Load() {
			raw = strings.ReplaceAll(raw, `"unread":true`, `"unread":false`)
		}
		fmt.Fprintf(w, "[%s]", raw)
	})
	query, _ := ParseQuery("")
	if _, err := c.Fetch(context.Background(), query, 10, nil); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(c.Cache.path)))
	cache, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	action := &Client{HTTP: c.HTTP, BaseURL: c.BaseURL, Cache: cache}
	if err := action.Action(context.Background(), "42", "read"); err != nil {
		t.Fatal(err)
	}
	result, err := c.Fetch(context.Background(), query, 10, nil)
	if err != nil || len(result.Items) != 1 || result.Items[0].Unread {
		t.Fatalf("fetch did not refresh the invalidated page: %+v, %v", result, err)
	}
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	page, ok := reopened.Get(Endpoint(query, 1))
	if !ok || len(page.Items) != 1 || page.Items[0].Unread {
		t.Fatalf("fetch did not persist the post-action page: %+v", page)
	}
}

func TestOtherClientsActionInvalidatesInflightFetch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusResetContent)
			return
		}
		close(started)
		<-release
		fmt.Fprintf(w, "[%s]", threadJSON("42", "Release", "subscribed"))
	})
	root := filepath.Dir(filepath.Dir(filepath.Dir(c.Cache.path)))
	cache, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	action := &Client{HTTP: c.HTTP, BaseURL: c.BaseURL, Cache: cache}
	query, _ := ParseQuery("")
	finished := make(chan error, 1)
	go func() {
		_, err := c.Fetch(context.Background(), query, 10, nil)
		finished <- err
	}()
	<-started
	err = action.Action(context.Background(), "42", "read")
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get(Endpoint(query, 1)); ok {
		t.Fatal("inflight fetch restored another client's invalidated page")
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

func TestFetchFractionalDateBounds(t *testing.T) {
	for _, inclusive := range []bool{false, true} {
		t.Run(fmt.Sprintf("inclusive=%t", inclusive), func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				before, err := time.Parse(time.RFC3339, r.URL.Query().Get("before"))
				if err != nil || before.Nanosecond() != 0 {
					t.Errorf("REST before bound must use whole seconds: %s", r.URL)
				}
				since, err := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
				if err != nil || since.Nanosecond() != 0 {
					t.Errorf("REST since bound must use whole seconds: %s", r.URL)
				}
				var items []string
				for i, timestamp := range []string{
					"2026-10-09T11:59:59Z", "2026-10-09T12:00:00Z", "2026-10-09T12:00:01Z",
				} {
					updated, _ := time.Parse(time.RFC3339, timestamp)
					matches := updated.After(since) && updated.Before(before)
					if inclusive {
						matches = !updated.Before(since) && !updated.After(before)
					}
					if matches {
						items = append(items, strings.ReplaceAll(threadJSON(fmt.Sprint(i), "Release", "subscribed"),
							"2026-10-09T12:00:00Z", timestamp))
					}
				}
				fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
			})
			query, err := ParseQuery("since:2026-10-09T11:59:59.500Z before:2026-10-09T12:00:00.500Z")
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Fetch(context.Background(), query, 10, nil)
			if err != nil || len(result.Items) != 1 || result.Items[0].ID != "1" {
				t.Fatalf("fractional bounds lost a locally matching notification: %+v, %v", result, err)
			}
			cached := c.Cached(query, 10)
			if !reflect.DeepEqual(cached.Items, result.Items) {
				t.Fatalf("cached fractional bounds differ: %+v", cached)
			}
		})
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
	q, err := ParseQuery("repo:acme/app repo:acme/other repo:ACME/APP is:issue-or-pull-request is:discussion is:unread")
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

func TestDistinctTypeFiltersAndCache(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "[%s,%s,%s]", threadJSON("1", "Issue", "mention"),
			threadJSON("2", "PullRequest", "mention"), threadJSON("3", "Release", "subscribed"))
	})
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{"is:issue", []string{"1"}},
		{"is:pr", []string{"2"}},
		{"is:pull-request", []string{"2"}},
		{"is:issue-or-pull-request", []string{"1", "2"}},
		{"is:issue is:pr is:unread", []string{"1", "2"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Fetch(context.Background(), q, 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, item := range result.Items {
				ids = append(ids, item.ID)
			}
			if !slices.Equal(ids, tc.ids) || result.More {
				t.Fatalf("type filter returned %v, want %v", ids, tc.ids)
			}
			if cached := c.Cached(q, 10); !reflect.DeepEqual(cached.Items, result.Items) {
				t.Fatalf("cached type filter differs: %+v", cached)
			}
		})
	}
}

func TestFetchAuthorStateAndReadAlternatives(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"login":"octocat","node_id":"U_octocat"}`)
		case "/users/someone":
			fmt.Fprint(w, `{"login":"someone","node_id":"U_someone"}`)
		case "/notifications":
			if r.URL.Query().Get("all") != "true" {
				t.Error("read/unread alternatives excluded read notifications")
			}
			read := strings.ReplaceAll(threadJSON("2", "Issue", "subscribed"), `"unread":true`, `"unread":false`)
			read = strings.ReplaceAll(read, "/pulls/7", "/issues/8")
			fmt.Fprintf(w, "[%s,%s]", threadJSON("1", "PullRequest", "mention"), read)
		case "/graphql":
			fmt.Fprint(w, `{"data":{
				"v0":{"issueOrPullRequest":{"state":"OPEN","author":{"id":"U_octocat","login":"octocat"}}},
				"v1":{"issueOrPullRequest":{"state":"CLOSED","author":{"id":"U_someone","login":"someone"}}}
			}}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	q, err := ParseQuery("author:@ME author:someone state:open state:closed is:issue-or-pull-request is:read is:unread")
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
			participating, err := ParseQuery(scope + "reason:mention is:unread is:issue-or-pull-request")
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
	q, err := ParseQuery("is:issue-or-pull-request")
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
