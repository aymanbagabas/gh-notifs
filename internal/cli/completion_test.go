package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionScripts(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out, stderr, err := runCommand(t, "", "completion", shell)
			if err != nil || stderr != "" || !strings.Contains(out, "gh-notifs") || !strings.Contains(out, "__complete") {
				t.Fatalf("completion %s: output=%q stderr=%q error=%v", shell, out, stderr, err)
			}
		})
	}
	if _, _, err := runCommand(t, "", "completion", "invalid"); err == nil {
		t.Fatal("unknown completion shell was accepted")
	}
}

func TestCompletionCandidates(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"__complete", "li"}, "list\t"},
		{[]string{"__complete", "list", "--qu"}, "--query\t"},
		{[]string{"__complete", "list", "--co"}, "--config\t"},
		{[]string{"__complete", "list", "--query", ""}, ":4"},
		{[]string{"__complete", "list", "--repo", ""}, ":4"},
		{[]string{"__complete", "list", "--config", ""}, ":0"},
	} {
		out, _, err := runCommand(t, "", tt.args...)
		if err != nil || !strings.Contains(out, tt.want) {
			t.Fatalf("%v: completion=%q error=%v; want %q", tt.args, out, err, tt.want)
		}
	}
	config := filepath.Join(t.TempDir(), "must-not-be-created.yml")
	_, _, err := runCommand(t, "", "__complete", "--config", config, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("completion touched the configuration file: %v", err)
	}
}
