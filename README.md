# gh-notifs

A GitHub notification inbox for the terminal, built with
[go-gh](https://github.com/cli/go-gh) and
[Vision](https://altscreen.io/vision).

Run `gh notifs` for a full-screen inbox. Pipe the same command to get TSV
instead. The program uses actual notification threads, not a search for
unread issues.

## Build and install

You need Go 1.26.5 or later. Vision is currently private, so source builds
require access to `altscreen/vision`. Published binaries do not need that
source access.

```sh
git clone https://github.com/aymanbagabas/gh-notifs
cd gh-notifs
GOPRIVATE=altscreen.io/vision go mod download
make build
gh extension install .
gh notifs
```

Use your normal Git credentials for private dependencies. If you already
set `GOPRIVATE`, add Vision to that list rather than replace it.

You can also run `./bin/gh-notifs` as a standalone program. It uses the account
and host configured by `gh`, or the authentication environment variables
supported by go-gh, such as `GH_TOKEN` and `GH_HOST`. It does not need to
start a `gh` process for API requests.

The token must support the notifications API and have access to the
repositories you use. Permission failures appear as errors.

Once a binary release is published:

```sh
gh extension install aymanbagabas/gh-notifs
gh extension upgrade notifs
```

The Go rewrite replaces the Bash implementation. The root `gh-notifs` file
is only a two-line launcher for local extension development. Released
extensions run the compiled binary directly. On Windows, build with
`go build -o gh-notifs.exe .` for local extension installation.

The old flags, fzf
interface, and eight-column output are not compatible with this version.

## Inbox

```sh
gh notifs
gh notifs --global
gh notifs --repo cli/cli
gh notifs --org acme
gh notifs --global --query 'is:unread reason:review-requested'
gh notifs --interval 5m
gh notifs --interval 0
```

Inside a GitHub repository, the default scope is that repository. Outside
one, it includes all repositories. Both read and unread items appear by
default. An explicit query scope, `--repo`, `--org`, or `--global` overrides
automatic repository scope. `GH_REPO` also sets the current repository.

The interface has a query bar, views and scopes, a paginated list, and an
optional Markdown preview. At narrow widths, Tab exposes the scope selector
without the sidebar. The footer shows the last successful refresh, cached
state, the next check, and request errors.

All returned notification types remain in the list, including types with
no public detail endpoint. Those items retain their notification metadata
and can open the scoped GitHub inbox. Some types have only a basic preview.
There is no separate activity feed or full discussion timeline.

### Appearance and previews

The inbox and Markdown previews share GitHub-like dark and light palettes.
The program queries the terminal's background color at startup.
It enables light/dark reports with mode 2031 and queries the background
again when the terminal reports a theme change. Mode 2033 reports visibility,
not color changes. Terminals without color reports use the dark palette.
Use `--no-color` to disable colors.

The query editor uses the terminal cursor and scrolls long queries.
Bracketed paste preserves UTF-8 characters across byte chunks.
Previews use go-gh Markdown with Glamour. Rendered previews stay in memory
until their content, width, or theme changes.

The `themes/` directory contains both palettes as Glamour JSON files.
Other Glamour programs can use these files through their custom-style options.
For programs that support it, set `GLAMOUR_STYLE` to the file's absolute path.
This inbox always uses its shared palette rather than `GLAMOUR_STYLE`.

### Keyboard shortcuts

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move |
| `g` / `G`, Home / End | First / last item on the current page or focused sidebar |
| `/` or `s` | Edit query; Enter applies; Escape cancels |
| `Tab` | Switch between the list and scope selector |
| `Enter` / `o` | Open the item in the browser; apply the selected sidebar view |
| `v` | Toggle the preview |
| `Ctrl+D` / `Ctrl+U` | Scroll the preview, or move through the list |
| `x` / `Shift+A` | Select one item / all loaded items |
| `Shift+I` | Mark selected items read |
| `e` | Mark selected items done, after confirmation |
| `Shift+M` | Unsubscribe, after confirmation |
| `Shift+S` | Subscribe, after confirmation |
| `[` / `]`, Page Up / Page Down | Previous / next result page |
| `r` | Refresh immediately, unless a request is already active or GitHub has rate-limited requests |
| `i` | Return to the initial query |
| `?` | Show help |
| `q`, Escape, `Ctrl+C` | Quit |

The read, done, and unsubscribe shortcuts match GitHub's documented inbox
shortcuts. Other shortcuts are terminal conveniences. Moving the cursor or
opening a preview does not mark a notification read. Opening its website
page can change its read state on GitHub.

The page controls appear above the list as `[ Previous` and `Next ]`.
Unavailable directions appear dimmed. Page Up and Page Down also change pages.
The header shows the total page count only when all matching results are loaded.
Page navigation reuses loaded rows and fetches additional rows when needed.
The `g` and `G` shortcuts stay within the current page; they do not fetch the entire inbox.

### Queries

Terms combine with **AND**. Quote title phrases that contain spaces.
Queries are applied to notification threads, not to a separate search result.

| Query | Meaning |
| --- | --- |
| `repo:OWNER/REPO` | Repository |
| `org:OWNER` | Repository owner or organization |
| `reason:review-requested` | Actual notification reason |
| `is:read`, `is:unread` | Notification read state |
| `is:pr`, `is:issue`, `is:issue-or-pull-request` | Issue or pull request type |
| `is:discussion`, `is:release`, `is:commit`, `is:check-suite` | Other subject types |
| `author:LOGIN`, `author:@me` | Issue or pull request author |
| `state:open`, `state:closed`, `state:merged`, `state:draft` | Issue or pull request state |
| `since:2026-10-01` | Notification updated strictly after midnight UTC |
| `before:2026-11-01` | Notification updated strictly before midnight UTC |
| `"slow uploads"` | Case-insensitive title substring |

Dates also accept RFC3339 timestamps. Type filters additionally support
`is:gist`, `is:repository-invitation`, `is:repository-advisory`, and
`is:repository-vulnerability-alert`.

Reason values come from REST, such as `mention`, `team-mention`, `assign`,
`comment`, `author`, `manual`, `subscribed`, and `ci-activity`. Underscores
and hyphens are equivalent in reason values. Type and reason filters match
local lowercase, hyphen-separated names. The TUI displays those same names:
`PullRequest` becomes `pull-request`, and `review_requested` becomes
`review-requested`. TSV retains the original API values.

Author and state filters need extra details and currently match only issues
and pull requests. Unsupported fields produce errors. There is no OR or
negation syntax.

### Public API boundaries

The program uses supported public APIs only.

- Supported actions: mark read, mark done, subscribe, and unsubscribe.
- Not supported: Saved, Done history, restore, and mark unread.
- Website custom filters do not synchronize with this program.
- Scopes mean repositories and organizations, not GitHub Projects.

The tool does not simulate unsupported GitHub state in a local database.

## TSV and scripts

Without terminal input and output, the default command fetches once,
prints TSV, and exits. `list` always prints TSV, even on a terminal.
It does not start polling or initialize Vision.

```sh
gh notifs list --limit 10
gh notifs list --query 'is:unread' --header
gh notifs list --org acme --query 'reason:review-requested' | cut -f8,11
gh notifs list --page 2 --limit 20
gh notifs --global | cut -f12
```

The default page size is 50. Filtered pages can require multiple API pages.
The UI labels the number of **loaded** results, not the total inbox size.

The fixed columns are:

| Field | Value |
| --- | --- |
| 1 | Notification thread ID |
| 2 | Unread: `true` or `false` |
| 3 | Notification update time, UTC RFC3339 |
| 4 | Last-read time, UTC RFC3339, or empty |
| 5 | `owner/repository` |
| 6 | GitHub subject type |
| 7 | GitHub notification reason |
| 8 | `owner/repository#number`, or the repository when no number is available |
| 9 | Issue or PR state, or empty |
| 10 | Issue or PR author, or empty |
| 11 | Title |
| 12 | Browser URL |

There is no header unless `--header` is set. Tabs, newlines, terminal
controls, and Unicode format controls inside fields become spaces.
Output contains no color codes or hyperlinks. Errors go to stderr with a
nonzero exit status; failed list requests do not silently print cached data.

Actions accept thread IDs or full TSV rows:

```sh
gh notifs read 123456789
gh notifs done 123456789
gh notifs unsubscribe 123456789
gh notifs subscribe 123456789
```

These commands modify GitHub immediately. Unlike the TUI, they do not ask
for confirmation. Use IDs returned by your own inbox.

Open accepts HTTPS URLs or full TSV rows:

```sh
gh notifs list | cut -f12 | fzf | gh notifs open
```

## Requests, refresh, and cache

REST supplies thread identity, read state, reason, and notification times.
GraphQL adds issue and PR state and author data. It queries the exact
subjects from the inbox rather than searching for potentially different items.

The TUI displays REST rows before detail requests finish. Details use
batches of at most 25 subjects and at most four concurrent requests.
Unchanged details stay in memory. Previews load only when requested.
Scope changes cancel obsolete work, and late results cannot replace the
current view.

Refresh uses conditional requests with ETag and Last-Modified. A `304`
reuses the cached page. Automatic refresh defaults to five minutes.
Use `--interval 2m` to change the interval, or `--interval 0` to disable it.
The initial fetch still runs when automatic refresh is disabled.
Automatic refresh honors the longer of the configured interval and
GitHub's `X-Poll-Interval`. The timer starts after each refresh finishes.
Press `r` to refresh immediately without waiting for that timer.
The program always honors rate-limit backoff,
avoids overlapping refreshes, and pauses automatic refresh when
the terminal reports lost focus. It revalidates loaded pages rather than
assuming a changed first page explains every older notification.

The metadata cache is under the operating system's user cache directory,
inside `gh-notifs/`. The host and numeric GitHub user ID separate inbox caches.
Each unfamiliar token requires one `/user` lookup. A local mapping associates
its fingerprint with the verified account. Later starts reuse that mapping
without another identity request. Token rotation preserves the account's inbox cache.
Identity requests run outside the UI loop. The token itself is never stored there.
Files use owner-only permissions on Unix. Cached titles can contain private
information. Preview bodies are not persisted.

Successful refreshes replace page contents. Successful actions invalidate
cached pages. Entries older than seven days are removed on cache writes.
The UI can show cached rows during a refresh or a network failure, with the
cached state and errors visible.

## Development and examples

```sh
make test
make check
make example
make build
```

`go run ./examples/tsv` prints a synthetic notification through the actual
TSV formatter. It needs no account or network. API tests use local HTTP
fixtures; renderer tests exercise Vision at multiple terminal sizes.

The theme example also needs no account or network:

```sh
go run ./examples/theme
go run ./examples/theme --light
go run ./examples/theme --export themes
```

The export command regenerates both shipped Glamour files from the shared palette.

For a real inbox example, use a prepared public account:

```sh
./gh-notifs --global --query 'is:unread'
./gh-notifs list --global --limit 5 | cut -f8,11
```

The old Bash demo shim and its recording are removed. Record the current
binary with public data; never publish a private inbox recording.

CI and release builds require a `VISION_READ_TOKEN` Actions secret with
read access to the private Vision repository. The workflow reports a
dependency access failure instead of bypassing it. After Vision becomes
public, this extra credential will no longer be necessary.

## License

MIT. See [LICENSE](LICENSE).
