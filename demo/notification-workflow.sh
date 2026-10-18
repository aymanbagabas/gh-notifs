#!/usr/bin/env bash
set -euo pipefail

# Fictional GitHub Universe demo only, not release documentation.
# Run with: bash demo/notification-workflow.sh
# These commands do not change notifications on GitHub.
gh notifs list -q 'repo:aymanbagabas/gh-notifs' -L 3 --header

# Save full, headerless rows in selected.tsv in the current directory (overwritten).
# Inspect reference and title (columns 8 and 11) before any separate action.
gh notifs list -q 'repo:aymanbagabas/gh-notifs is:pr is:unread' -L 3 | tee selected.tsv | cut -f8,11

# No rows means this account has no matching notifications.
# Run mutation commands separately, only after checking the saved full rows.
# Actions need IDs or full TSV rows, not the reference/title projection.
