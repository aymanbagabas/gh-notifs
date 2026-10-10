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
gh notifs -R cli/cli -q 'is:unread' -n 10
```

Short flags work with both the inbox and `list`:

| Short flag | Long flag |
| --- | --- |
| `-q` | `--query` |
| `-R` | `--repo` |
| `-n`, `-L` | `--limit` |
| `-h` | `--help` |

`-R` and `-L` match GitHub CLI conventions. Here, `-q` filters notifications, not JSON through jq.
Other options retain their long names to avoid conflicting shortcut meanings.

Inside a GitHub repository, the default scope is that repository. Outside
one, it includes all repositories. Both read and unread items appear by
default. An explicit query scope, `--repo`, `--org`, or `--global` overrides
automatic repository scope. `GH_REPO` also sets the current repository.

The interface has a query bar, views and scopes, a paginated list, and an
optional Markdown preview. At narrow widths, Tab exposes the scope selector
without the sidebar. The footer shows the last successful refresh, cached
state and request errors.

Notification items fill the list width. The current item has a two-line
highlight. A blue `●` marks unread items. Hovering over a row reveals its
`✓` selection control. The control turns blue under the pointer and green
when selected. Selected rows retain the checkmark and a subtle tint after the pointer
leaves. The keyboard cursor has a stronger highlight.
Long labels and titles end with `…`.

Type icons include `⇄` for pull requests, `⊙` for issues, `◇` for discussions,
`◆` for releases, `●` for commits, and `✓` for checks. Only the type icon
uses the state color: green for open, purple for merged, red for closed,
and gray for draft or unknown states.
The second line uses muted gray, with the reference and state on the left
and the reason and update age on the right. The age has a fixed-width column,
so changes such as `30m ago` to `1h ago` do not shift the reason column.
References have OSC 8 hyperlinks; titles do not. Use your terminal's link
gesture to open a reference; ordinary clicks still focus the item.

All returned notification types remain in the list, including types with
no public detail endpoint. Those items retain their notification metadata
and can open the scoped GitHub inbox. Some types have only a basic preview.
There is no separate activity feed.

### Appearance and previews

The inbox and Markdown previews share GitHub-like dark and light palettes.
Markdown uses blue title bands, cyan headings and metadata labels, purple
emphasis, shaded quotes and inline code, and full-color syntax.
The app header, search field, and scope sidebar use the main background.
The preview header retains its subtle background.
An accent border identifies the active search field or sidebar.
Repository names use cyan; owner groups and help headings use purple.
Notification metadata stays muted, and type icons retain their state colors.
The program queries the terminal's background color at startup.
It enables light/dark reports with mode 2031 and queries the background
again when the terminal reports a theme change. Mode 2033 reports visibility,
not color changes. Terminals without color reports use the dark palette.
Use `--no-color` to disable colors.

The query editor uses the terminal cursor and scrolls long queries.
Bracketed paste preserves UTF-8 characters across byte chunks.
Previews use go-gh Markdown with Glamour. Rendered previews stay in memory
until their content, width, or theme changes.

Issue and pull request previews show metadata in a collapsible right sidebar: author,
assignees, labels, participants, milestone, and linked closing issues or pull requests.
Pull requests also show requested reviewers, review decisions, submitted reviews,
branches, the merge author, checks, and deployments when available.
Checks and deployments describe the latest pull request commit.
Each list shows up to 20 entries, its total, and a browser link for additional entries.
Metadata loads only for the selected preview, not for every inbox row.
Preview requests wait until the selection stays unchanged for 200 milliseconds.
Keyboard and mouse navigation cancel pending requests. Closing the preview
cancels its pending or active request.
If a metadata request fails, the body remains visible and the footer reports the error.
Unavailable fields do not appear as empty lists. Other notification types retain
their basic previews, with author details when the API supplies them.

Press `m` or click **metadata** or **hide metadata** to hide or show the metadata sidebar.
Drag its divider to change its width. The sidebar remembers its width
until you exit. Narrow previews switch between **Body** and **metadata**.
Click a pane, or use Alt+Left/Right, to choose where Ctrl+D/U scrolls.
The wheel scrolls the pane beneath the pointer.
Pane resizing and visibility changes do not send API requests.
Details are grouped by topic, with empty sections omitted.

The preview has three tabs: **Body** (`1`), **Discussion** (`2`), and **Threads** (`3`).
Discussion shows issue or pull request comments and submitted review messages.
Threads shows inline code-review conversations, file locations, diff context,
and resolved or outdated status. Threads apply only to pull requests.
Entries include authors, timestamps, Markdown, and links to GitHub.
Use `,` and `.` or the clickable Previous/Next controls to change discussion pages.
Each page contains up to 20 entries, in GitHub's returned order.
Each inline thread shows up to 20 comments and links to the full thread when more exist.
Discussion requests run only when you open their tab, with the same 200 ms
debounce as the body preview. Loaded pages remain in memory for the current selection.
Switching notifications cancels obsolete requests. Discussion errors stay in
their pane; they do not replace the body or metadata.

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
| `b` | Collapse or expand the sidebar |
| `Space` | Collapse or expand the focused repository group |
| `Enter` / `o` | Open the item in the browser; apply the selected sidebar view |
| `v` | Toggle the preview |
| `m` | Toggle preview details |
| `1` / `2` / `3` | Preview body / discussion / inline review threads |
| `,` / `.` | Previous / next discussion page |
| Alt+Left / Alt+Right | Focus preview body / details |
| `Ctrl+D` / `Ctrl+U` | Scroll the preview, or move through the list |
| `x` / `Shift+A` | Select one item / all loaded items |
| `Shift+X` | Clear all selections |
| `Shift+I` | Mark selected items read |
| `e` | Mark selected items done, after confirmation |
| `Shift+M` | Unsubscribe, after confirmation |
| `Shift+S` | Subscribe, after confirmation |
| `[` / `]`, Page Up / Page Down | Previous / next result page |
| `r` | Refresh immediately, unless a request is already active or GitHub has rate-limited requests |
| `i` | Return to the initial query |
| `?` | Show help |
| `q`, `Ctrl+C` | Quit |

The read, done, and unsubscribe shortcuts match GitHub's documented inbox
shortcuts. Other shortcuts are terminal conveniences. Moving the cursor or
opening a preview does not mark a notification read. Opening its website
page can change its read state on GitHub.

The page controls share the page-count row as `[ Previous` and `Next ]`.
Available buttons have an accent background; unavailable buttons use a muted
style. Page Up and Page Down also change pages.
The header shows the total page count only when all matching results are loaded.
Page navigation reuses loaded rows and fetches additional rows when needed.
The `g` and `G` shortcuts stay within the current page; they do not fetch the entire inbox.

The bottom-right counter shows the highlighted item's position across all loaded pages, not the number of checked items.
For example, press `j` from the first item to change `Item 1/50` to `Item 2/50`.
A `+` after the count means more results are available. An empty list shows `Item 0/0`.
The position shows `-` while the requested page has no loaded item.

Help expands the footer upward and reduces the inbox height above it.
Letter-based buttons underline their shortcut within the label instead of showing a separate key prefix.
Existing shortcuts stay the same, including Shift for `I`, `M`, and `S`.
The **checkbox** button toggles selection. **I read it** marks notifications read; **Mute** unsubscribes.
For example, click **Sidebar** or press its underlined `b` to hide or show the sidebar.
Number and punctuation shortcuts remain explicit. The help columns show full key combinations.
Expanded help places search prompts and examples above the Navigate, Actions,
and Search keybinding columns. Narrow terminals stack these columns.
Click a shortcut with a single action to use it.
The refresh status and action controls remain visible. Use Up/Down, Page
Up/Down, or the mouse wheel over help to scroll its text.
Press `?` or Escape, or click **Close help**, to restore the normal footer.

### Mouse and search completion

Click either line of an item to focus it. Click its `✓` control to toggle
multi-selection. Shift+click selects the range from the current item to the
clicked item, including both endpoints. It keeps other selections.
For example, click the first item, then Shift+click the fourth to select all four.
Press Shift+X to clear all selections, including those on other loaded pages.
Some terminals use Shift+click for text selection and do not send it to the app.
In those terminals, use `x` or the `✓` controls instead.

The footer controls support clicks, including Open,
Preview, Refresh, and thread actions. Confirmation prompts have clickable
Yes and No controls, with `Y` and `N` underlined. Action confirmations open as floating dialogs.
Drag a dialog's title bar to move it. Click Yes, No, or the close
button. Tab and Left/Right choose a button; Enter activates it.
Yes, No, and the close button highlight under the pointer.
No has the initial focus. `y` confirms; `n` or Escape cancels.
The dialog blocks background interaction and keeps its original notification targets.
Errors remain in the footer, and help remains in its expandable panel.

Drag the sidebar divider to change its width. Click **Sidebar** to hide
or show it. Repository entries use an accent color, with separate owner
groups. Click a group's arrow to collapse it; click its name to filter by owner.
The mouse wheel scrolls the list, sidebar, preview, or completion popup
under the pointer.

Click the query field to place the cursor. Type a field prefix, such as
`rea` or `is:`, to see suggestions. Up/Down selects a suggestion; Tab,
Enter, or a click inserts it. Enter applies the query when no suggestions
remain. Escape cancels the edit.
When suggestions extend beyond the popup, a right-edge scrollbar shows
the visible range and its position.

Completions include supported fields, types, reasons, states, and date
examples. Repository, organization, and author suggestions come from loaded
notifications. Completion does not issue API requests.

The search field supports terminal-style text editing:

| Keys | Action |
| --- | --- |
| Home / End, `Ctrl+A` / `Ctrl+E` | Move to the start / end |
| Left / Right, `Ctrl+B` / `Ctrl+F` | Move one character |
| `Alt+B` / `Alt+F`, `Ctrl+Left` / `Ctrl+Right`, `Alt+Left` / `Alt+Right` | Move by word |
| Backspace / Delete, `Ctrl+H` / `Ctrl+D` | Delete the previous / next character |
| `Ctrl+U` / `Ctrl+K` | Cut to the start / end |
| `Ctrl+W` | Cut the previous whitespace-separated term |
| `Alt+Backspace`, `Ctrl+Backspace` | Cut the previous word |
| `Alt+D`, `Ctrl+Delete` | Cut the next word |
| `Ctrl+Y` | Insert the last cut text |

Word movement treats punctuation as a boundary. `Ctrl+W` can remove a
complete term such as `repo:cli/cli`. Cut text stays in the application;
it does not change the system clipboard. Alt shortcuts require the terminal
to send Alt/Meta keys.

### Configured scopes

The interactive inbox creates `gh-notifs/config.yml` in the user configuration
directory when the file does not exist. `XDG_CONFIG_HOME` takes precedence.
Otherwise, the directory is `~/.config` on Linux, `~/Library/Application Support`
on macOS, and `%AppData%` on Windows.

The generated YAML includes instructions and the default scopes.
Existing files are never overwritten. Invalid files produce an error.
Restart the inbox after edits. List commands do not create or read this file.
Use `--config PATH` to select a different file:

```sh
gh notifs --config ./inbox.yml
```

Each scope replaces the complete current query, without an implicit
repository or organization constraint. Entries appear in file order.
Repository groups from loaded notifications appear below them.

```yaml
scopes:
  - name: Inbox
    query: ""
  - name: My reviews
    query: "reason:review-requested is:unread"
  - name: CLI issues
    query: "repo:cli/cli is:issue"
```

Use `scopes: []` to show only repository groups.
Run `go run ./examples/config` to print the documented defaults without
creating a configuration file.

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
It fetches once and does not initialize Vision.

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

The inbox fetches on startup. Press `r` to refresh it manually.
Refresh uses conditional requests with ETag and Last-Modified. A `304`
reuses the cached page. The program honors rate-limit backoff and avoids
overlapping refreshes. It revalidates loaded pages rather than assuming
a changed first page explains every older notification.

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

The preview example shows metadata above the body without an account or network:

```sh
go run ./examples/preview
```

The discussion examples show comments, review messages, and inline threads:

```sh
go run ./examples/discussion
go run ./examples/discussion --threads
go run ./examples/discussion --repo cli/cli --number 14640
```

For live results, use `--issue` for issue comments or `--threads` for inline review threads.
If another page exists, the example prints the `--cursor` argument for it.

Use an authenticated GitHub account to fetch live metadata:

```sh
go run ./examples/preview --repo cli/cli --number 14640
go run ./examples/preview --repo cli/cli --number 14627 --issue
```

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
