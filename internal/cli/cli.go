package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aymanbagabas/gh-notifs/internal/config"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/tui"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
)

const description = `With terminal input and output, start the notification inbox.
Otherwise, fetch once and print TSV. The list command always prints TSV.
Read and unread notifications are included by default.

Query fields:
  repo:OWNER/REPO org:ORG reason:review-requested is:read is:unread
  reason:participating
  is:issue-or-pull-request is:discussion is:release is:commit is:check-suite
  is:issue is:pr is:pull-request
  author:LOGIN author:@me state:open state:closed state:merged state:draft
  since:YYYY-MM-DD before:YYYY-MM-DD "title text"
Dates use notification update times. RFC3339 timestamps also work.
Author and state filters currently apply to issues and pull requests.
Unsupported fields and website-only states produce an error.
Use is:issue-or-pull-request for both issues and pull requests.
Use is:issue for issues only, or is:pr/is:pull-request for pull requests only.
These specific type filters differ from the website's combined behavior.
Repeated values in each filter group are alternatives.
Type and read status are separate groups; every group must match.
Every title term and date bound must match. AND and OR are title words,
not operators. -R and --org add alternatives to repo: and org: groups.
Multiple repo: values use concurrent repository requests with one result limit.

TSV columns (no header unless --header):
  id unread updated_at last_read_at repository type reason reference
  state author title url
Times use UTC RFC3339; missing values are empty. Tabs, newlines, and
terminal controls inside values become spaces.`

type options struct {
	queryText, repo, org, host, configPath string
	global, header, noColor                bool
	help, version                          bool
	limit, page                            int
}

func Run(ctx context.Context, args []string, version string) error {
	return run(ctx, args, version, os.Stdin, os.Stdout, os.Stderr)
}

func run(ctx context.Context, args []string, version string, input io.Reader, output, stderr io.Writer) error {
	opts, command, args, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch command {
	case "", "list", "open", "read", "done", "subscribe", "unsubscribe":
	default:
		return fmt.Errorf("unknown command %q; use --help", command)
	}
	if opts.help {
		return writeHelp(output, command)
	}
	if opts.version {
		_, err := fmt.Fprintln(output, version)
		return err
	}
	if (command == "" || command == "list") && len(args) > 0 {
		return fmt.Errorf("unexpected argument %q; use --help", args[0])
	}
	return opts.run(ctx, command, args, input, output, stderr)
}

func parseArgs(args []string) (options, string, []string, error) {
	var opts options
	flags := flag.NewFlagSet("gh-notifs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.queryText, "query", "", "notification query")
	flags.StringVar(&opts.queryText, "q", "", "notification query")
	flags.StringVar(&opts.repo, "repo", "", "repository scope")
	flags.StringVar(&opts.repo, "R", "", "repository scope")
	flags.StringVar(&opts.org, "org", "", "organization scope")
	flags.StringVar(&opts.host, "hostname", "", "GitHub host")
	flags.StringVar(&opts.configPath, "config", "", "scope configuration file")
	flags.BoolVar(&opts.global, "global", false, "all repositories")
	flags.BoolVar(&opts.header, "header", false, "TSV header")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable colors")
	flags.BoolVar(&opts.help, "help", false, "show help")
	flags.BoolVar(&opts.help, "h", false, "show help")
	flags.BoolVar(&opts.version, "version", false, "show version")
	flags.IntVar(&opts.limit, "limit", 50, "result limit")
	flags.IntVar(&opts.limit, "L", 50, "result limit")
	flags.IntVar(&opts.page, "page", 1, "result page")
	if err := flags.Parse(args); err != nil {
		return opts, "", nil, err
	}
	command := ""
	args = flags.Args()
	if len(args) > 0 {
		command, args = args[0], args[1:]
		if command == "help" {
			opts.help, command = true, ""
			if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
				command, args = args[0], args[1:]
			}
		}
		if err := flags.Parse(args); err != nil {
			return opts, command, nil, err
		}
		args = flags.Args()
	}
	return opts, command, args, nil
}

const flagHelp = `  -q, --query TEXT       Filter notifications; repeated values are alternatives
  -R, --repo OWNER/REPO  Add [HOST/]OWNER/REPO to repository alternatives
      --org ORG          Add an organization to owner alternatives
      --global           Ignore the current repository
      --hostname HOST    GitHub host; defaults to the gh configuration
  -L, --limit N          Results per page (default 50)
      --page N           List result page (default 1; list only)
      --config PATH      Scope configuration file (interactive inbox only)
      --no-color         Disable TUI color
      --header           Include column names in TSV
      --version          Print version
  -h, --help             Show help`

func writeHelp(output io.Writer, command string) error {
	use, long := "[command] [flags]", description
	example := `  gh-notifs
  gh-notifs --global -q 'reason:review-requested'
  gh-notifs list -R cli/cli -q 'is:unread' -L 10
  gh-notifs list -R cli/cli -q 'repo:cli/go-gh reason:mention reason:comment'
  gh-notifs list -L 10 --header
  gh-notifs list -L 1 | gh-notifs open
  gh-notifs read 123456789
  gh-notifs done 123456789
  gh-notifs subscribe 123456789
  gh-notifs unsubscribe 123456789`
	for _, spec := range []struct{ name, use, short, long, example string }{
		{"list", "list", "Fetch notifications once and print TSV", description,
			"  gh-notifs list -R cli/cli -q 'is:unread' -L 10\n  gh-notifs list --global --header\n  gh-notifs list --page 2 -L 20"},
		{"open", "open [URL ...]", "Open notifications in the browser", "Accept HTTPS URLs as arguments, or TSV rows/URLs on stdin.",
			"  gh-notifs open https://github.com/cli/cli/issues/1\n  gh-notifs list -R cli/cli -L 1 | gh-notifs open\n  gh-notifs open < selected.tsv"},
		{"read", "read [THREAD_ID ...]", "Mark notifications read", "Accept thread IDs as arguments, or TSV rows/IDs on stdin.",
			"  gh-notifs read 123456789 987654321\n  gh-notifs list -R cli/cli -q 'is:unread' -L 10 | gh-notifs read\n  gh-notifs read < selected.tsv"},
		{"done", "done [THREAD_ID ...]", "Mark notifications done", "Accept thread IDs as arguments, or TSV rows/IDs on stdin.",
			"  gh-notifs done 123456789\n  gh-notifs list -R cli/cli -q 'is:read' -L 10 | gh-notifs done\n  gh-notifs done < selected.tsv"},
		{"subscribe", "subscribe [THREAD_ID ...]", "Subscribe to notifications", "Accept thread IDs as arguments, or TSV rows/IDs on stdin.",
			"  gh-notifs subscribe 123456789\n  printf '%s\\n' 123456789 987654321 | gh-notifs subscribe\n  gh-notifs subscribe < selected.tsv"},
		{"unsubscribe", "unsubscribe [THREAD_ID ...]", "Unsubscribe from notifications", "Accept thread IDs as arguments, or TSV rows/IDs on stdin.",
			"  gh-notifs unsubscribe 123456789\n  printf '%s\\n' 123456789 987654321 | gh-notifs unsubscribe\n  gh-notifs unsubscribe < selected.tsv"},
	} {
		if command == "" {
			if spec.name == "list" {
				long += "\n\nCommands:"
			}
			long += fmt.Sprintf("\n  %-12s %s", spec.name, spec.short)
		} else if spec.name == command {
			use, long, example = spec.use, spec.long, spec.example
		}
	}
	if command != "" && command != "list" {
		long += "\nUse headerless output from list for TSV input; do not include --header."
		if command != "open" {
			long += "\nThese actions modify GitHub immediately without confirmation.\nUse notification thread IDs from list, not issue or pull request numbers."
		}
	}
	_, err := fmt.Fprintf(output, "Usage: gh-notifs %s\n\n%s\n\nFlags:\n%s\n\n"+
		"Flags can precede or follow the command, but must precede IDs or URLs.\n"+
		"Use -L 10 or -L=10, not -L10. After the command, -- ends flag parsing.\n\nExamples:\n%s\n",
		use, long, flagHelp, example)
	return err
}

func (opts options) run(ctx context.Context, command string, args []string, input io.Reader, output, stderr io.Writer) error {
	host, configPath := opts.host, opts.configPath
	header, noColor, limit, page := opts.header, opts.noColor, opts.limit, opts.page
	if limit < 1 || page < 1 || limit > int(^uint(0)>>1)/page {
		return fmt.Errorf("--limit and --page must be positive and their product must fit an integer")
	}
	if command == "" && page != 1 {
		return fmt.Errorf("--page requires the list command")
	}
	if host == "" {
		host, _ = auth.DefaultHost()
	}
	if host == "" || strings.ContainsAny(host, "/:@ \t\n") {
		return fmt.Errorf("invalid GitHub hostname")
	}
	if command == "open" {
		b := browser.New("", output, stderr)
		return eachInput(input, args, func(line string) error {
			fields := strings.Split(line, "\t")
			return tui.OpenBrowser(b, fields[len(fields)-1])
		})
	}
	if command != "" && command != "list" {
		client, err := notifs.NewClient(host)
		if err != nil {
			return err
		}
		return eachInput(input, args, func(line string) error {
			id, _, _ := strings.Cut(line, "\t")
			return client.Action(ctx, id, command)
		})
	}
	query, err := opts.scopedQuery(host)
	if err != nil {
		return err
	}
	client, err := notifs.NewClient(host)
	if err != nil {
		return err
	}
	inputFile, inputIsFile := input.(*os.File)
	outputFile, outputIsFile := output.(*os.File)
	if command == "" && inputIsFile && outputIsFile && term.IsTerminal(inputFile) && term.IsTerminal(outputFile) {
		if configPath == "" {
			configPath, err = config.Path()
			if err != nil {
				return err
			}
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		return tui.Run(ctx, client, tui.Options{Query: query, PageSize: limit,
			NoColor: noColor || term.IsColorDisabled(), Scopes: cfg.Scopes})
	}
	result, err := client.Fetch(ctx, query, limit*page, nil)
	if err != nil {
		return err
	}
	start := min((page-1)*limit, len(result.Items))
	items := result.Items[start:]
	if err := client.Enrich(ctx, items); err != nil {
		return err
	}
	if header {
		if _, err := fmt.Fprintln(output, Columns); err != nil {
			return err
		}
	}
	return WriteTSV(output, items)
}

func (opts options) scopedQuery(host string) (notifs.Query, error) {
	queryText, repo, org := opts.queryText, opts.repo, opts.org
	if (repo != "" && org != "") || (opts.global && (repo != "" || org != "")) {
		return notifs.Query{}, fmt.Errorf("--repo, --org, and --global are mutually exclusive")
	}
	query, err := notifs.ParseQuery(queryText)
	if err != nil {
		return notifs.Query{}, err
	}
	hasScope := false
	for _, term := range query.Terms {
		hasScope = hasScope || term.Field == "repo" || term.Field == "org"
	}
	if repo == "" && org == "" && !opts.global && !hasScope {
		if current, err := repository.Current(); err == nil && current.Host == host {
			repo = current.Owner + "/" + current.Name
		} else if err != nil && os.Getenv("GH_REPO") != "" {
			return notifs.Query{}, fmt.Errorf("resolve GH_REPO: %w", err)
		}
	}
	if repo != "" {
		parsed, err := repository.ParseWithHost(repo, host)
		if err != nil {
			return notifs.Query{}, err
		}
		if parsed.Host != host {
			return notifs.Query{}, fmt.Errorf("repository host differs from --hostname")
		}
		queryText = "repo:" + parsed.Owner + "/" + parsed.Name + " " + queryText
	}
	if org != "" {
		if strings.ContainsAny(org, " \t\n/:") {
			return notifs.Query{}, fmt.Errorf("invalid organization name")
		}
		queryText = "org:" + org + " " + queryText
	}
	return notifs.ParseQuery(strings.TrimSpace(queryText))
}

func eachInput(input io.Reader, args []string, fn func(string) error) error {
	if len(args) > 0 {
		for _, value := range args {
			if err := fn(value); err != nil {
				return err
			}
		}
		return nil
	}
	if file, ok := input.(*os.File); ok && term.IsTerminal(file) {
		return fmt.Errorf("provide arguments or pipe TSV rows to this command")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			if err := fn(line); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
