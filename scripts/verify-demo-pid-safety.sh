#!/usr/bin/env sh
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

PORT=${DEMO_PID_SAFETY_PORT:-54321}
DEMO_DIR=$(mktemp -d "${TMPDIR:-/tmp}/neul-demo-pid-safety.XXXXXX")
VICTIM_PID=
VICTIM_WRAPPER_PID=

cleanup() {
	if [ -n "$VICTIM_PID" ] && kill -0 "$VICTIM_PID" 2>/dev/null; then
		kill "$VICTIM_PID" 2>/dev/null || true
	fi
	if [ -n "$VICTIM_WRAPPER_PID" ]; then
		wait "$VICTIM_WRAPPER_PID" 2>/dev/null || true
	fi
	rm -rf "$DEMO_DIR"
}
trap cleanup EXIT INT TERM

(
	sleep 120 &
	printf '%s\n' "$!" > "$DEMO_DIR/victim.pid"
	wait "$!" || true
	printf 'done\n' > "$DEMO_DIR/victim.done"
) &
VICTIM_WRAPPER_PID=$!

for _ in 1 2 3 4 5 6 7 8 9 10; do
	if [ -s "$DEMO_DIR/victim.pid" ]; then
		break
	fi
	sleep 1
done
if [ ! -s "$DEMO_DIR/victim.pid" ]; then
	printf 'demo pid safety failed: stale PID victim did not start\n' >&2
	exit 1
fi

VICTIM_PID=$(cat "$DEMO_DIR/victim.pid")
printf '%s\n' "$VICTIM_PID" > "$DEMO_DIR/neul-server.pid"
printf '127.0.0.1:%s\n' "$PORT" > "$DEMO_DIR/neul-server.addr"

if ! HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh stop > "$DEMO_DIR/stop.out" 2>&1; then
	printf 'demo pid safety failed: stale PID stop should remove markers without failing\n' >&2
	cat "$DEMO_DIR/stop.out" >&2
	exit 1
fi

sleep 1
if [ -e "$DEMO_DIR/victim.done" ] || ! kill -0 "$VICTIM_PID" 2>/dev/null; then
	printf 'demo pid safety failed: stale PID stop terminated an unrelated process\n' >&2
	cat "$DEMO_DIR/stop.out" >&2
	exit 1
fi
if [ -e "$DEMO_DIR/neul-server.pid" ] || [ -e "$DEMO_DIR/neul-server.addr" ]; then
	printf 'demo pid safety failed: stale PID markers were not removed\n' >&2
	cat "$DEMO_DIR/stop.out" >&2
	exit 1
fi

kill "$VICTIM_PID" 2>/dev/null || true
wait "$VICTIM_WRAPPER_PID" 2>/dev/null || true
VICTIM_PID=
VICTIM_WRAPPER_PID=

printf 'demo pid safety passed\n'
