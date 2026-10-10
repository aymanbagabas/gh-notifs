package notifs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type identity struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
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
	if user.ID <= 0 || user.Login == "" {
		return fmt.Errorf("GitHub identity requires a positive user ID and a login")
	}
	cache, err := OpenCache(c.cacheRoot, c.Host, strconv.FormatInt(user.ID, 10))
	if err != nil {
		return fmt.Errorf("open account cache: %w", err)
	}
	c.mu.Lock()
	c.Cache, c.login = cache, user.Login
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
	if user.ID <= 0 || user.Login == "" {
		return fmt.Errorf("GitHub returned an invalid account identity")
	}
	if err := writeJSON(c.identityPath(), user); err != nil {
		return fmt.Errorf("save account identity: %w", err)
	}
	return c.useIdentity(user)
}
