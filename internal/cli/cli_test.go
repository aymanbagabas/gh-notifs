package cli

import (
	"context"
	"strings"
	"testing"
)

func TestHelpFlagAlignment(t *testing.T) {
	_, options, ok := strings.Cut(help, "Options:\n")
	if !ok {
		t.Fatal("help has no Options section")
	}
	options, _, _ = strings.Cut(options, "\n\n")
	for _, line := range strings.Split(options, "\n") {
		if strings.Index(line, "--") != 10 {
			t.Fatalf("long flag is not aligned at column 11: %q", line)
		}
		_, description, ok := strings.Cut(line[10:], "  ")
		description = strings.TrimLeft(description, " ")
		if !ok || description == "" || len(line)-len(description) != 29 {
			t.Fatalf("description is not aligned at column 30: %q", line)
		}
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
			{"limit n", []string{"-n", "0"}, "--limit and --page must be positive"},
			{"limit L", []string{"-L", "0"}, "--limit and --page must be positive"},
			{"limit equals", []string{"-n=0"}, "--limit and --page must be positive"},
			{"short limit last", []string{"--limit", "0", "-n", "10", "-q", "unknown:value"}, `unsupported query field "unknown"`},
			{"gh limit last", []string{"-n", "0", "-L", "10", "-q", "unknown:value"}, `unsupported query field "unknown"`},
			{"long limit last", []string{"-L", "10", "--limit", "0"}, "--limit and --page must be positive"},
			{"repo", []string{"-R", "cli/cli", "--global"}, "--repo, --org, and --global are mutually exclusive"},
			{"missing query", []string{"-q"}, "flag needs an argument: -q"},
			{"missing repo", []string{"-R"}, "flag needs an argument: -R"},
			{"missing limit", []string{"-n"}, "flag needs an argument: -n"},
			{"help", []string{"-h"}, ""},
		} {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				args := []string{"--hostname", "github.com"}
				if command != "" {
					args = append([]string{command}, args...)
				}
				err := Run(context.Background(), append(args, tt.args...), "test")
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
