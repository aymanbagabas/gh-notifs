package notifs

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
	if err := writeJSON(cache.path, map[string]Page{
		"notifications": {Items: []Thread{{Repository: "acme/existing"}}},
	}); err != nil {
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
