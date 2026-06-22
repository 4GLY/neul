#!/usr/bin/env sh
set -eu

command=${1:-start}
script_dir=$(dirname "$0")
repo_root=$(cd "$script_dir/.." && pwd)
cd "$repo_root"

HOST=${HOST-127.0.0.1}
PORT=${PORT-8080}
DEMO_DIR=${DEMO_DIR:-.demo}
DEMO_BIN=${DEMO_BIN:-$DEMO_DIR/neul-server}
DEMO_DB=${DEMO_DB:-$DEMO_DIR/neul.sqlite}
DEMO_LOG=${DEMO_LOG:-$DEMO_DIR/neul-server.log}
DEMO_PID=${DEMO_PID:-$DEMO_DIR/neul-server.pid}
DEMO_ADDR_FILE=${DEMO_ADDR_FILE:-$DEMO_DIR/neul-server.addr}
DEMO_HOME=${DEMO_HOME:-$DEMO_DIR/home}
DEMO_STATIC_DIR=${DEMO_STATIC_DIR:-web/dist}
DEMO_ADDR=$HOST:$PORT
SETUP_TOKEN_PREFIX=${SETUP_TOKEN_PREFIX:-$(awk -F '"' '/setupTokenOutputPrefix =/ { print $2; exit }' internal/server/auth.go)}
if [ -z "$SETUP_TOKEN_PREFIX" ]; then
	printf 'Could not read setup token output prefix from internal/server/auth.go\n' >&2
	exit 2
fi

abs_path() {
	case "$1" in
		/*) printf '%s\n' "$1" ;;
		*) printf '%s/%s\n' "$repo_root" "$1" ;;
	esac
}

DEMO_BIN_ABS=$(abs_path "$DEMO_BIN")
DEMO_BIN_NAME=$(basename "$DEMO_BIN_ABS")
DEMO_DB_ABS=$(abs_path "$DEMO_DB")
DEMO_HOME_ABS=$(abs_path "$DEMO_HOME")
DEMO_STATIC_DIR_ABS=$(abs_path "$DEMO_STATIC_DIR")

ACCESS_HOST=${ACCESS_HOST:-$HOST}
if [ "$HOST" = "0.0.0.0" ]; then
	ACCESS_HOST=127.0.0.1
fi
DEMO_URL=${DEMO_URL:-http://$ACCESS_HOST:$PORT}

validate_start_config() {
	case "$HOST" in
		'')
			printf 'HOST must not be empty. Use HOST=127.0.0.1 or HOST=0.0.0.0.\n' >&2
			exit 2
			;;
		*:*)
			printf 'HOST must not include a colon. Pass the port with PORT=; IPv6 hosts are not supported by this demo helper.\n' >&2
			exit 2
			;;
		'*')
			printf 'Wildcard HOST=* is not supported by this demo helper. Use HOST=0.0.0.0 for tailnet demos.\n' >&2
			exit 2
			;;
		*/*|*" "*)
			printf 'HOST must be a hostname or IPv4 literal without spaces or slashes.\n' >&2
			exit 2
			;;
		*[!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-]*)
			printf 'HOST must be a hostname or IPv4 literal containing only letters, digits, dots, and hyphens.\n' >&2
			exit 2
			;;
		.*|*.|-*|*-|*..*|*.-*|*-.*)
			printf 'HOST must not have empty labels or leading/trailing dots or hyphens.\n' >&2
			exit 2
			;;
	esac

	case "$PORT" in
		''|*[!0123456789]*)
			printf 'PORT must be a number from 1 to 65535.\n' >&2
			exit 2
			;;
	esac
	if [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
		printf 'PORT must be a number from 1 to 65535.\n' >&2
		exit 2
	fi
}

read_demo_pid() {
	[ -f "$DEMO_PID" ] || return 1
	pid=$(cat "$DEMO_PID")
	case "$pid" in
		''|*[!0123456789]*)
			return 1
			;;
	esac
	printf '%s\n' "$pid"
}

demo_pid_command() {
	ps -p "$1" -o command= 2>/dev/null || true
}

demo_pid_name() {
	ps -p "$1" -o comm= 2>/dev/null || true
}

pid_owned_by_demo() {
	pid=$1
	kill -0 "$pid" 2>/dev/null || return 1
	command_name=$(demo_pid_name "$pid")
	command_line=$(demo_pid_command "$pid")
	[ -n "$command_name" ] && [ -n "$command_line" ] || return 1
	case "$command_name" in
		"$DEMO_BIN_NAME"|*"/$DEMO_BIN_NAME")
			;;
		*)
			return 1
			;;
	esac
	case "$command_line" in
		*"$DEMO_BIN_ABS"*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

clear_demo_markers() {
	rm -f "$DEMO_PID" "$DEMO_ADDR_FILE"
}

pid_running() {
	pid=$(read_demo_pid) || return 1
	pid_owned_by_demo "$pid"
}

extract_setup_token() {
	awk -v prefix="$SETUP_TOKEN_PREFIX" 'index($0, prefix) == 1 { print substr($0, length(prefix) + 1); exit }' "$1"
}

stop_demo() {
	if [ ! -f "$DEMO_PID" ]; then
		printf 'No demo pid file at %s\n' "$DEMO_PID"
		return 0
	fi

	pid=$(read_demo_pid) || {
		printf 'Demo pid marker at %s is stale or invalid\n' "$DEMO_PID"
		clear_demo_markers
		return 0
	}
	if ! kill -0 "$pid" 2>/dev/null; then
		printf 'Demo pid %s is not running\n' "$pid"
		clear_demo_markers
		return 0
	fi
	if ! pid_owned_by_demo "$pid"; then
		command_line=$(demo_pid_command "$pid")
		if [ -n "$command_line" ]; then
			printf 'Demo pid %s is not the expected neul demo process; removing stale marker\n' "$pid"
		else
			printf 'Demo pid %s could not be verified; removing stale marker\n' "$pid"
		fi
		clear_demo_markers
		return 0
	fi

	if kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null || true
		for _ in 1 2 3 4 5 6 7 8 9 10; do
			if ! kill -0 "$pid" 2>/dev/null; then
				break
			fi
			sleep 1
		done
		if kill -0 "$pid" 2>/dev/null; then
			kill -9 "$pid" 2>/dev/null || true
		fi
		if kill -0 "$pid" 2>/dev/null; then
			printf 'Could not stop neul demo pid=%s; leaving marker files in place.\n' "$pid" >&2
			return 1
		fi
		printf 'Stopped neul demo pid=%s\n' "$pid"
	else
		printf 'Demo pid %s is not running\n' "$pid"
	fi
	clear_demo_markers
}

wait_for_health() {
	for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
		if curl -fsS "$DEMO_URL/api/healthz" >/dev/null 2>&1; then
			return 0
		fi
		if ! pid_running; then
			printf 'neul demo failed to start. See %s\n' "$DEMO_LOG" >&2
			stop_demo
			return 1
		fi
		sleep 1
	done
	printf 'neul demo did not become healthy at %s. See %s\n' "$DEMO_URL" "$DEMO_LOG" >&2
	stop_demo
	return 1
}

start_demo() {
	validate_start_config
	trap 'stop_demo; exit 130' INT TERM
	mkdir -p "$DEMO_DIR" "$DEMO_HOME"
	fresh_db=0
	if [ ! -f "$DEMO_DB" ]; then
		fresh_db=1
	fi

	if pid_running; then
		running_addr=unknown
		if [ -f "$DEMO_ADDR_FILE" ]; then
			running_addr=$(cat "$DEMO_ADDR_FILE")
		fi
		if [ "$running_addr" != "$DEMO_ADDR" ]; then
			printf 'neul demo already running with NEUL_ADDR=%s; requested NEUL_ADDR=%s. Run make demo-stop first.\n' "$running_addr" "$DEMO_ADDR" >&2
			exit 1
		fi
		printf 'neul demo already running: pid=%s\n' "$(cat "$DEMO_PID")"
		printf 'Open: %s\n' "$DEMO_URL"
		printf 'Stop: make demo-stop\n'
		return 0
	fi
	clear_demo_markers

	if [ ! -d web/node_modules ]; then
		pnpm --dir web install --frozen-lockfile
	else
		printf 'web dependencies already installed\n'
	fi
	if [ ! -f web/dist/index.html ]; then
		pnpm --dir web build
	else
		printf 'web build already present\n'
	fi
	if [ ! -x "$DEMO_BIN" ]; then
		go build -o "$DEMO_BIN" ./cmd/neul-server
	else
		printf 'server binary already built: %s\n' "$DEMO_BIN"
	fi
	NEUL_ADDR="$DEMO_ADDR" \
	NEUL_DB="$DEMO_DB_ABS" \
	NEUL_HOME_DIR="$DEMO_HOME_ABS" \
	NEUL_PUBLIC_ORIGIN="${NEUL_PUBLIC_ORIGIN:-$DEMO_URL}" \
	NEUL_STATIC_DIR="$DEMO_STATIC_DIR_ABS" \
		"$DEMO_BIN_ABS" > "$DEMO_LOG" 2>&1 &
	printf '%s\n' "$!" > "$DEMO_PID"
	printf '%s\n' "$DEMO_ADDR" > "$DEMO_ADDR_FILE"

	wait_for_health
	trap - INT TERM

	printf 'neul demo running\n'
	printf 'NEUL_ADDR=%s\n' "$DEMO_ADDR"
	printf 'Open: %s\n' "$DEMO_URL"
	if [ "$HOST" = "0.0.0.0" ]; then
		printf 'Remote: use http://<lan-ip>:%s, http://<tailscale-hostname>:%s, or http://<tailnet-ip>:%s from another trusted device\n' "$PORT" "$PORT" "$PORT"
	fi
	setup_token=$(extract_setup_token "$DEMO_LOG")
	if [ -n "$setup_token" ]; then
		printf 'Setup token: %s\n' "$setup_token"
	elif [ "$fresh_db" = "1" ]; then
		printf 'Setup token was not printed for a fresh demo DB. See %s\n' "$DEMO_LOG" >&2
		stop_demo
		return 1
	else
		printf 'Setup token: not reprinted for this existing DB\n'
		printf 'Reuse the previous setup token if it has not been exchanged, or run make demo-clean demo to reset local demo state.\n'
	fi
	printf 'Log: %s\n' "$DEMO_LOG"
	printf 'Stop: make demo-stop\n'
}

case "$command" in
	start)
		start_demo
		;;
	stop)
		stop_demo
		;;
	clean)
		stop_demo
		rm -f "$DEMO_BIN" "$DEMO_DB" "$DEMO_DB-journal" "$DEMO_LOG" "$DEMO_PID" "$DEMO_ADDR_FILE"
		rm -rf "$DEMO_HOME" "$DEMO_DIR"
		printf 'Removed %s\n' "$DEMO_DIR"
		;;
	status)
		if pid_running; then
			printf 'neul demo running: pid=%s\n' "$(cat "$DEMO_PID")"
			printf 'Open: %s\n' "$DEMO_URL"
		else
			printf 'neul demo is not running\n'
		fi
		;;
	*)
		printf 'unknown demo command: %s\n' "$command" >&2
		exit 2
		;;
esac
