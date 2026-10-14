#!/bin/sh
# Install or update the neul hub as a LaunchAgent on the hub Mac and publish it on the tailnet.
# Usage: deploy/install-launchagent.sh <tailscale-login>[,<login>...]
set -eu

logins=${1:?"usage: $0 <tailscale-login>[,<login>...]"}
repo=$(cd "$(dirname "$0")/.." && pwd)
port=${NEUL_PORT:-7430}
label=dev.neul.hub
plist="$HOME/Library/LaunchAgents/$label.plist"
logs="$HOME/Library/Logs/neul"
ts=$(command -v tailscale || echo /Applications/Tailscale.app/Contents/MacOS/Tailscale)

(cd "$repo" && bun install --frozen-lockfile)
mkdir -p "$logs" "$(dirname "$plist")"

# zsh -c reads ~/.zshenv, so bun (mise shims) and ssh resolve the same way as in a terminal.
cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key>
  <array><string>/bin/zsh</string><string>-c</string><string>exec bun src/server/main.ts</string></array>
  <key>WorkingDirectory</key><string>$repo</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>NODE_ENV</key><string>production</string>
    <key>NEUL_PORT</key><string>$port</string>
    <key>NEUL_ALLOWED_LOGINS</key><string>$logins</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$logs/hub.log</string>
  <key>StandardErrorPath</key><string>$logs/hub.log</string>
</dict>
</plist>
EOF

domain="gui/$(id -u)"
launchctl bootout "$domain/$label" 2>/dev/null || true
# bootout returns before the old job is gone; bootstrapping too early fails with "5: Input/output error".
i=0
while launchctl print "$domain/$label" >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -le 50 ] || { echo "timed out waiting for $label to unload" >&2; exit 1; }
  sleep 0.2
done
launchctl bootstrap "$domain" "$plist"
"$ts" serve --bg --https="$port" "http://127.0.0.1:$port"
echo "neul hub: launchd $label, https on tailnet port $port, logs $logs/hub.log"
