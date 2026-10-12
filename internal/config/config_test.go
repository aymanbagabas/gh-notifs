package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateDefaultsAndPreserveEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-notifs", "config.yml")
	cfg, err := Load(path)
	if err != nil || len(cfg.Scopes) != 6 || cfg.Scopes[0].Query != "" || cfg.AutoRefresh || cfg.Bell {
		t.Fatalf("default config: %+v, %v", cfg, err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != DefaultText || !strings.Contains(string(b), "# Each scope") {
		t.Fatal("default file has no instructions")
	}
	custom := "auto_refresh: true\nbell: true\nscopes:\n  - name: My PRs\n    query: 'repo:acme/app is:pr'\n"
	if err := os.WriteFile(path, []byte(custom), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || len(cfg.Scopes) != 1 || cfg.Scopes[0].Query != "repo:acme/app is:pr" || !cfg.AutoRefresh || !cfg.Bell {
		t.Fatalf("custom scopes: %+v, %v", cfg, err)
	}
	b, _ = os.ReadFile(path)
	if string(b) != custom {
		t.Fatal("configuration was overwritten")
	}
}

func TestInvalidConfigIsReportedWithoutReplacement(t *testing.T) {
	for _, input := range []string{"", "scops: []", "auto_refresh: sometimes", "bell: loud", "scopes:\n- name: ''\n  query: ''", "scopes:\n- name: Bad\n  query: 'is:saved'", "scopes: []\n---\nscopes: []"} {
		path := filepath.Join(t.TempDir(), "config.yml")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted invalid config %q", input)
		}
		b, _ := os.ReadFile(path)
		if string(b) != input {
			t.Fatal("invalid config was replaced")
		}
	}
}
