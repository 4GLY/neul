#!/usr/bin/env sh
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

PORT=${DEMO_SMOKE_PORT:-}
if [ -z "$PORT" ]; then
	for _ in 1 2 3 4 5; do
		candidate=$(python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)
		if ! curl -fsS "http://127.0.0.1:$candidate/api/healthz" >/dev/null 2>&1; then
			PORT=$candidate
			break
		fi
	done
fi
if [ -z "$PORT" ]; then
	printf 'demo smoke failed: could not find an unused local port\n' >&2
	exit 1
fi
DEMO_DIR=${DEMO_SMOKE_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/neul-demo-smoke.XXXXXX")}
COOKIES=$DEMO_DIR/cookies.txt
OUTPUT=$DEMO_DIR/start.out
PAIR_RESPONSE=$DEMO_DIR/pair-init.json
REMOTE_DIR=
CLEAN_OUTPUT=${DEMO_DIR}.clean.out

cleanup() {
	HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh stop >/dev/null 2>&1 || true
	rm -rf "$DEMO_DIR"
	if [ -n "$REMOTE_DIR" ]; then
		HOST=0.0.0.0 PORT="$PORT" DEMO_DIR="$REMOTE_DIR" sh scripts/demo.sh stop >/dev/null 2>&1 || true
		rm -rf "$REMOTE_DIR"
	fi
}
trap cleanup EXIT INT TERM

for bad_host in "" "*" "foo:bar" "127.0.0.1:8080" "::" "::1" "bad host" "-bad" "bad-"; do
	if HOST="$bad_host" PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh start > "$DEMO_DIR/bad-host.out" 2>&1; then
		printf 'demo smoke failed: bad HOST=%s was accepted\n' "$bad_host" >&2
		cat "$DEMO_DIR/bad-host.out" >&2
		exit 1
	fi
done
for bad_port in abc 0 70000; do
	if HOST=127.0.0.1 PORT="$bad_port" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh start > "$DEMO_DIR/bad-port.out" 2>&1; then
		printf 'demo smoke failed: bad PORT=%s was accepted\n' "$bad_port" >&2
		cat "$DEMO_DIR/bad-port.out" >&2
		exit 1
	fi
done

for _ in 1 2 3; do
	if HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh start > "$OUTPUT" 2>&1; then
		break
	fi
	if grep -q 'failed to start' "$OUTPUT"; then
		rm -rf "$DEMO_DIR"
		mkdir -p "$DEMO_DIR"
		continue
	fi
	cat "$OUTPUT" >&2
	exit 1
done
if ! grep -q 'neul demo running' "$OUTPUT"; then
	printf 'demo smoke failed: demo did not start\n' >&2
	cat "$OUTPUT" >&2
	exit 1
fi

if ! grep -q "NEUL_ADDR=127.0.0.1:$PORT" "$OUTPUT"; then
	printf 'demo smoke failed: NEUL_ADDR marker missing\n' >&2
	cat "$OUTPUT" >&2
	exit 1
fi
HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh status > "$DEMO_DIR/status.out"
if ! grep -q '^neul demo running:' "$DEMO_DIR/status.out"; then
	printf 'demo smoke failed: status did not report running demo\n' >&2
	cat "$DEMO_DIR/status.out" >&2
	exit 1
fi

token=$(awk '/^Setup token:/ { print $3; exit }' "$OUTPUT")
if [ -z "$token" ]; then
	printf 'demo smoke failed: setup token missing\n' >&2
	cat "$OUTPUT" >&2
	exit 1
fi

response_code=$(curl -sS -o "$DEMO_DIR/session.out" -w '%{http_code}' -c "$COOKIES" \
	-H 'Content-Type: application/json' \
	-d "{\"setupToken\":\"$token\"}" \
	"http://127.0.0.1:$PORT/api/session/local")
if [ "$response_code" != "204" ]; then
	printf 'demo smoke failed: session exchange status=%s\n' "$response_code" >&2
	cat "$DEMO_DIR/session.out" >&2
	exit 1
fi

HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh stop
HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh start > "$DEMO_DIR/restart.out" 2>&1
if ! grep -q '^Setup token: not reprinted for this existing DB$' "$DEMO_DIR/restart.out"; then
	printf 'demo smoke failed: same-DB restart message missing\n' >&2
	cat "$DEMO_DIR/restart.out" >&2
	exit 1
fi

pair_status=$(curl -sS -o "$PAIR_RESPONSE" -w '%{http_code}' -b "$COOKIES" -X POST \
	"http://127.0.0.1:$PORT/api/pair/init")
if [ "$pair_status" != "201" ]; then
	printf 'demo smoke failed: pair init status=%s\n' "$pair_status" >&2
	cat "$PAIR_RESPONSE" >&2
	exit 1
fi

pair_code=$(python3 - "$PAIR_RESPONSE" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    print(json.load(handle)["code"])
PY
)
mkdir -p "$DEMO_DIR/agent-config"
go run ./cmd/neul agent enroll \
	--server "http://127.0.0.1:$PORT" \
	--pair "$pair_code" \
	--config-dir "$DEMO_DIR/agent-config" \
	--connect-once > "$DEMO_DIR/enroll.out"
if ! grep -q '^Connected$' "$DEMO_DIR/enroll.out"; then
	printf 'demo smoke failed: agent enrollment did not connect\n' >&2
	cat "$DEMO_DIR/enroll.out" >&2
	exit 1
fi

if HOST=0.0.0.0 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh start > "$DEMO_DIR/mismatch.out" 2>&1; then
	printf 'demo smoke failed: bind mismatch was accepted\n' >&2
	cat "$DEMO_DIR/mismatch.out" >&2
	exit 1
fi
if ! grep -q 'already running with NEUL_ADDR' "$DEMO_DIR/mismatch.out"; then
	printf 'demo smoke failed: bind mismatch message missing\n' >&2
	cat "$DEMO_DIR/mismatch.out" >&2
	exit 1
fi

HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh stop
if curl -fsS "http://127.0.0.1:$PORT/api/healthz" >/dev/null 2>&1; then
	printf 'demo smoke failed: port %s still listening\n' "$PORT" >&2
	exit 1
fi

REMOTE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/neul-demo-remote.XXXXXX")
HOST=0.0.0.0 PORT="$PORT" DEMO_DIR="$REMOTE_DIR" sh scripts/demo.sh start > "$REMOTE_DIR/remote.out" 2>&1
if ! grep -q '^Remote:' "$REMOTE_DIR/remote.out"; then
	printf 'demo smoke failed: remote access hint missing\n' >&2
	cat "$REMOTE_DIR/remote.out" >&2
	exit 1
fi
HOST=0.0.0.0 PORT="$PORT" DEMO_DIR="$REMOTE_DIR" sh scripts/demo.sh clean >/dev/null

HOST=127.0.0.1 PORT="$PORT" DEMO_DIR="$DEMO_DIR" sh scripts/demo.sh clean > "$CLEAN_OUTPUT"
if [ -d "$DEMO_DIR" ]; then
	printf 'demo smoke failed: clean did not remove DEMO_DIR\n' >&2
	cat "$CLEAN_OUTPUT" >&2
	exit 1
fi

printf 'demo smoke passed\n'
