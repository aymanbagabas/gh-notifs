# gh-notifs

A GitHub notification inbox for your terminal.
Browse notifications, read conversations and reviews, and manage your inbox with the keyboard or mouse.
Use the same program in scripts through tab-separated output.

Built in Go with [go-gh](https://github.com/cli/go-gh) and [Vision](https://altscreen.io/vision).

## Get started

Install [GitHub CLI](https://cli.github.com/), then authenticate:

```sh
gh auth login
```

Skip `gh auth login` if your account is already configured.
Your credentials must permit access to notifications and the relevant repositories.
Release binaries do not require Go.

### GitHub CLI extension

```sh
gh extension install aymanbagabas/gh-notifs
gh notifs
```

To update the extension:

```sh
gh extension upgrade notifs
```

GitHub CLI selects the release binary for your operating system and architecture.

### Standalone binary

Download a binary from the [latest release](https://github.com/aymanbagabas/gh-notifs/releases/latest).
Choose your operating system and architecture:

| Operating system | Asset suffixes |
| --- | --- |
| Linux | `linux-amd64`, `linux-arm64` |
| macOS | `darwin-amd64` (Intel), `darwin-arm64` (Apple silicon) |
| Windows | `windows-amd64.exe`, `windows-arm64.exe` |

Rename the downloaded file to `gh-notifs` and place it on your PATH.
On Windows, name it `gh-notifs.exe`.
On Linux and macOS, make it executable:

```sh
chmod +x gh-notifs
```

Run `gh-notifs` to open the inbox.

The examples below use `gh-notifs`; `gh notifs` accepts the same arguments.
Both forms use GitHub CLI credentials, including supported environment variables such as `GH_TOKEN` and `GH_HOST`.

Use `gh-notifs --help` for all flags, or `gh-notifs help read` for a command's examples.
Flags can precede or follow the command, but must precede positional IDs or URLs:

```sh
gh-notifs -R cli/cli list -L 10 --header
gh-notifs list -R cli/cli -L=10 --header
gh-notifs read --hostname github.com 123456789
```

Separate flag names and values with a space or `=`.
Use `-L 10` or `-L=10`, not `-L10`.
After the command, `--` ends flag parsing.
Shell completion scripts are not provided.
The inbox's query editor has built-in completions.

## Browse your inbox

```sh
gh-notifs
gh-notifs --global
gh-notifs -R cli/cli
gh-notifs --org acme
gh-notifs --global -q 'is:unread reason:review-requested' -L 10
```

Inside a GitHub repository, the inbox defaults to that repository.
Use `--global` for all repositories, `-R` for one repository, or `--org` for an organization.
Read and unread notifications appear by default.
`-L` sets the page size; the default is 50.
Use `--hostname` to select another GitHub host.

The inbox shows notification reasons, read state, and issue or pull request details.
Each row shows the available author as `@login` before the right-aligned reason and age.
Author and reason columns fit the widest loaded values, with two-space separators and a fixed age column.
Long author names are truncated; very narrow rows omit them. This uses existing details without extra API requests.
Previews include Markdown, metadata, comments, submitted reviews, and inline review threads where available.
The interface follows the terminal's light or dark theme. Use `--no-color` to disable colors.

### Controls

| Key | Action |
| --- | --- |
| Up / Down, `k` / `j` | Move between items |
| `/` | Edit the query |
| `Ctrl+Space` | Show completions in the query editor |
| `Tab` | Switch between the list and scopes |
| `b` | Show or hide the sidebar |
| `v` | Show or hide the preview |
| `1` / `2` / `3` | Show Body / Discussion / Threads |
| `m` | Show or hide preview metadata |
| `,` / `.` | Previous / next discussion page |
| `[` / `]` | Previous / next notification page |
| `Enter` / `o` | Open the current item in the browser |
| `x` | Toggle selection |
| `Shift+A` / `Shift+X` | Select all loaded items / clear all selections |
| `Shift+I` | Mark items read |
| `e` | Mark items done, after confirmation |
| `Shift+M` / `Shift+S` | Unsubscribe / subscribe, after confirmation |
| `r` | Refresh |
| `?` | Show full help |
| `q` / `Ctrl+C` | Quit |

Actions use the selected items, or the current item when nothing is selected.
Mark read acts immediately.
Escape cancels a query edit or closes help or a dialog; it does not quit.
An empty search shows all filter names. Press Tab to insert the first filter, or use Up/Down to choose one.
Enter applies an empty query until you select a suggestion.
Ctrl+Space opens completions at the cursor, including filter names after a space and values after a filter's colon.
For example, type `is:unread `, then press Ctrl+Space to choose another filter.
Each suggestion has a short description. For example, `reason:mention` shows "You were directly @mentioned".
Select a suggestion to insert only its query text, without the description.
Suggestions also appear as you type. Enter accepts a selected suggestion, or applies the query when none is selected.

Click a row to focus it or its checkmark to select it.
Shift+click selects a range from the current item.
Scroll the pane under the pointer, or drag a divider to resize it.
Some terminals reserve Shift+click for text selection; use `x` instead.

Previewing a notification does not mark it read.
Opening its web page can change its read state on GitHub.
The footer shows the cursor position, and the terminal title shows the loaded view's unread count.
A `+` means more results exist.

The inbox fetches on startup and refreshes when you press `r`.
Notification pages persist across launches in the system cache directory under `gh-notifs`.
Cached pages include author names, author IDs, issue/PR states, and URLs.
The inbox shows saved details immediately and retains them during refresh while the notification timestamp remains unchanged.
Each session still fetches details in the background to update stale names and states.
Author and state filters wait for detail validation rather than showing unverified cached matches.
Notifications without saved details require their first detail fetch.
Repository history merges observations from separate list and action processes.
Actions also persist an account-specific invalidation generation with the cached pages.
A later save from another process cannot restore pre-action pages.
Detail writes use the same invalidation checks and update only matching entries that still exist.
Saves merge newer persisted pages, so a late detail response cannot replace another process's newer notification refresh.
An active inbox applies these invalidations on its next fetch.
Notification pages and their invalidation generation share `inbox-v1.json`.
Previews and conversations stay in memory.
Cached notifications can contain private repository information.

## Filter notifications

Repeated values are alternatives within each group: repositories, owners, authors, reasons, issue states, types, or read states.
Every group must match.
Type and read state are separate groups, even though both use `is:`.
For example, `is:issue-or-pull-request is:release is:unread` selects unread issues, pull requests, and releases.
`is:read is:unread` includes both read states.

Every title term and date bound must match.
`AND` and `OR` are ordinary title words, not query operators.
`-R` and `--org` add alternatives to their respective `repo:` and `org:` groups.
For example, `-R cli/cli -q 'repo:cli/go-gh'` includes both repositories.

```sh
gh-notifs --global -q 'is:unread reason:mention'
gh-notifs --global -q 'reason:participating is:unread'
gh-notifs -R cli/cli -q 'is:issue-or-pull-request state:open'
gh-notifs --global -q 'author:@me since:2026-10-01'
gh-notifs --global -q '"slow uploads"'
gh-notifs --global -q 'reason:mention reason:comment is:unread'
gh-notifs -q 'repo:cli/cli repo:cli/go-gh is:issue-or-pull-request is:release'
```

| Filter | Examples |
| --- | --- |
| Repository or owner | `repo:cli/cli`, `org:acme` |
| Read state | `is:read`, `is:unread` |
| Type | `is:issue`, `is:pr`, `is:pull-request`, `is:issue-or-pull-request`, `is:discussion`, `is:release`, `is:commit` |
| Notification reason | `reason:mention`, `reason:participating`, `reason:review-requested`, `reason:subscribed` |
| Author | `author:octocat`, `author:@me` |
| Issue or pull request state | `state:open`, `state:closed`, `state:merged`, `state:draft` |
| Update time | `since:2026-10-01`, `before:2026-11-01` |
| Title text | `"slow uploads"` |

Use `is:issue-or-pull-request` for both issues and pull requests.
`is:issue` selects only issues. `is:pr` and `is:pull-request` select only pull requests.
For example, `is:pr is:unread` selects unread pull requests, while `is:issue is:pr is:unread` selects both unread types.
These filters act locally and intentionally differ from the website's [combined type behavior](https://docs.github.com/en/subscriptions-and-notifications/reference/inbox-filters#custom-filter-limitations).
Author and state filters apply to issues and pull requests.
Author filters match stable GitHub user IDs, not cached display names.
`author:@me` reuses your authenticated account ID without another profile request.
Each distinct author name requires one profile lookup per client session; subsequent fetches reuse that identity and cached issue/PR details.
A resolved name continues to identify that account during the session, even after a rename.
After restart, names resolve again; an old name may be unavailable or belong to another account.
Unknown names and failed profile lookups produce an error.
Cached display names can remain unchanged until the notification details refresh.
`reason:participating` uses the REST parameter `participating=true`; it is not a notification reason value.
Requests also use this optimization when every reason alternative is `author`, `comment`, `mention`, or `participating`.
Specific reason filters still apply locally.
When other alternatives accompany `reason:participating`, separate requests preserve participation membership and the other reasons.
For example, `reason:participating reason:subscribed` merges participating notifications with subscribed notifications.
The public API does not guarantee exact equivalence with the website's participation filter.
Dates use notification update times and accept `YYYY-MM-DD` or RFC3339.
Date-only values use midnight UTC; `since` and `before` exclude the boundary itself.
Fractional RFC3339 bounds retain their precision in the local filter.
REST requests use whole seconds: `since` rounds down, and a fractional `before` rounds up.
These wider request bounds avoid the loss of locally matching notifications.
Repeated dates use the latest `since` and earliest `before`, regardless of term order.
Use `gh-notifs --help` for more query examples.

## Use notifications in scripts

`list` always prints TSV. The root command also prints TSV when input or output is not a terminal.

```sh
gh-notifs list -R cli/cli -L 10 --header
gh-notifs list --global -q 'is:unread' | cut -f8,11
gh-notifs list -R cli/cli --page 2 -L 20
gh-notifs --global | cut -f12
```

Each row has these columns:

| Column | Value |
| --- | --- |
| 1 | Notification thread ID |
| 2 | Unread: `true` or `false` |
| 3 | Updated time |
| 4 | Last-read time |
| 5 | Repository |
| 6 | Subject type |
| 7 | Notification reason |
| 8 | Reference, such as `cli/cli#123` |
| 9 | Issue or pull request state |
| 10 | Author |
| 11 | Title |
| 12 | Browser URL |

Times use UTC RFC3339. Missing values are empty.
Output has no header unless you pass `--header`, and contains no terminal styling.
Errors go to stderr with a nonzero exit status.

### Perform actions

**Warning: Check the targets before you run an action.**
CLI actions change GitHub immediately, without confirmation.
Use notification thread IDs from `list`, not issue or pull request numbers.
Replace the example IDs with your own.

```sh
gh-notifs read 123456789 987654321
gh-notifs done 123456789
gh-notifs subscribe 123456789
gh-notifs unsubscribe 123456789
```

Each action also accepts IDs or complete TSV rows on stdin.
Do not include `--header` in action input.
For example, save a list and inspect it before you mark those notifications read:

```sh
gh-notifs list -R cli/cli -q 'is:unread' -L 10 > selected.tsv
cut -f1,8,11 selected.tsv
gh-notifs read < selected.tsv
```

To mark a filtered batch as done:

```sh
gh-notifs list -R cli/cli -q 'is:read' -L 10 | gh-notifs done
```

Actions stop at the first error. Earlier successful changes remain applied.
Use an action's `--help` for more examples.

### Open in the browser

`open` accepts HTTPS URLs or complete TSV rows:

```sh
gh-notifs open https://github.com/cli/cli/issues/1
gh-notifs list -R cli/cli -L 1 | gh-notifs open
```

## Configure sidebar scopes

The inbox creates `gh-notifs/config.yml` in your user configuration directory on its first launch.
Use `$XDG_CONFIG_HOME` when set; otherwise, use `~/.config` on Linux, `~/Library/Application Support` on macOS, or `%AppData%` on Windows.
You can select another file with `--config`:

```sh
gh-notifs --config ./inbox.yml
```

```yaml
scopes:
  - name: Inbox
    query: ""
  - name: My reviews
    query: "is:unread reason:review-requested"
  - name: CLI issues and pull requests
    query: "repo:cli/cli is:issue-or-pull-request"
```

Each scope replaces the complete query, including any repository filter.
Restart the inbox after edits.
Use `scopes: []` to hide configured scopes; the Repositories section remains available.
Noninteractive commands do not read or create this file.

### Repository history

The sidebar shows up to 25 repositories from fetched notifications, newest notification activity first.
Configured scopes appear above Repositories, with a blank row between the sections.
Click a repository to replace the query with its `repo:` filter.
The full history supplies `repo:` completions, including repositories outside the displayed 25.
For example, type `repo:` in the query editor to find an older repository.

All observed repositories persist across launches, separately for each GitHub host and account.
Read, done, and subscription actions do not erase this history.
The history uses a separate `repositories-v1.json` file beside the notification cache.
It can contain private repository names.
It requires no extra API requests and includes only repositories the app has fetched.
It does not show unread counts or remove repositories when their notifications disappear.

## API limits

REST provides notifications and read state. GraphQL adds issue and pull request details.
API requests fetch up to 50 notifications globally or 100 from a single repository.
Each distinct `repo:` value uses its repository endpoint. Multiple repository requests run concurrently and paginate independently.
Results merge by notification ID and sort by newest notification update time before the shared result limit applies.
The cache follows the same request plan. A failed request produces an error, not a successful partial result.
The `-L` option sets the displayed result count, independently of the API page size.
Some notification types have only a basic preview.
The supported API does not provide mark unread, saved items, or done history.
Website filters do not synchronize with sidebar scopes.

## Development

Source builds need Go 1.26.5 or later.

```sh
git clone https://github.com/aymanbagabas/gh-notifs
cd gh-notifs
go mod download
make build
./bin/gh-notifs
```

To test a local extension, build the executable at the repository root:

```sh
go build -o gh-notifs .
gh extension install .
```

On Windows, use `go build -o gh-notifs.exe .`.

```sh
make build    # Build bin/gh-notifs
make test     # Run tests with the race detector
make check    # Run go vet
make example  # Run offline examples
```

The [examples](examples) cover repository history, API queries, TSV output, themes, configuration, previews, and discussions.

### Release process

The [Build workflow](.github/workflows/build.yml) tests the application and builds binaries for Linux, macOS, and Windows on amd64 and arm64.
When a `v*` tag is pushed, it publishes the six binaries to GitHub Releases with generated release notes.
Each binary embeds the tag as its version.
Asset names end in `OS-ARCH`, with `.exe` on Windows, as required by `gh extension install`.

The workflow's `GITHUB_TOKEN` publishes the release; no separate publication token is required.
After the checks pass and the changes are committed, push a version tag such as `v1.0.0` to publish.

## License

[MIT](LICENSE).
