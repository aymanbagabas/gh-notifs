# gh-notifs

Your GitHub inbox, in the terminal. Read conversations, filter notifications, and mark items read or done with the keyboard or mouse.

![gh-notifs: browse notifications, preview a pull request, and filter the inbox](assets/demo.gif)

*The demo uses fictional notifications.*

## Install

Install [GitHub CLI](https://cli.github.com/). Sign in with `gh auth login`. Then run:

```sh
gh extension install aymanbagabas/gh-notifs
gh notifs
```

Update with `gh extension upgrade notifs`. Standalone binaries are available on the [releases page](https://github.com/aymanbagabas/gh-notifs/releases/latest).
They use the same GitHub CLI credentials.

## Use

The inbox shows read and unread notifications across all repositories. Press `/` to filter.
You can also start with a query:

```sh
gh notifs -q 'is:unread reason:review-requested'
gh notifs -q 'repo:cli/cli'
gh notifs -q 'org:acme'
gh notifs -q 'author:@me state:open'
```

Use the arrow keys to move between items. Press `v` for a preview. Press `Enter` to open the item on GitHub.
Press `x` to select items. Press `Shift+I` to mark them read. Press `e` to mark them done.

Press `?` for all controls. Press `q` to quit.

Previews do not mark notifications read. Enable automatic refresh and notification bells with `gh notifs --auto-refresh --bell`.
Both are off by default.

## Use with pipes

`list` shows a compact table in a terminal, with headers, colors, and relative times.
Its columns are READ, REFERENCE, TITLE, STATE, REASON, and UPDATED. READ shows `yes` for read notifications and `no` for unread notifications.
Use `--no-color` or `NO_COLOR=1` to disable colors.

Terminal lists use `GH_PAGER`, the GitHub CLI `pager` setting, or `PAGER`, in that order.
For example, use `GH_PAGER=less gh notifs list -L 100`.
Set `GH_PAGER=cat` or `GH_PAGER=""` to disable the pager. Without a configured pager, output goes directly to the terminal.
For `less`, the default `LESS=FRX` preserves colors and exits when the output fits on one screen.
An existing `LESS` value takes precedence.

Pipes and files receive the same 12 tab-separated fields (TSV), with UTC RFC3339 times.
Use `--header` to add TSV column names. Terminal tables always include headers.
Use `-L` to limit results:

```sh
gh notifs list -L 10
gh notifs list -L 10 --header > notifications.tsv
gh notifs list -q 'is:unread' | cut -f8,11
```

Run `go run ./examples/tsv` for an offline example with fictional data.
It shows a table in a terminal. Pipe it to `cat` to see the TSV format.
Use `GH_PAGER="less -+F" go run ./examples/tsv` to try the pager, then press `q` to exit.

**Check the targets before you run an action.** CLI actions change GitHub without confirmation:

```sh
gh notifs list -q 'repo:cli/cli is:unread' -L 10 > selected.tsv
cut -f1,8,11 selected.tsv
gh notifs read < selected.tsv
```

Actions accept notification thread IDs or TSV rows, not issue or pull request numbers. Omit `--header` when output feeds an action.

## Customize

The inbox creates `gh-notifs/config.yml` in your user configuration directory. Edit it to save settings and sidebar queries:

```yaml
auto_refresh: true
bell: true
scopes:
  - name: Inbox
    query: ""
  - name: My reviews
    query: "reason:review-requested"
```

Restart after edits. Use `--config PATH` for a different file.
Cached notifications and repository history can contain private information.

See `gh notifs --help` for all commands, filters, and options.

## License

[MIT](LICENSE).
