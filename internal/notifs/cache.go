package notifs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	mu             sync.Mutex
	path           string
	pages          map[string]Page
	repositories   map[string]time.Time
	generation     uint64
	diskGeneration uint64
}

type cacheData struct {
	Generation uint64          `json:"generation"`
	Pages      map[string]Page `json:"pages"`
}

func OpenCache(root, host, identity string) (*Cache, error) {
	dir := filepath.Join(root, "gh-notifs", cacheKey(host, identity))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	c := &Cache{path: filepath.Join(dir, "inbox-v1.json"), pages: make(map[string]Page),
		repositories: make(map[string]time.Time)}
	lock, err := c.lockFile()
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	data, err := c.readPages()
	if err != nil {
		return nil, err
	}
	c.pages, c.diskGeneration = data.Pages, data.Generation
	b, err := os.ReadFile(c.repositoryPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &c.repositories); err != nil {
			return nil, fmt.Errorf("read repository history: %w", err)
		}
	}
	if c.repositories == nil {
		c.repositories = make(map[string]time.Time)
	}
	for _, page := range c.pages {
		c.recordRepositories(page.Items)
	}
	return c, nil
}

func (c *Cache) readPages() (cacheData, error) {
	data := cacheData{Pages: make(map[string]Page)}
	b, err := os.ReadFile(c.path)
	if err == nil {
		err = json.Unmarshal(b, &data)
	}
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if data.Pages == nil {
		data.Pages = make(map[string]Page)
	}
	return data, err
}

func (c *Cache) lockFile() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(c.repositoryPath()+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockCacheFile(lock); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// The caller holds both the memory lock and the account's file lock.
func (c *Cache) syncGeneration() error {
	data, err := c.readPages()
	if err != nil {
		return err
	}
	if data.Generation != c.diskGeneration {
		c.generation++
		c.diskGeneration, c.pages = data.Generation, data.Pages
		for _, page := range c.pages {
			c.recordRepositories(page.Items)
		}
	}
	return nil
}

func (c *Cache) refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.lockFile()
	if err != nil {
		return err
	}
	defer lock.Close()
	return c.syncGeneration()
}

type Repository struct {
	Name      string
	UpdatedAt time.Time
}

func (c *Cache) repositoryPath() string {
	return filepath.Join(filepath.Dir(c.path), "repositories-v1.json")
}

// The caller holds c.mu, except during OpenCache initialization.
func (c *Cache) recordRepositories(items []Thread) {
	for _, item := range items {
		if item.Repository == "" {
			continue
		}
		updated, ok := c.repositories[item.Repository]
		if !ok || item.UpdatedAt.After(updated) {
			c.repositories[item.Repository] = item.UpdatedAt
		}
	}
}

func (c *Cache) RecentRepositories() []Repository {
	c.mu.Lock()
	defer c.mu.Unlock()
	repositories := make([]Repository, 0, len(c.repositories))
	for name, updated := range c.repositories {
		repositories = append(repositories, Repository{Name: name, UpdatedAt: updated})
	}
	sort.Slice(repositories, func(i, j int) bool {
		if repositories[i].UpdatedAt.Equal(repositories[j].UpdatedAt) {
			return repositories[i].Name < repositories[j].Name
		}
		return repositories[i].UpdatedAt.After(repositories[j].UpdatedAt)
	})
	return repositories
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
	c.recordRepositories(page.Items)
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
		c.recordRepositories(page.Items)
	}
}

func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	lock, err := c.lockFile()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := c.syncGeneration(); err != nil {
		return err
	}
	return c.save(c.diskGeneration)
}

// The generation and pages share one atomic write, so a crash cannot detach them.
func (c *Cache) save(generation uint64) error {
	for key, p := range c.pages {
		if time.Since(p.CheckedAt) > 7*24*time.Hour {
			delete(c.pages, key)
		}
	}
	historyErr := c.saveRepositories()
	if historyErr != nil {
		historyErr = fmt.Errorf("save repository history: %w", historyErr)
	}
	pageErr := writeJSON(c.path, cacheData{Generation: generation, Pages: c.pages})
	if pageErr == nil {
		c.diskGeneration = generation
	}
	return errors.Join(historyErr, pageErr)
}

func (c *Cache) saveRepositories() error {
	path := c.repositoryPath()
	// The account's file lock also protects this history merge.
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		var repositories map[string]time.Time
		if err := json.Unmarshal(b, &repositories); err != nil {
			return err
		}
		for name, updated := range repositories {
			if previous, ok := c.repositories[name]; !ok || updated.After(previous) {
				c.repositories[name] = updated
			}
		}
	}
	return writeJSON(path, c.repositories)
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
	defer c.mu.Unlock()
	c.generation++
	c.pages = make(map[string]Page)
	lock, err := c.lockFile()
	if err != nil {
		return err
	}
	defer lock.Close()
	data, err := c.readPages()
	if err != nil {
		return err
	}
	return c.save(data.Generation + 1)
}
