package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

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
	for _, alias := range []string{"-q", "-L", "-R", "-h"} {
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
			{"repo", []string{"-R", "cli/cli", "--global"}, "--repo, --org, and --global are mutually exclusive"},
			{"missing query", []string{"-q"}, "flag needs an argument"},
			{"missing repo", []string{"-R"}, "flag needs an argument"},
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
		{[]string{"--repo", "cli/cli", "--org", "cli"}, "mutually exclusive"},
		{[]string{"--hostname", "https://github.com"}, "invalid GitHub hostname"},
		{[]string{"-R", "example.com/cli/cli", "--hostname", "github.com"}, "repository host differs"},
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

func TestScopeFlagsAddAlternatives(t *testing.T) {
	for _, opts := range []options{
		{repo: "cli/cli", queryText: "repo:cli/go-gh is:unread"},
		{org: "cli", queryText: "org:acme is:unread"},
	} {
		original := opts
		q, err := opts.scopedQuery("github.com")
		if err != nil {
			t.Fatal(err)
		}
		repos := []string{"cli/cli", "cli/go-gh"}
		if opts.org != "" {
			repos = []string{"cli/cli", "acme/app"}
		} else if len(q.Requests()) != 2 {
			t.Fatal("repository flag did not produce two request alternatives")
		}
		for _, repo := range repos {
			if !q.Match(notifs.Thread{Repository: repo, Unread: true}, "") ||
				q.Match(notifs.Thread{Repository: repo}, "") {
				t.Fatalf("scope %q did not preserve read-state filtering: %+v", repo, q)
			}
		}
		if q.Match(notifs.Thread{Repository: "other/other", Unread: true}, "") || opts != original {
			t.Fatal("scope escaped its group or mutated caller options")
		}
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
