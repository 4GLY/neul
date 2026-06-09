#!/usr/bin/env sh
set -eu

failures=0

require_file() {
	file=$1
	if [ ! -f "$file" ]; then
		printf 'FAIL missing file: %s\n' "$file"
		failures=$((failures + 1))
	fi
}

require_pattern() {
	file=$1
	pattern=$2
	label=$3
	if [ ! -f "$file" ]; then
		printf 'FAIL %s: %s is missing\n' "$label" "$file"
		failures=$((failures + 1))
		return
	fi
	if ! grep -Eq "$pattern" "$file"; then
		printf 'FAIL %s: pattern %s not found in %s\n' "$label" "$pattern" "$file"
		failures=$((failures + 1))
	fi
}

require_file "README.md"
require_file "Makefile"
require_file "scripts/demo.sh"
require_file "scripts/verify-demo-smoke.sh"
require_pattern "Makefile" '^[[:space:]]*demo:' "make demo target"
require_pattern "Makefile" '^[[:space:]]*demo-stop:' "make demo-stop target"
require_pattern "Makefile" 'sh scripts/demo\.sh start' "make demo calls demo helper"
require_pattern "Makefile" 'sh scripts/demo\.sh stop' "make demo-stop calls demo helper"
require_pattern "Makefile" '^[[:space:]]*verify-docs:' "make verify-docs target"
require_pattern "Makefile" '^[[:space:]]*verify-demo:' "make verify-demo target"
require_pattern "README.md" '^make demo$' "README canonical start command"
require_pattern "README.md" '^make demo HOST=0\.0\.0\.0 PORT=18090$' "README tailnet bind host command"
require_pattern "README.md" '재사용' "README documents cached demo artifacts"
require_pattern "README.md" 'PORT="\$\{PORT:-8080\}"' "README configurable token-exchange port"
require_pattern "README.md" '^neul setup token: <token>$' "README setup token log contract"
require_pattern "README.md" '/api/session/local' "README setup-token session endpoint"
require_pattern "README.md" '^go run \./cmd/neul agent enroll --server http://127\.0\.0\.1:<PORT> --pair pair_\.\.\. --connect-once$' "README UI first machine enrollment"
require_pattern "README.md" '^go run \./cmd/neul agent enroll --server http://127\.0\.0\.1:<PORT> --pair pair_\.\.\. --config-dir \.demo/agent-config --connect-once$' "README repeatable local enrollment"
require_pattern "README.md" '^### http, https, Tailscale 접근$' "README http/https section"
require_pattern "README.md" 'http://127\.0\.0\.1:<PORT>' "README local http access"
require_pattern "README.md" '프록시가 .*http://127\.0\.0\.1:<PORT>' "README https proxy guidance"
require_pattern "README.md" '(Tailscale|tailnet)' "README Tailscale hostname access"
require_pattern "README.md" '^make demo-stop$' "README cleanup command"
require_pattern "README.md" '^make demo-clean$' "README reset cleanup command"
require_pattern "README.md" '\.demo/home' "README demo-clean removes home state"
require_pattern "README.md" '\.demo/` 디렉터리' "README demo-clean removes demo directory"
require_pattern "README.md" '^make demo-status$' "README status command"
require_pattern "README.md" 'IPv6 host' "README rejects IPv6 hosts"
require_pattern "README.md" '^make verify-demo$' "README demo smoke verification command"

if [ "$failures" -ne 0 ]; then
	printf 'docs validation failed with %s failure(s)\n' "$failures"
	exit 1
fi

printf 'docs validation passed\n'
