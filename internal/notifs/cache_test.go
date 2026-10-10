package notifs

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCachedDetailsUpdateOnlyMatchingFields(t *testing.T) {
	cache, err := OpenCache(t.TempDir(), "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	item := Thread{ID: "1", Repository: "acme/app", Number: 7, Type: "PullRequest",
		UpdatedAt: time.Now(), Title: "Current title", Unread: true, Reason: "mention"}
	other := item
	other.UpdatedAt = item.UpdatedAt.Add(time.Hour)
	page := Page{Items: []Thread{item, other}, ETag: `"inbox"`, LastModified: "unchanged",
		Next: true, CheckedAt: time.Now()}
	cache.Put("page", page)
	cache.Put("another-query", page)
	detail := item
	detail.Author, detail.AuthorID, detail.State, detail.URL = "octocat", "U_42", "open", "https://github.com/acme/app/pull/7"
	detail.Title, detail.Unread, detail.Reason = "Old title", false, "subscribed"
	if err := cache.storeDetails([]Thread{detail}, cache.Generation()); err != nil {
		t.Fatal(err)
	}
	want := page
	want.Items = []Thread{item, other}
	want.Items[0].copyDetails(detail)
	for _, key := range []string{"page", "another-query"} {
		got, ok := cache.Get(key)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("detail write changed unrelated fields: %+v", got)
		}
	}
	if page.Items[0].Author != "" {
		t.Fatal("detail write mutated the caller's page")
	}
	snapshot := cache.detailSnapshot()
	delete(snapshot, detailKey(item))
	if len(cache.detailSnapshot()) != 1 {
		t.Fatal("detail snapshot shares mutable cache state")
	}
	if err := os.WriteFile(cache.path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.storeDetails([]Thread{detail}, cache.Generation()); err == nil {
		t.Fatal("detail write hid a cache read error")
	}
}

func TestLateDetailsCannotRestoreInvalidatedPages(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprintf("external=%t", external), func(t *testing.T) {
			root := t.TempDir()
			cache, err := OpenCache(root, "github.com", "account")
			if err != nil {
				t.Fatal(err)
			}
			item := Thread{ID: "1", Repository: "acme/app", Number: 7, Type: "PullRequest",
				UpdatedAt: time.Now(), Author: "oldname", AuthorID: "U_42", State: "open"}
			cache.Put("page", Page{Items: []Thread{item}, CheckedAt: time.Now()})
			if err := cache.Save(); err != nil {
				t.Fatal(err)
			}
			generation := cache.Generation()
			action := cache
			if external {
				action, err = OpenCache(root, "github.com", "account")
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := action.Invalidate(); err != nil {
				t.Fatal(err)
			}
			current := item
			current.Author = "current"
			action.Put("new-page", Page{Items: []Thread{current}, CheckedAt: time.Now()})
			if err := action.Save(); err != nil {
				t.Fatal(err)
			}
			item.Author = "late-response"
			if err := cache.storeDetails([]Thread{item}, generation); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenCache(root, "github.com", "account")
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := reopened.Get("page"); ok {
				t.Fatal("late details restored a removed page")
			}
			if page, ok := reopened.Get("new-page"); !ok || page.Items[0].Author != "current" {
				t.Fatal("late details overwrote the replacement page")
			}
			isolated, err := OpenCache(root, "github.com", "other-account")
			if err != nil {
				t.Fatal(err)
			}
			if len(isolated.detailSnapshot()) != 0 {
				t.Fatal("author details crossed account boundaries")
			}
		})
	}
}

func TestCacheSaveMergesNewerPages(t *testing.T) {
	root := t.TempDir()
	first, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first.Put("shared", Page{Items: []Thread{{ID: "old"}}, CheckedAt: now.Add(-time.Minute)})
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	first.Put("local-only", Page{Items: []Thread{{ID: "local"}}, CheckedAt: now})
	first.Put("local-newer", Page{Items: []Thread{{ID: "newer"}}, CheckedAt: now})
	second.Put("shared", Page{Items: []Thread{}, ETag: `"empty"`, CheckedAt: now})
	second.Put("disk-only", Page{Items: []Thread{{ID: "disk"}}, CheckedAt: now})
	second.Put("local-newer", Page{Items: []Thread{{ID: "older"}}, CheckedAt: now.Add(-time.Second)})
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if page, ok := reopened.Get("shared"); !ok || len(page.Items) != 0 || page.ETag != `"empty"` {
		t.Fatal("stale save replaced the newer shared page")
	}
	for key, id := range map[string]string{"local-only": "local", "disk-only": "disk", "local-newer": "newer"} {
		if page, ok := reopened.Get(key); !ok || len(page.Items) != 1 || page.Items[0].ID != id {
			t.Fatalf("merge lost the latest %s page: %+v", key, page)
		}
	}
}

func TestCacheSavePreservesAnotherClientsDetails(t *testing.T) {
	root := t.TempDir()
	first, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	item := Thread{ID: "1", Repository: "acme/app", Type: "Issue", Number: 7,
		Author: "oldname", AuthorID: "U_42", State: "open"}
	first.Put("page", Page{Items: []Thread{item}, CheckedAt: time.Now()})
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	item.Author = "newname"
	if err := second.storeDetails([]Thread{item}, second.Generation()); err != nil {
		t.Fatal(err)
	}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if page, ok := reopened.Get("page"); !ok || len(page.Items) != 1 || page.Items[0].Author != "newname" {
		t.Fatal("equal page timestamps lost the newer persisted details")
	}
}

func TestActionPreventsAnotherClientsStalePageSave(t *testing.T) {
	list := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusResetContent)
			return
		}
		fmt.Fprintf(w, "[%s]", threadJSON("42", "Release", "subscribed"))
	})
	query, _ := ParseQuery("")
	if _, err := list.Fetch(context.Background(), query, 10, nil); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(list.Cache.path)))
	actionCache, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	action := &Client{HTTP: list.HTTP, BaseURL: list.BaseURL, Cache: actionCache}
	if err := action.Action(context.Background(), "42", "read"); err != nil {
		t.Fatal(err)
	}
	if err := list.Cache.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get(Endpoint(query, 1)); ok {
		t.Fatal("the other client restored an invalidated notification page")
	}
	if _, ok := list.Cache.Get(Endpoint(query, 1)); ok {
		t.Fatal("the other client retained the invalidated page in memory")
	}
	if got := reopened.RecentRepositories(); len(got) != 1 || got[0].Name != "acme/app" {
		t.Fatalf("invalidation lost repository history: %+v", got)
	}
}

func TestStaleCacheSavePreservesPostActionPages(t *testing.T) {
	root := t.TempDir()
	stale, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	stale.Put("old", Page{Items: []Thread{{Repository: "acme/old"}}, CheckedAt: time.Now()})
	if err := stale.Save(); err != nil {
		t.Fatal(err)
	}
	current, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Invalidate(); err != nil {
		t.Fatal(err)
	}
	current.Put("new", Page{Items: []Thread{{Repository: "acme/new"}}, CheckedAt: time.Now()})
	if err := current.Save(); err != nil {
		t.Fatal(err)
	}
	if err := stale.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("stale writer restored the old page")
	}
	if _, ok := reopened.Get("new"); !ok {
		t.Fatal("stale writer erased the post-action page")
	}
	if got := reopened.RecentRepositories(); len(got) != 2 {
		t.Fatalf("stale writer lost repository history: %+v", got)
	}
}

func TestCacheCrossProcessInvalidation(t *testing.T) {
	const rootVariable = "GH_NOTIFS_TEST_CACHE_ROOT"
	if root := os.Getenv(rootVariable); root != "" {
		cache, err := OpenCache(root, "github.com", "account")
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Invalidate(); err != nil {
			t.Fatal(err)
		}
		return
	}
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	cache.Put("old", Page{Items: []Thread{{Repository: "acme/app"}}, CheckedAt: time.Now()})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestCacheCrossProcessInvalidation$")
	command.Env = append(os.Environ(), rootVariable+"="+root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child invalidation: %v\n%s", err, output)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("parent process restored the invalidated page")
	}
	if got := reopened.RecentRepositories(); len(got) != 1 || got[0].Name != "acme/app" {
		t.Fatalf("cross-process invalidation lost history: %+v", got)
	}
}

func TestCacheV1PersistsInvalidation(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cache.path) != "inbox-v1.json" {
		t.Fatalf("unexpected cache filename: %s", cache.path)
	}
	cache.Put("old", Page{Items: []Thread{{Repository: "acme/app"}}, CheckedAt: time.Now()})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	cache, err = OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get("old"); !ok {
		t.Fatal("saved page was not restored")
	}
	if err := cache.Invalidate(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("restart restored an invalidated page")
	}
	if reopened.diskGeneration != cache.diskGeneration || reopened.diskGeneration == 0 {
		t.Fatal("restart lost the invalidation generation")
	}
	if len(reopened.RecentRepositories()) != 1 {
		t.Fatal("invalidation lost repository history")
	}
}

func TestCacheDoesNotOverwriteInvalidPages(t *testing.T) {
	for _, operation := range []string{"save", "invalidate"} {
		t.Run(operation, func(t *testing.T) {
			cache, err := OpenCache(t.TempDir(), "github.com", "account")
			if err != nil {
				t.Fatal(err)
			}
			const broken = "{broken"
			if err := os.WriteFile(cache.path, []byte(broken), 0600); err != nil {
				t.Fatal(err)
			}
			if operation == "save" {
				err = cache.Save()
			} else {
				err = cache.Invalidate()
			}
			if err == nil {
				t.Fatal("invalid page state was not reported")
			}
			body, err := os.ReadFile(cache.path)
			if err != nil || string(body) != broken {
				t.Fatalf("invalid page state was overwritten: %q, %v", body, err)
			}
		})
	}
}

func TestInvalidationPersistsDespiteHistoryError(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	cache.Put("old", Page{Items: []Thread{{Repository: "acme/app"}}, CheckedAt: time.Now()})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	stale, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	history, err := os.ReadFile(cache.repositoryPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache.repositoryPath(), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Invalidate(); err == nil || !strings.Contains(err.Error(), "save repository history") {
		t.Fatalf("history error was not reported: %v", err)
	}
	if err := os.WriteFile(cache.repositoryPath(), history, 0600); err != nil {
		t.Fatal(err)
	}
	if err := stale.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("old"); ok {
		t.Fatal("history error prevented persistent page invalidation")
	}
}

func TestFailedInvalidationDoesNotMaskAnotherClientsAction(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(cache.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache.path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Invalidate(); err == nil {
		t.Fatal("invalid cache did not fail invalidation")
	}
	if err := os.WriteFile(cache.path, saved, 0600); err != nil {
		t.Fatal(err)
	}
	cache.Put("stale", Page{Items: []Thread{{Repository: "acme/app"}}, CheckedAt: time.Now()})
	other, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Invalidate(); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("stale"); ok {
		t.Fatal("failed invalidation masked another client's successful invalidation")
	}
}

func TestRepositoryHistoryConcurrentWriters(t *testing.T) {
	root := t.TempDir()
	const writers = 8
	caches := make([]*Cache, writers)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := range caches {
		cache, err := OpenCache(root, "github.com", "account")
		if err != nil {
			t.Fatal(err)
		}
		cache.Put("page", Page{Items: []Thread{
			{Repository: fmt.Sprintf("acme/repo%d", i), UpdatedAt: start},
			{Repository: "acme/shared", UpdatedAt: start.Add(time.Duration(i) * time.Hour)},
		}})
		caches[i] = cache
	}
	var wg sync.WaitGroup
	for _, cache := range caches {
		wg.Go(func() {
			if err := cache.Save(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	repositories := reopened.RecentRepositories()
	if len(repositories) != writers+1 {
		t.Fatalf("concurrent saves lost repositories: %+v", repositories)
	}
	want := Repository{Name: "acme/shared", UpdatedAt: start.Add((writers - 1) * time.Hour)}
	if repositories[0] != want {
		t.Fatalf("concurrent saves lost the newest timestamp: got %+v, want %+v", repositories[0], want)
	}
}

func TestActionPreservesAnotherClientsRepositoryHistory(t *testing.T) {
	list := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusResetContent)
			return
		}
		fmt.Fprintf(w, "[%s]", threadJSON("42", "Release", "subscribed"))
	})
	root := filepath.Dir(filepath.Dir(filepath.Dir(list.Cache.path)))
	actionCache, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	action := &Client{HTTP: list.HTTP, BaseURL: list.BaseURL, Cache: actionCache}
	query, _ := ParseQuery("")
	result, err := list.Fetch(context.Background(), query, 10, nil)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("list: %+v, %v", result, err)
	}
	if err := action.Action(context.Background(), "42", "read"); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	want := []Repository{{Name: "acme/app", UpdatedAt: result.Items[0].UpdatedAt}}
	if got := reopened.RecentRepositories(); !reflect.DeepEqual(got, want) {
		t.Fatalf("action erased list history: got %+v, want %+v", got, want)
	}
	if _, ok := reopened.Get(Endpoint(query, 1)); ok {
		t.Fatal("action retained the notification page")
	}
}

func TestRepositoryHistoryPersistence(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	cache.Put("first", Page{Items: []Thread{
		{Repository: "acme/older", UpdatedAt: older},
		{Repository: "acme/newer", UpdatedAt: newer},
		{Repository: "acme/newer", UpdatedAt: older},
		{Repository: "", UpdatedAt: newer},
	}})
	cache.Put("second", Page{Items: []Thread{{Repository: "acme/another", UpdatedAt: newer}}})
	want := []Repository{{"acme/another", newer}, {"acme/newer", newer}, {"acme/older", older}}
	if got := cache.RecentRepositories(); !reflect.DeepEqual(got, want) {
		t.Fatalf("history: %+v, want %+v", got, want)
	}
	generation := cache.Generation()
	if err := cache.Invalidate(); err != nil {
		t.Fatal(err)
	}
	cache.PutIfCurrent("stale", Page{Items: []Thread{{Repository: "acme/stale", UpdatedAt: newer}}}, generation)
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.RecentRepositories(); !reflect.DeepEqual(got, want) {
		t.Fatalf("invalidation or restart lost history: %+v", got)
	}
	if _, ok := reopened.Get("first"); ok {
		t.Fatal("invalidation kept the notification page")
	}
	info, err := os.Stat(cache.repositoryPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("history permissions: %v, %v", info, err)
	}
	for _, identity := range []struct{ host, account string }{
		{"github.com", "another-account"}, {"enterprise.example", "account"},
	} {
		other, err := OpenCache(root, identity.host, identity.account)
		if err != nil {
			t.Fatal(err)
		}
		if len(other.RecentRepositories()) != 0 {
			t.Fatal("repository history crossed an account or host boundary")
		}
	}
}

func TestRepositoryHistorySeedsExistingPages(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(cache.path, cacheData{Pages: map[string]Page{
		"notifications": {Items: []Thread{{Repository: "acme/existing"}}},
	}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.RecentRepositories(); len(got) != 1 || got[0].Name != "acme/existing" {
		t.Fatalf("existing cache did not seed history: %+v", got)
	}
	if err := reopened.Save(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("notifications"); ok {
		t.Fatal("old page did not expire")
	}
	reopened, err = OpenCache(root, "github.com", "account")
	if err != nil || len(reopened.RecentRepositories()) != 1 {
		t.Fatalf("page expiry erased repository history: %v", err)
	}
}

func TestRepositoryHistoryErrors(t *testing.T) {
	root := t.TempDir()
	cache, err := OpenCache(root, "github.com", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cache.repositoryPath(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(); err == nil || !strings.Contains(err.Error(), "save repository history") {
		t.Fatalf("history write failure was not reported: %v", err)
	}
}

func TestRepositoryHistorySaveDoesNotReplaceUnreadableHistory(t *testing.T) {
	for _, body := range []string{"{broken", `{"acme/app":"invalid timestamp"}`} {
		t.Run(body, func(t *testing.T) {
			cache, err := OpenCache(t.TempDir(), "github.com", "account")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cache.repositoryPath(), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := cache.Save(); err == nil || !strings.Contains(err.Error(), "save repository history") {
				t.Fatalf("history read failure was not reported: %v", err)
			}
			got, err := os.ReadFile(cache.repositoryPath())
			if err != nil || string(got) != body {
				t.Fatalf("unreadable history was replaced: %q, %v", got, err)
			}
		})
	}
}
