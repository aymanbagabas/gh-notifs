#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
root="$PWD"
session="gh-notifs-demo-$$"
tui() { tui-test --session "$session" "$@"; }
trap 'tui close' EXIT

go build -o bin/inbox-demo ./examples/inbox
mkdir -p assets
tui run --cols 120 --rows 36 --cwd "$root" \
  --env TERM=xterm-256color --env COLORTERM=truecolor "$root/bin/inbox-demo"
tui expect text "Keep uploads responsive"
tui wait idle
tui record start "$root/assets/demo.gif" --fps 12 --idle-time-limit 3 --background '#0d1117'

# Pauses give viewers time to read; assertions wait for application state.
sleep 2
tui key press j
sleep 1
tui key press x
sleep 1
tui key press x k v
tui expect text "What changed"
sleep 3
tui key press m
sleep 2
tui key press v /
tui type 'reason:'
tui expect text 'review-requested'
sleep 2
tui type 'review-requested'
tui key press Enter
tui expect text '2 loaded'
sleep 3
tui key press / Ctrl+u Enter
tui expect text '6 loaded'
tui click text 'Unread'
tui expect text '3 loaded'
sleep 2
tui click text 'Inbox'
tui expect text '6 loaded'
sleep 2
tui record stop
tui key press q
tui wait exit
tui state | grep -qx 'exited: 0'
