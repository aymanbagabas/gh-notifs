package notifs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Page struct {
	Items        []Thread  `json:"items"`
	Next         bool      `json:"next"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	CheckedAt    time.Time `json:"checked_at"`
}

type Cache struct {
	mu         sync.Mutex
	path       string
	pages      map[string]Page
	generation uint64
}

func OpenCache(root, host, identity string) (*Cache, error) {
	dir := filepath.Join(root, "gh-notifs", cacheKey(host, identity))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	c := &Cache{path: filepath.Join(dir, "inbox-v1.json"), pages: make(map[string]Page)}
	b, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &c.pages); err != nil {
		return nil, err
	}
	if c.pages == nil {
		c.pages = make(map[string]Page)
	}
	return c, nil
}

func cacheKey(host, identity string) string {
	sum := sha256.Sum256([]byte(host + "\x00" + identity))
	return hex.EncodeToString(sum[:16])
}

func (c *Cache) Get(key string) (Page, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pages[key]
	p.Items = append([]Thread(nil), p.Items...)
	return p, ok
}

func (c *Cache) Put(key string, page Page) {
	c.mu.Lock()
	defer c.mu.Unlock()
	page.Items = append([]Thread(nil), page.Items...)
	c.pages[key] = page
}

func (c *Cache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

func (c *Cache) PutIfCurrent(key string, page Page, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == generation {
		page.Items = append([]Thread(nil), page.Items...)
		c.pages[key] = page
	}
}

func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, p := range c.pages {
		if time.Since(p.CheckedAt) > 7*24*time.Hour {
			delete(c.pages, key)
		}
	}
	return writeJSON(c.path, c.pages)
}

func writeJSON(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cache-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (c *Cache) Invalidate() error {
	c.mu.Lock()
	c.generation++
	c.pages = make(map[string]Page)
	c.mu.Unlock()
	return c.Save()
}
