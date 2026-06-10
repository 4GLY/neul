#!/usr/bin/env sh
set -eu

criterion=${1:-all}
failures=0

fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

require_text() {
	file=$1
	text=$2
	label=$3
	if ! grep -Fq "$text" "$file"; then
		fail "$label: missing '$text' in $file"
	fi
}

require_absent_between() {
	file=$1
	start=$2
	end=$3
	text=$4
	label=$5
	require_text "$file" "$start" "$label start marker"
	require_text "$file" "$end" "$label end marker"
	if awk -v start="$start" -v end="$end" -v text="$text" '
		index($0, start) { in_section = 1; seen_start = 1; next }
		in_section && index($0, end) { in_section = 0; seen_end = 1 }
		in_section && index($0, text) { found = 1 }
		END { exit (seen_start && seen_end && found) ? 0 : 1 }
	' "$file"; then
		fail "$label: '$text' found between '$start' and '$end' in $file"
	fi
}

packaged_primary_flow() {
	require_text "docs/mvp.md" "packaged neul client" "MVP packaged client primary path"
	require_text "docs/mvp.md" "macOS: Homebrew tap 또는 signed .pkg" "MVP macOS package format"
	require_text "docs/mvp.md" "Linux: Debian/Ubuntu .deb와 tarball" "MVP Linux package format"
	require_text "docs/mvp.md" "neul://enroll?server=" "MVP browser deep-link enroll handoff"
	require_text "docs/mvp.md" "local callback" "MVP local callback approval"
	require_text "web/src/copy.ts" "macOS: Homebrew tap 또는 signed .pkg" "web macOS install instruction"
	require_text "web/src/copy.ts" "Linux: Debian/Ubuntu .deb 또는 tarball" "web Linux install instruction"
	require_text "web/src/copy.ts" "neul enroll --server <origin>" "web target enroll command"
	require_text "web/src/onboardingWizard.test.tsx" "not.toContain(\"--pair\")" "web hides pair token in target command"
	require_text "internal/domain/contracts.md" "Primary packaged client onboarding flow" "contract packaged onboarding heading"
	require_text "internal/domain/contracts.md" "neul client install" "contract packaged client install"
	require_text "internal/domain/contracts.md" "neul://enroll?server=" "contract browser deep-link handoff"
}

fallback_debug_separation() {
	require_text "README.md" "### fallback/debug: checkout-local enrollment" "README fallback heading"
	require_text "docs/qa/agent-onboarding.md" "## Fallback/debug checkout-local enrollment" "QA fallback heading"
	require_absent_between "docs/mvp.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "MVP primary flow"
	require_absent_between "internal/domain/contracts.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "contract primary flow"
	require_absent_between "README.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "README primary flow"
	require_absent_between "docs/mvp.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair <token>" "MVP primary pair-token command"
	require_absent_between "internal/domain/contracts.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair <token>" "contract primary pair-token command"
	require_text "docs/qa/agent-onboarding.md" "## Fallback/debug checkout-local enrollment" "QA fallback section"
}

security_model_guardrails() {
	require_text "docs/mvp.md" "First-run states:" "MVP first-run states"
	require_text "docs/mvp.md" "not_logged_in" "MVP not logged in state"
	require_text "docs/mvp.md" "waiting_for_browser_approval" "MVP browser approval state"
	require_text "docs/mvp.md" "enrolled" "MVP enrolled state"
	require_text "docs/mvp.md" "offline" "MVP offline state"
	require_text "docs/mvp.md" "error" "MVP error state"
	require_text "docs/mvp.md" "Self-hosted owner approval model:" "MVP owner approval model"
	require_text "docs/mvp.md" "Device code is fallback-only" "MVP device code decision"
	require_text "docs/mvp.md" "allowed pair-token handoffs" "MVP token storage guardrail"
	require_text "internal/domain/contracts.md" "Self-hosted owner approval model" "contract owner approval model"
	require_text "internal/domain/contracts.md" "local callback binds to 127.0.0.1 only" "contract local callback bind guardrail"
	require_text "internal/domain/contracts.md" "Device code is fallback-only" "contract device code decision"
	require_text "internal/domain/contracts.md" "First-run states" "contract first-run states"
	require_text "internal/domain/contracts.md" "Pairing browser approval API" "contract browser approval API"
	require_text "internal/domain/contracts.md" "First-run state mapping" "contract state mapping"
	require_text "docs/qa/agent-onboarding.md" "Allowed pair-token handoffs" "QA allowed handoff guardrail"
	require_text "web/src/copy.ts" "allowedPairTokenHandoffs" "web security handoff copy"
	if grep -Fq "packaged-client command bridge" docs/mvp.md internal/domain/contracts.md web/src/copy.ts; then
		fail "packaged-client command bridge must not be an allowed pair-token handoff"
	fi
}

case "$criterion" in
	all)
		packaged_primary_flow
		fallback_debug_separation
		security_model_guardrails
		;;
	packaged-primary-flow)
		packaged_primary_flow
		;;
	fallback-debug-separation)
		fallback_debug_separation
		;;
	security-model-guardrails)
		security_model_guardrails
		;;
	*)
		printf 'usage: %s [all|packaged-primary-flow|fallback-debug-separation|security-model-guardrails]\n' "$0" >&2
		exit 2
		;;
esac

if [ "$failures" -ne 0 ]; then
	printf 'docs validation failed with %s failure(s)\n' "$failures"
	exit 1
fi

printf 'docs validation passed\n'
