package cli

import (
	"bufio"
	"context"
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
	"github.com/spf13/cobra"
)

const description = `With terminal input and output, start the notification inbox.
Otherwise, fetch once and print TSV. The list command always prints TSV.
Read and unread notifications are included by default.

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
terminal controls inside values become spaces.`

type options struct {
	queryText, repo, org, host, configPath string
	global, header, noColor                bool
	limit, page                            int
}

func Run(ctx context.Context, args []string, version string) error {
	root, err := newCommand(version)
	if err != nil {
		return err
	}
	root.SetArgs(args)
	root.SetIn(os.Stdin)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.ExecuteContext(ctx)
}

func newCommand(version string) (*cobra.Command, error) {
	var opts options
	root := &cobra.Command{
		Use: "gh-notifs", Short: "A GitHub notification inbox for the terminal",
		Long: description, Version: version,
		SilenceUsage: true, SilenceErrors: true,
		Args: cobra.NoArgs, ValidArgsFunction: cobra.NoFileCompletions,
		Example: `  gh-notifs
  gh-notifs --global -q 'reason:review-requested'
  gh-notifs list -R cli/cli -q 'is:unread' -L 10
  gh-notifs list -L 10 --header
  gh-notifs list -L 1 | gh-notifs open
  gh-notifs read 123456789
  gh-notifs done 123456789
  gh-notifs subscribe 123456789
  gh-notifs unsubscribe 123456789`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run(cmd, "", args)
		},
	}
	root.SetVersionTemplate("{{.Root.Version}}\n")
	flags := root.PersistentFlags()
	flags.StringVarP(&opts.queryText, "query", "q", "", "Filter notifications with a `query`; terms combine with AND")
	flags.StringVarP(&opts.repo, "repo", "R", "", "Scope to `[HOST/]OWNER/REPO`")
	flags.StringVar(&opts.org, "org", "", "Scope to an `organization`")
	flags.StringVar(&opts.host, "hostname", "", "GitHub `host`; defaults to the gh configuration")
	flags.StringVar(&opts.configPath, "config", "", "Scope configuration `file` (interactive inbox only)")
	flags.BoolVar(&opts.global, "global", false, "Ignore the current repository")
	flags.BoolVar(&opts.header, "header", false, "Include column names in TSV")
	flags.BoolVar(&opts.noColor, "no-color", false, "Disable TUI color")
	flags.Bool("version", false, "Print version")
	flags.IntVarP(&opts.limit, "limit", "L", 50, "Results per page")
	flags.IntVar(&opts.page, "page", 1, "List result page (list only)")
	if err := root.MarkPersistentFlagFilename("config"); err != nil {
		return nil, err
	}
	for _, name := range []string{"query", "repo", "org", "hostname", "limit", "page"} {
		if err := root.RegisterFlagCompletionFunc(name, cobra.NoFileCompletions); err != nil {
			return nil, err
		}
	}
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
		cmd := &cobra.Command{
			Use: spec.use, Short: spec.short, Long: spec.long, Example: spec.example, Version: version,
			ValidArgsFunction: cobra.NoFileCompletions,
			RunE: func(cmd *cobra.Command, args []string) error {
				return opts.run(cmd, spec.name, args)
			},
		}
		if spec.name == "list" {
			cmd.Args = cobra.NoArgs
		} else {
			cmd.Long += "\nUse headerless output from list for TSV input; do not include --header."
			if spec.name != "open" {
				cmd.Long += "\nThese actions modify GitHub immediately without confirmation.\nUse notification thread IDs from list, not issue or pull request numbers."
			}
		}
		root.AddCommand(cmd)
	}
	root.AddCommand(&cobra.Command{
		Use: "completion <shell>", Short: "Generate shell completions for standalone gh-notifs",
		Long:    "Generate a shell completion script for standalone gh-notifs.\nThis does not configure completion for the gh notifs extension command.",
		Version: version, Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Example:   "  gh-notifs completion zsh > _gh-notifs",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported completion shell %q", args[0])
			}
		},
	})
	return root, nil
}

func (opts options) run(cmd *cobra.Command, command string, args []string) error {
	ctx := cmd.Context()
	queryText, repo, org, host, configPath := opts.queryText, opts.repo, opts.org, opts.host, opts.configPath
	global, header, noColor, limit, page := opts.global, opts.header, opts.noColor, opts.limit, opts.page
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
		b := browser.New("", cmd.OutOrStdout(), cmd.ErrOrStderr())
		return eachInput(cmd.InOrStdin(), args, func(line string) error {
			fields := strings.Split(line, "\t")
			return tui.OpenBrowser(b, fields[len(fields)-1])
		})
	}
	if command != "" && command != "list" {
		client, err := notifs.NewClient(host)
		if err != nil {
			return err
		}
		return eachInput(cmd.InOrStdin(), args, func(line string) error {
			id, _, _ := strings.Cut(line, "\t")
			return client.Action(ctx, id, command)
		})
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
	input, inputIsFile := cmd.InOrStdin().(*os.File)
	output, outputIsFile := cmd.OutOrStdout().(*os.File)
	if command == "" && inputIsFile && outputIsFile && term.IsTerminal(input) && term.IsTerminal(output) {
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
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), Columns); err != nil {
			return err
		}
	}
	return WriteTSV(cmd.OutOrStdout(), items)
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
