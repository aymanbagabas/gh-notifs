#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
go build -o bin/inbox-demo ./examples/inbox
mkdir -p assets
if [[ -e bin/inbox-frames ]]; then
  echo "bin/inbox-frames already exists; move it before recording." >&2
  exit 1
fi
vhs examples/inbox/demo.tape

# Encode exported frames because VHS 0.12 cancels its encoder context.
ffmpeg -v error -y \
  -framerate 12 -i bin/inbox-frames/frame-text-%05d.png \
  -framerate 12 -i bin/inbox-frames/frame-cursor-%05d.png \
  -filter_complex '[0:v][1:v]overlay,pad=iw+40:ih+40:20:20:color=0x0d1117,split[a][b];[a]palettegen[p];[b][p]paletteuse' \
  -loop 0 assets/demo.gif
rm -r bin/inbox-frames
