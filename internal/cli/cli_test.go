package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/aymanbagabas/gh-notifs/internal/config"
)

func TestRefreshSwitchesOverrideConfig(t *testing.T) {
	for _, defaults := range []bool{false, true} {
		for _, tc := range []struct {
			args          []string
			refresh, bell bool
		}{
			{nil, defaults, defaults},
			{[]string{"--auto-refresh", "--bell"}, true, true},
			{[]string{"--no-auto-refresh", "--no-bell"}, false, false},
			{[]string{"--no-auto-refresh", "--auto-refresh", "--bell", "--no-bell"}, true, false},
			{[]string{"--auto-refresh", "list", "--no-auto-refresh", "--bell"}, false, true},
			{[]string{"--no-bell", "list", "--bell", "--auto-refresh"}, true, true},
		} {
			opts, _, _, err := parseArgs(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{AutoRefresh: defaults, Bell: defaults, Scopes: []config.Scope{{Name: "Inbox"}}}
			before := cfg
			got := opts.inboxOptions(cfg)
			if got.AutoRefresh != tc.refresh || got.Bell != tc.bell {
				t.Errorf("defaults=%t args=%v: got %+v", defaults, tc.args, got)
			}
			if !reflect.DeepEqual(cfg, before) {
				t.Fatal("caller configuration changed")
			}
		}
	}
	for _, arg := range []string{"--auto-refresh=false", "--no-auto-refresh=false", "--bell=false", "--no-bell=false"} {
		if _, _, _, err := parseArgs([]string{arg}); err == nil {
			t.Errorf("accepted value in %s instead of the paired switch", arg)
		}
	}
}

func runCommand(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := run(context.Background(), args, "test", strings.NewReader(input), &out, &stderr)
	return out.String(), stderr.String(), err
}

func TestHelpFlagAlignment(t *testing.T) {
	help, stderr, err := runCommand(t, "", "--help")
	if err != nil || stderr != "" {
		t.Fatalf("help failed: %v, %q", err, stderr)
	}
	_, options, ok := strings.Cut(help, "Flags:\n")
	if !ok {
		t.Fatal("help has no Flags section")
	}
	options, _, _ = strings.Cut(options, "\n\n")
	column := -1
	for _, line := range strings.Split(options, "\n") {
		if strings.Index(line, "--") != 6 {
			t.Fatalf("long flag is not aligned at column 7: %q", line)
		}
		_, description, ok := strings.Cut(line[6:], "  ")
		description = strings.TrimLeft(description, " ")
		if column < 0 {
			column = len(line) - len(description)
		}
		if !ok || description == "" || len(line)-len(description) != column {
			t.Fatalf("description is not aligned at column %d: %q", column+1, line)
		}
	}
	for _, alias := range []string{"-q", "-L", "-h"} {
		if !strings.Contains(options, alias) {
			t.Fatalf("help does not describe alias %s", alias)
		}
	}
}

func TestCommandExamples(t *testing.T) {
	for _, action := range []string{"open", "read", "done", "subscribe", "unsubscribe"} {
		t.Run(action, func(t *testing.T) {
			rootHelp, _, err := runCommand(t, "", "--help")
			if err != nil || !strings.Contains(rootHelp, "gh-notifs "+action) {
				t.Fatalf("root help omits %s example: %v", action, err)
			}
			help, stderr, err := runCommand(t, "", action, "--help")
			if err != nil || stderr != "" {
				t.Fatalf("help failed: %v, %q", err, stderr)
			}
			for _, want := range []string{"Examples:", "| gh-notifs " + action,
				"gh-notifs " + action + " < selected.tsv", "do not include --header"} {
				if !strings.Contains(help, want) {
					t.Errorf("help omits %q", want)
				}
			}
			if action != "open" && !strings.Contains(help, "without confirmation") {
				t.Error("action help omits the mutation warning")
			}
		})
	}
}

func TestFlagAliases(t *testing.T) {
	for _, command := range []string{"", "list"} {
		for _, tt := range []struct {
			name string
			args []string
			want string
		}{
			{"query", []string{"-q", "unknown:value"}, `unsupported query field "unknown"`},
			{"query equals", []string{"-q=unknown:value"}, `unsupported query field "unknown"`},
			{"short query last", []string{"--query", "is:unread", "-q", "unknown:value"}, `unsupported query field "unknown"`},
			{"long query last", []string{"-q", "unknown:value", "--query", "state:invalid"}, `unsupported state "invalid"`},
			{"removed limit n", []string{"-n", "10"}, "flag provided but not defined: -n"},
			{"limit L", []string{"-L", "0"}, "--limit and --page must be positive"},
			{"limit equals", []string{"-L=0"}, "--limit and --page must be positive"},
			{"short limit last", []string{"--limit", "0", "-L", "10", "-q", "unknown:value"}, `unsupported query field "unknown"`},
			{"long limit last", []string{"-L", "10", "--limit", "0"}, "--limit and --page must be positive"},
			{"removed repo alias", []string{"-R", "cli/cli"}, "flag provided but not defined: -R"},
			{"removed repo", []string{"--repo", "cli/cli"}, "flag provided but not defined: -repo"},
			{"removed org", []string{"--org", "cli"}, "flag provided but not defined: -org"},
			{"removed global", []string{"--global"}, "flag provided but not defined: -global"},
			{"missing query", []string{"-q"}, "flag needs an argument"},
			{"missing limit", []string{"-L"}, "flag needs an argument"},
			{"help", []string{"-h"}, ""},
		} {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				args := []string{"--hostname", "github.com"}
				if command != "" {
					args = append([]string{command}, args...)
				}
				out, stderr, err := runCommand(t, "", append(args, tt.args...)...)
				if stderr != "" || (tt.want != "" && out != "") {
					t.Fatalf("parser printed unexpected output: stdout=%q, stderr=%q", out, stderr)
				}
				if tt.want == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("got %v, want %q", err, tt.want)
				}
			})
		}
	}
}

func TestCommandValidation(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"unknown"}, "unknown command"},
		{[]string{"list", "extra"}, "unexpected argument"},
		{[]string{"--page", "2"}, "--page requires the list command"},
		{[]string{"--hostname", "https://github.com"}, "invalid GitHub hostname"},
		{[]string{"--query", "state:invalid", "list"}, "unsupported state"},
		{[]string{"-L0"}, "flag provided but not defined: -L0"},
		{[]string{"--interval", "1m"}, "flag provided but not defined: -interval"},
		{[]string{"completion", "bash"}, "unknown command"},
		{[]string{"__complete", "list"}, "unknown command"},
		{[]string{"open", "http://example.com"}, "invalid browser URL"},
	} {
		out, stderr, err := runCommand(t, "", tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) || out != "" || stderr != "" {
			t.Fatalf("%v: output=%q stderr=%q error=%v; want %q", tt.args, out, stderr, err, tt.want)
		}
	}
}

func TestHelpAndVersionNeedNoAuthentication(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
	for _, command := range []string{"", "list", "open", "read", "done", "subscribe", "unsubscribe"} {
		for _, flag := range []string{"--help", "--version"} {
			args := []string{flag}
			if command != "" {
				args = append([]string{command}, args...)
			}
			out, stderr, err := runCommand(t, "", args...)
			if err != nil || out == "" || stderr != "" {
				t.Fatalf("%v: output=%q stderr=%q error=%v", args, out, stderr, err)
			}
			if flag == "--version" && out != "test\n" {
				t.Fatalf("%v: unexpected version %q", args, out)
			}
		}
	}
	out, _, err := runCommand(t, "", "help", "list")
	if err != nil || !strings.Contains(out, "gh-notifs list") {
		t.Fatalf("subcommand help: %q, %v", out, err)
	}
}

func TestFlagParsingBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		args, rest    []string
		limit         int
		query         string
	}{
		{"defaults", "", nil, nil, 50, ""},
		{"before command", "list", []string{"-L", "10", "list"}, nil, 10, ""},
		{"after command", "list", []string{"list", "-L=10"}, nil, 10, ""},
		{"both sides", "list", []string{"--limit", "5", "list", "-L", "10"}, nil, 10, ""},
		{"reverse aliases", "list", []string{"-L", "5", "list", "--limit", "10"}, nil, 10, ""},
		{"command as value", "list", []string{"-q", "read", "list"}, nil, 50, "read"},
		{"separator as value", "list", []string{"-q", "--", "list", "-L", "10"}, nil, 10, "--"},
		{"flags before IDs", "read", []string{"read", "-L", "10", "123", "456"}, []string{"123", "456"}, 10, ""},
		{"stop at ID", "read", []string{"read", "123", "--version"}, []string{"123", "--version"}, 50, ""},
		{"explicit separator", "read", []string{"read", "--", "--version"}, []string{"--version"}, 50, ""},
		{"empty value", "list", []string{"-q", "old", "list", "--query="}, nil, 50, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := slices.Clone(tt.args)
			opts, command, rest, err := parseArgs(tt.args)
			if err != nil || command != tt.command || !slices.Equal(rest, tt.rest) ||
				opts.limit != tt.limit || opts.queryText != tt.query || opts.version {
				t.Fatalf("parseArgs(%q) = %+v, %q, %q, %v", tt.args, opts, command, rest, err)
			}
			if !slices.Equal(tt.args, original) {
				t.Fatal("parser mutated caller arguments")
			}
		})
	}
}

type scopeTransport func(*http.Request) (*http.Response, error)

func (f scopeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestQueryScopeIgnoresCurrentRepository(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", root},
		{"-C", root, "remote", "add", "origin", "https://github.com/local/checkout.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "fictional-scope-test-token")
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, envRepo := range []string{"", "env/override", "not/a/valid/repo"} {
		t.Run(envRepo, func(t *testing.T) {
			t.Setenv("GH_REPO", envRepo)
			for _, command := range []string{"", "list"} {
				for _, tc := range []struct{ query, endpoint string }{
					{"", "/notifications"},
					{"is:unread", "/notifications"},
					{"author:@me", "/notifications"},
					{"repo:cli/cli is:unread", "/repos/cli/cli/notifications"},
					{"org:acme", "/notifications"},
				} {
					calls := 0
					http.DefaultTransport = scopeTransport(func(r *http.Request) (*http.Response, error) {
						body := "[]"
						switch r.URL.Path {
						case "/user":
							body = `{"id":1,"node_id":"U_scope","login":"example"}`
						case tc.endpoint:
							calls++
						default:
							return nil, fmt.Errorf("unexpected request: %s", r.URL)
						}
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
							Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
					args := []string{"--hostname", "github.com"}
					if command != "" {
						args = append(args, command)
					}
					if tc.query != "" {
						args = append(args, "-q", tc.query)
					}
					out, stderr, err := runCommand(t, "", args...)
					if err != nil || out != "" || stderr != "" || calls != 1 {
						t.Fatalf("%v: output=%q stderr=%q error=%v requests=%d", args, out, stderr, err, calls)
					}
				}
			}
		})
	}
}

func TestInputArgumentsAndPipelines(t *testing.T) {
	for _, tt := range []struct {
		input string
		args  []string
		want  string
	}{
		{"ignored\n", []string{"1", "2"}, "1|2"},
		{"1\ttrue\n\n2\n", nil, "1\ttrue|2"},
		{"", nil, ""},
	} {
		var got []string
		err := eachInput(strings.NewReader(tt.input), tt.args, func(line string) error {
			got = append(got, line)
			return nil
		})
		if err != nil || strings.Join(got, "|") != tt.want {
			t.Fatalf("input: got %v, %v; want %q", got, err, tt.want)
		}
	}
	err := eachInput(strings.NewReader("1\n2\n"), nil, func(string) error { return io.ErrClosedPipe })
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("input lost the action error: %v", err)
	}
}
