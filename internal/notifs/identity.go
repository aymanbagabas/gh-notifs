package notifs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type identity struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
	Login  string `json:"login"`
}

func (c *Client) identityPath() string {
	return filepath.Join(c.cacheRoot, "gh-notifs", "identities", c.identityKey+".json")
}

func (c *Client) loadIdentity() (bool, error) {
	b, err := os.ReadFile(c.identityPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read cached identity: %w", err)
	}
	var user identity
	if err := json.Unmarshal(b, &user); err != nil {
		return false, fmt.Errorf("decode cached identity: %w", err)
	}
	return true, c.useIdentity(user)
}

func (c *Client) useIdentity(user identity) error {
	if user.ID <= 0 || user.Login == "" || user.NodeID == "" {
		return fmt.Errorf("GitHub identity requires a positive user ID, a node ID, and a login")
	}
	cache, err := OpenCache(c.cacheRoot, c.Host, strconv.FormatInt(user.ID, 10))
	if err != nil {
		return fmt.Errorf("open account cache: %w", err)
	}
	c.mu.Lock()
	c.Cache, c.accountID = cache, user.NodeID
	c.mu.Unlock()
	return nil
}

// Initialize resolves an unfamiliar credential in the request worker, not the UI loop.
func (c *Client) Initialize(ctx context.Context) error {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	ready := c.Cache != nil
	c.mu.Unlock()
	if ready {
		return nil
	}
	loaded, err := c.loadIdentity()
	if loaded || err != nil {
		return err
	}
	resp, err := c.request(ctx, http.MethodGet, "user", nil, nil)
	if err != nil {
		return fmt.Errorf("resolve account identity: %w", err)
	}
	defer resp.Body.Close()
	var user identity
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return fmt.Errorf("decode account identity: %w", err)
	}
	if user.ID <= 0 || user.Login == "" || user.NodeID == "" {
		return fmt.Errorf("GitHub returned an invalid account identity")
	}
	if err := writeJSON(c.identityPath(), user); err != nil {
		return fmt.Errorf("save account identity: %w", err)
	}
	return c.useIdentity(user)
}

func (c *Client) cachedAuthors(q Query) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	resolved := make(map[string]string)
	for _, name := range q.values("author") {
		id := c.authors[name]
		if name == "@me" && c.accountID != "" {
			id = c.accountID
		}
		if id != "" {
			resolved[name] = id
		}
	}
	return resolved
}

func (c *Client) resolveAuthors(ctx context.Context, q Query) (map[string]string, error) {
	names := q.values("author")
	if len(names) == 0 {
		return nil, nil
	}
	c.authorMu.Lock()
	defer c.authorMu.Unlock()
	resolved := c.cachedAuthors(q)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := resolved[name]
		if id == "" {
			endpoint := "users/" + url.PathEscape(name)
			if name == "@me" {
				endpoint = "user"
			}
			resp, err := c.request(ctx, http.MethodGet, endpoint, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("resolve author %q: %w", name, err)
			}
			var user struct {
				NodeID string `json:"node_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&user)
			resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("decode author %q: %w", name, err)
			}
			if user.NodeID == "" {
				return nil, fmt.Errorf("GitHub returned no node ID for author %q", name)
			}
			id = user.NodeID
			c.mu.Lock()
			if c.authors == nil {
				c.authors = make(map[string]string)
			}
			c.authors[name] = id
			c.mu.Unlock()
		}
		resolved[name] = id
	}
	return resolved, nil
}
