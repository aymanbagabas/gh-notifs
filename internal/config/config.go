package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"gopkg.in/yaml.v3"
)

const DefaultText = `# gh-notifs configuration
# Each scope has a display name and a complete notification query.
# A click replaces the current query. No repository scope is added implicitly.
# Use an empty query for all notifications. Scope order follows this file.
# Restart gh-notifs after edits. Existing files are never overwritten.
# Examples: repo:cli/cli is:unread, org:acme reason:review-requested
# Quote YAML strings. Supported fields:
# repo, org, reason, is, author, state, since, before, and quoted title text.
# Repeated filter values are alternatives. Each filter group must match.
# Type and read status are separate groups.
# Use is:issue-or-pull-request for both issues and pull requests.
# is:issue selects only issues; is:pr and is:pull-request select only pull requests.
# Every title term and date bound must match. AND and OR are title words.
# Dates use YYYY-MM-DD or RFC3339. Repeated dates use the strictest bounds.
# Use scopes: [] to show only the repository history.
auto_refresh: false
bell: false
scopes:
  - name: Inbox
    query: ""
  - name: Unread
    query: "is:unread"
  - name: Review requested
    query: "reason:review-requested"
  - name: Mentioned
    query: "reason:mention"
  - name: Team mentioned
    query: "reason:team-mention"
  - name: Assigned
    query: "reason:assign"
`

type Scope struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

type Config struct {
	Scopes      []Scope `yaml:"scopes"`
	AutoRefresh bool    `yaml:"auto_refresh"`
	Bell        bool    `yaml:"bell"`
}

func Path() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(root, "gh-notifs", "config.yml"), nil
}

func Load(path string) (Config, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Config{}, fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := io.WriteString(f, DefaultText)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return Config{}, fmt.Errorf("write config %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return Config{}, fmt.Errorf("create config %s: %w", path, err)
	}
	f, err = os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("only one YAML document is supported")
		}
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	for i, scope := range cfg.Scopes {
		if strings.TrimSpace(scope.Name) == "" || notifs.Plain(scope.Name) != scope.Name {
			return Config{}, fmt.Errorf("config %s: scope %d needs a printable name", path, i+1)
		}
		if _, err := notifs.ParseQuery(scope.Query); err != nil {
			return Config{}, fmt.Errorf("config %s: scope %q: %w", path, scope.Name, err)
		}
	}
	return cfg, nil
}
