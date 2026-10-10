package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/tui"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
)

const help = `Usage: gh notifs [list] [options]
       gh notifs open [URL ...]
       gh notifs read|done|subscribe|unsubscribe [THREAD_ID ...]

With terminal input and output, start the notification inbox.
Otherwise, fetch once and print TSV. The list command always prints TSV.
Read and unread notifications are included by default.

Options:
  --query TEXT       Filter notifications; terms combine with AND
  --repo OWNER/REPO  Scope to a repository (-R)
  --org ORG          Scope to an organization
  --global          Ignore the current repository
  --hostname HOST   GitHub host; defaults to the gh configuration
  --limit N         Results per page (default 50; alias -n)
  --page N          List result page (default 1; list only)
  --interval TIME   Automatic refresh interval (default 5m; 0 disables)
  --no-color        Disable TUI color
  --header          Include column names in TSV
  --version         Print version
  --help            Show help

Query fields:
  repo:OWNER/REPO org:ORG reason:review-requested is:read is:unread
  is:pr is:issue is:discussion is:release is:commit is:check-suite
  author:LOGIN author:@me state:open state:closed state:merged state:draft
  since:YYYY-MM-DD before:YYYY-MM-DD "title text"
Dates use notification update times. RFC3339 timestamps also work.
Author and state filters currently apply to issues and pull requests.
Unsupported fields and website-only states produce an error.

TSV columns (no header unless --header):
  id unread updated_at last_read_at repository type reason reference
  state author title url
Times use UTC RFC3339; missing values are empty. Tabs, newlines, and
terminal controls inside values become spaces.

Examples:
  gh notifs
  gh notifs --global --query 'reason:review-requested'
  gh notifs list --query 'is:unread' --limit 10
  gh notifs list --org acme | cut -f9,11
  gh notifs list | cut -f12 | gh notifs open

Actions accept IDs as arguments, or TSV rows/IDs on stdin.
Open accepts HTTPS URLs, or TSV rows/URLs on stdin.
`

func Run(ctx context.Context, args []string, version string) error {
	command := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if command == "help" {
		fmt.Fprint(os.Stdout, help)
		return nil
	}
	switch command {
	case "", "list", "open", "read", "done", "subscribe", "unsubscribe":
	default:
		return fmt.Errorf("unknown command %q; use --help", command)
	}
	flags := flag.NewFlagSet("gh notifs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var queryText, repo, org, host string
	var global, header, noColor, showVersion bool
	var limit, page int
	var interval time.Duration
	flags.StringVar(&queryText, "query", "", "notification query")
	flags.StringVar(&repo, "repo", "", "repository scope")
	flags.StringVar(&repo, "R", "", "repository scope")
	flags.StringVar(&org, "org", "", "organization scope")
	flags.StringVar(&host, "hostname", "", "GitHub host")
	flags.BoolVar(&global, "global", false, "all repositories")
	flags.BoolVar(&header, "header", false, "TSV header")
	flags.BoolVar(&noColor, "no-color", false, "disable colors")
	flags.BoolVar(&showVersion, "version", false, "show version")
	flags.IntVar(&limit, "limit", 50, "result limit")
	flags.IntVar(&limit, "n", 50, "result limit")
	flags.IntVar(&page, "page", 1, "result page")
	flags.DurationVar(&interval, "interval", 5*time.Minute, "automatic refresh interval; 0 disables")
	flags.Usage = func() { fmt.Fprint(flags.Output(), help) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if showVersion {
		fmt.Fprintln(os.Stdout, version)
		return nil
	}
	if limit < 1 || page < 1 || limit > int(^uint(0)>>1)/page {
		return fmt.Errorf("--limit and --page must be positive and their product must fit an integer")
	}
	if interval < 0 {
		return fmt.Errorf("--interval must be zero or a positive duration")
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
		b := browser.New("", os.Stdout, os.Stderr)
		return eachInput(flags.Args(), func(line string) error {
			fields := strings.Split(line, "\t")
			return tui.OpenBrowser(b, fields[len(fields)-1])
		})
	}
	if command != "" && command != "list" {
		client, err := notifs.NewClient(host)
		if err != nil {
			return err
		}
		return eachInput(flags.Args(), func(line string) error {
			id, _, _ := strings.Cut(line, "\t")
			return client.Action(ctx, id, command)
		})
	}
	if len(flags.Args()) > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Args()[0])
	}
	if (repo != "" && org != "") || (global && (repo != "" || org != "")) {
		return fmt.Errorf("--repo, --org, and --global are mutually exclusive")
	}
	query, err := notifs.ParseQuery(queryText)
	if err != nil {
		return err
	}
	hasScope := false
	for _, term := range query.Terms {
		hasScope = hasScope || term.Field == "repo" || term.Field == "org"
	}
	if repo == "" && org == "" && !global && !hasScope {
		if current, err := repository.Current(); err == nil && current.Host == host {
			repo = current.Owner + "/" + current.Name
		} else if err != nil && os.Getenv("GH_REPO") != "" {
			return fmt.Errorf("resolve GH_REPO: %w", err)
		}
	}
	if repo != "" {
		parsed, err := repository.ParseWithHost(repo, host)
		if err != nil {
			return err
		}
		if parsed.Host != host {
			return fmt.Errorf("repository host differs from --hostname")
		}
		queryText = "repo:" + parsed.Owner + "/" + parsed.Name + " " + queryText
	}
	if org != "" {
		if strings.ContainsAny(org, " \t\n/:") {
			return fmt.Errorf("invalid organization name")
		}
		queryText = "org:" + org + " " + queryText
	}
	query, err = notifs.ParseQuery(strings.TrimSpace(queryText))
	if err != nil {
		return err
	}
	client, err := notifs.NewClient(host)
	if err != nil {
		return err
	}
	if command == "" && term.IsTerminal(os.Stdin) && term.IsTerminal(os.Stdout) {
		return tui.Run(ctx, client, tui.Options{Query: query, PageSize: limit, Interval: interval, NoColor: noColor || term.IsColorDisabled()})
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
		if _, err := fmt.Fprintln(os.Stdout, Columns); err != nil {
			return err
		}
	}
	return WriteTSV(os.Stdout, items)
}

func eachInput(args []string, fn func(string) error) error {
	if len(args) > 0 {
		for _, value := range args {
			if err := fn(value); err != nil {
				return err
			}
		}
		return nil
	}
	if term.IsTerminal(os.Stdin) {
		return fmt.Errorf("provide arguments or pipe TSV rows to this command")
	}
	scanner := bufio.NewScanner(os.Stdin)
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
