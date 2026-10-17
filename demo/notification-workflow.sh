#!/usr/bin/env bash
set -euo pipefail

# Fictional presentation branch. This script only reads notifications.
gh notifs list -q 'repo:aymanbagabas/gh-notifs' -L 3 --header
gh notifs list -q 'repo:aymanbagabas/gh-notifs is:pr is:unread' -L 3 | cut -f8,11
