package main

import (
	"fmt"
	"os"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.MkdirTemp("", "gh-notifs-repositories-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	cache, err := notifs.OpenCache(root, "github.com", "example")
	if err != nil {
		return err
	}
	cache.Put("notifications", notifs.Page{Items: []notifs.Thread{
		{Repository: "acme/older", UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{Repository: "acme/newer", UpdatedAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)},
	}})
	if err := cache.Invalidate(); err != nil {
		return err
	}
	cache, err = notifs.OpenCache(root, "github.com", "example")
	if err != nil {
		return err
	}
	fmt.Println("Repositories after page invalidation and restart:")
	for _, repo := range cache.RecentRepositories() {
		fmt.Printf("%s  %s\n", repo.Name, repo.UpdatedAt.Format(time.DateOnly))
	}
	return nil
}
