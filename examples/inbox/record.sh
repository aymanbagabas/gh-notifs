#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
root="$PWD"
session="gh-notifs-demo-$$"
capture="$root/bin/$session.apng"
tui() { tui-test --session "$session" "$@"; }
trap 'tui close; rm -f "$capture"' EXIT

go build -o bin/inbox-demo ./examples/inbox
mkdir -p assets
tui run --cols 100 --rows 32 --cwd "$root" \
  --env TERM=xterm-256color --env COLORTERM=truecolor "$root/bin/inbox-demo"
tui expect text "6 loaded"
tui key press v
tui expect text "What changed"
tui record start "$capture" --fps 24 --zoom 1 --background '#0d1117'

# Pauses give viewers time to read; assertions wait for application state.
sleep 1.2
tui mouse drag 60 16 60 12
sleep 0.8
tui key press m
sleep 0.8
tui key press v
tui mouse move 45 8
tui mouse click 28 8
sleep 0.6
tui mouse click 28 8
tui mouse click 45 6
tui key press /
tui type 'reason:'
tui expect text 'review-requested'
sleep 0.6
tui type 'review-requested'
tui key press Enter
tui expect text '2 loaded'
sleep 1.2
tui key press / Ctrl+u Enter
tui expect text '6 loaded'
tui click text 'Unread'
tui expect text '3 loaded'
sleep 0.8
tui click text 'Inbox'
tui expect text '6 loaded'
tui mouse click 45 6
tui key press R B
tui expect text 'Auto:on'
sleep 0.8
tui key press R B v m
tui expect text 'What changed'
tui mouse drag 60 12 60 16
tui mouse move 0 0
sleep 1.2
tui record stop
tui key press q
tui wait exit
tui state | grep -qx 'exited: 0'

# Crop only tui-test's decoration, not terminal cells. Check the zoom-1 geometry.
size=$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height \
  -of csv=p=0:s=x "$capture")
if [[ "$size" != "2156x1544" ]]; then
  echo "Unexpected recording size: $size; update the frame crop before recording." >&2
  exit 1
fi
ffmpeg -v error -y -i "$capture" \
  -filter_complex 'crop=2000:1344:78:124,pad=iw+64:ih+64:32:32:color=0x0d1117,split[a][b];[a]palettegen[p];[b][p]paletteuse' \
  -loop 0 assets/demo.gif
