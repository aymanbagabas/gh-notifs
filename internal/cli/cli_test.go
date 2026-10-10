package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func runCommand(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()
	cmd, err := newCommand("test")
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
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
			{"removed limit n", []string{"-n", "10"}, "unknown shorthand flag: 'n'"},
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
		{[]string{"list", "extra"}, "unknown command"},
		{[]string{"--page", "2"}, "--page requires the list command"},
		{[]string{"--repo", "cli/cli", "--org", "cli"}, "mutually exclusive"},
		{[]string{"--hostname", "https://github.com"}, "invalid GitHub hostname"},
		{[]string{"-R", "example.com/cli/cli", "--hostname", "github.com"}, "repository host differs"},
		{[]string{"--query", "state:invalid", "list"}, "unsupported state"},
		{[]string{"-L0"}, "--limit and --page must be positive"},
		{[]string{"--interval", "1m"}, "unknown flag"},
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
	for _, command := range []string{"", "list", "open", "read", "done", "subscribe", "unsubscribe", "completion"} {
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
