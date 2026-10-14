#!/bin/sh
# Deploy origin/main to the hub's own checkout and restart it. Run on the hub Mac from the dev repo.
# The hub runs from a separate detached worktree so edits in the dev checkout never affect it.
# Usage: deploy/update-hub.sh <tailscale-login>[,<login>...]
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
hub=${NEUL_HUB_DIR:-$HOME/.local/share/neul/hub}

git -C "$repo" fetch -q origin
if [ ! -d "$hub" ]; then
  mkdir -p "$(dirname "$hub")"
  git -C "$repo" worktree add -q --detach "$hub" origin/main
fi
git -C "$hub" checkout -q --detach origin/main
echo "hub checkout: $(git -C "$hub" log --oneline -1)"
"$hub/deploy/install-launchagent.sh" "$@"
