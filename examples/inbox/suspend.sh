#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
session="gh-notifs-suspend-$$"
binary="$PWD/bin/$session"
tui() { tui-test --session "$session" "$@"; }
trap 'tui close; rm -f "$binary"' EXIT

go build -o "$binary" ./examples/inbox
tui open --shell bash --cols 120 --rows 32 --cwd "$PWD" \
  --env TERM=xterm-256color --env COLORTERM=truecolor
tui submit 'saved=$(stty -g)'
tui expect exit-code 0
tui submit "$(printf '%q' "$binary")"
tui expect text "6 loaded"
tui key press x v
tui expect text "What changed"
tui key press /
tui type 'reason:mention'
tui key press Ctrl+z
tui expect mode alternate_screen --off
tui expect mode cursor_visible
tui expect mode focus_events --off
tui wait ready
tui submit 'test "$(stty -g)" = "$saved"'
tui expect exit-code 0
tui submit fg
tui expect mode alternate_screen
tui expect mode focus_events
tui expect text "reason:mention"
tui key press Escape
tui expect text "What changed"
tui expect text "✓"
tui key press e
tui expect text "Mark done 1 notification(s)?"
tui key press Ctrl+z
tui expect mode alternate_screen --off
tui wait ready
tui resize 100 30
tui submit fg
tui expect text "Mark done 1 notification(s)?"
tui key press n q
tui expect exit-code 0

# The go run parent must stop too, so the shell can regain the terminal.
tui submit 'go run ./examples/inbox'
tui expect text "6 loaded"
tui key press Ctrl+z
tui expect mode alternate_screen --off
tui wait ready
tui submit fg
tui expect text "6 loaded"
tui key press q
tui expect exit-code 0
printf 'Suspend/resume preserved the inbox and restored the shell.\n'
