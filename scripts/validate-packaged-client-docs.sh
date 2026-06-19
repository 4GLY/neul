#!/usr/bin/env sh
set -eu

criterion=${1:-all}
failures=0

fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

require_text() {
	rt_file=$1
	rt_text=$2
	rt_label=$3
	if ! grep -Fq -- "$rt_text" "$rt_file"; then
		fail "$rt_label: missing '$rt_text' in $rt_file"
	fi
}

require_absent() {
	ra_file=$1
	ra_text=$2
	ra_label=$3
	if grep -Fq -- "$ra_text" "$ra_file"; then
		fail "$ra_label: stale '$ra_text' found in $ra_file"
	fi
}

require_executable() {
	re_file=$1
	re_label=$2
	if [ ! -x "$re_file" ]; then
		fail "$re_label: missing executable $re_file"
	fi
}

require_absent_between() {
	rab_file=$1
	rab_start=$2
	rab_end=$3
	rab_text=$4
	rab_label=$5
	require_text "$rab_file" "$rab_start" "$rab_label start marker"
	require_text "$rab_file" "$rab_end" "$rab_label end marker"
	if awk -v start="$rab_start" -v end="$rab_end" '
		index($0, start) { in_section = 1; seen_start = 1; next }
		in_section && index($0, end) { in_section = 0; seen_end = 1 }
		in_section { print }
		END { exit (seen_start && seen_end) ? 0 : 1 }
	' "$rab_file" | grep -Fq -- "$rab_text"; then
		fail "$rab_label: '$rab_text' found between '$rab_start' and '$rab_end' in $rab_file"
	fi
}

packaged_primary_flow() {
	require_executable "scripts/build-macos-dev-pkg.sh" "macOS dev package builder"
	require_text "scripts/build-macos-dev-pkg.sh" "pkgbuild" "macOS dev package builder uses pkgbuild"
	require_text "scripts/build-macos-dev-pkg.sh" "GOOS=darwin" "macOS dev package builder targets darwin"
	require_text "scripts/build-macos-dev-pkg.sh" "unsupported: macOS dev package build requires macOS" "macOS dev package builder non-macOS guard"
	require_text "scripts/build-macos-dev-pkg.sh" "/usr/local/bin/neul" "macOS dev package builder neul path"
	require_text "scripts/build-macos-dev-pkg.sh" "/usr/local/libexec/neul-agent" "macOS dev package builder neul-agent path"
	require_text "scripts/build-macos-dev-pkg.sh" "signature: unsigned" "macOS dev package builder unsigned evidence"
	require_text "docs/mvp.md" "packaged neul client" "MVP packaged client primary path"
	require_text "docs/mvp.md" "scripts/build-macos-dev-pkg.sh" "MVP macOS dev package builder"
	require_text "docs/mvp.md" "macOS local QA: unsigned dev .pkg" "MVP macOS dev package format"
	require_text "docs/mvp.md" "Developer ID Application and" "MVP macOS production signing certificate"
	require_text "docs/mvp.md" "Developer ID Installer certificates, notarization, and stapling" "MVP macOS production notarization limits"
	require_text "docs/mvp.md" "Linux: Debian/Ubuntu .deb와 tarball" "MVP Linux package format"
	require_text "docs/mvp.md" "/usr/local/bin/neul" "MVP package neul path"
	require_text "docs/mvp.md" "/usr/local/libexec/neul-agent" "MVP package neul-agent path"
	require_text "docs/mvp.md" "neul login --server <origin>" "MVP packaged login command"
	require_text "docs/mvp.md" "browser approval polling" "MVP browser approval polling"
	require_text "web/src/copy.ts" "macOS local QA: unsigned dev .pkg" "web macOS dev install instruction"
	require_text "web/src/copy.ts" "Production macOS: Developer ID Application/Installer, notarization, stapling" "web macOS production signing instruction"
	require_text "web/src/copy.ts" "Linux: Debian/Ubuntu .deb 또는 tarball" "web Linux install instruction"
	require_text "web/src/copy.ts" "neul login --server <origin>" "web target login command"
	require_text "web/src/OnboardingWizard.tsx" "neul login --server" "web wizard rendered login command"
	require_text "web/src/onboardingWizard.test.tsx" "neul login --server http://localhost:3000" "web wizard login command assertion"
	require_text "web/e2e/mvp-flow.ts" "neul login --server" "E2E primary login command assertion"
	require_text "web/src/copy.ts" "fallback/debug 명령으로 등록하세요" "web fallback/debug instruction"
	require_text "web/src/onboardingWizard.test.tsx" "not.toContain(\"--pair\")" "web hides pair code in target command"
	require_text "internal/domain/contracts.md" "Primary packaged client onboarding flow" "contract packaged onboarding heading"
	require_text "internal/domain/contracts.md" "scripts/build-macos-dev-pkg.sh" "contract macOS dev package builder"
	require_text "internal/domain/contracts.md" "neul client install" "contract packaged client install"
	require_text "internal/domain/contracts.md" "/usr/local/bin/neul" "contract package neul path"
	require_text "internal/domain/contracts.md" "/usr/local/libexec/neul-agent" "contract package neul-agent path"
	require_text "internal/domain/contracts.md" "Developer ID Application and Developer ID Installer certificates" "contract production signing certificates"
	require_text "internal/domain/contracts.md" "notarization, and stapling" "contract production notarization limits"
	require_text "internal/domain/contracts.md" "POST /api/pair/approval/claim" "contract approval claim polling"
}

fallback_debug_separation() {
	require_text "README.md" "### fallback/debug: checkout-local enrollment" "README fallback heading"
	require_text "docs/qa/agent-onboarding.md" "## Fallback/debug checkout-local enrollment" "QA fallback heading"
	require_text "web/src/copy.ts" "go run ./cmd/neul agent enroll --server <origin> --pair <pair-code> --connect-once" "web executable fallback command"
	require_text "web/e2e/mvp-flow.ts" "page.locator(\"code\").nth(1)" "E2E uses visible fallback command"
	require_absent_between "docs/mvp.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "MVP primary flow"
	require_absent_between "internal/domain/contracts.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "contract primary flow"
	require_absent_between "README.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "go run ./cmd/neul" "README primary flow"
	require_absent_between "docs/mvp.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair <pair-code>" "MVP primary pair-code command"
	require_absent_between "README.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair <pair-code>" "README primary pair-code command"
	require_absent_between "internal/domain/contracts.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair <pair-code>" "contract primary pair-code command"
	require_absent_between "docs/mvp.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair" "MVP primary pair flag"
	require_absent_between "README.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair" "README primary pair flag"
	require_absent_between "internal/domain/contracts.md" "<!-- packaged-primary:start -->" "<!-- packaged-primary:end -->" "--pair" "contract primary pair flag"
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
	require_text "docs/mvp.md" "browser-safe approval handoffs" "MVP browser-safe approval handoffs"
	require_text "internal/domain/contracts.md" "Self-hosted owner approval model" "contract owner approval model"
	require_text "internal/domain/contracts.md" "approval claim is machine-client polling" "contract approval claim polling guardrail"
	require_text "internal/domain/contracts.md" '`GET /api/pair/poll` is the source of truth only for fallback/debug pair-code expiry' "contract fallback pair poll expiry"
	require_text "internal/domain/contracts.md" 'Approval expiry uses `GET /api/pair/approval/status`' "contract approval expiry source"
	require_text "docs/mvp.md" 'durable `neul up` agent-start attempt' "MVP neul up heartbeat timeout anchor"
	require_text "internal/domain/contracts.md" 'durable `neul up` agent-start attempt' "contract neul up heartbeat timeout anchor"
	require_absent "docs/mvp.md" "claim 이후 120초" "MVP post-claim heartbeat timeout"
	require_absent "internal/domain/contracts.md" "claimed machine does not heartbeat within 120 seconds" "contract post-claim heartbeat timeout"
	require_absent "internal/domain/contracts.md" "Pair poll is the source of truth for onboarding expiry" "contract broad pair poll expiry"
	require_text "internal/domain/contracts.md" "First-run states" "contract first-run states"
	require_text "internal/domain/contracts.md" "Pairing browser approval API" "contract browser approval API"
	require_text "internal/domain/contracts.md" "First-run state mapping" "contract state mapping"
	require_text "docs/qa/agent-onboarding.md" "Browser-safe approval handoffs" "QA browser-safe handoff guardrail"
	require_text "web/src/copy.ts" "browserSafeApprovalHandoffs" "web approval handoff copy"
	require_text "docs/qa/agent-onboarding.md" "/api/pair/approval/status" "QA approval expiry source"
	require_absent "docs/qa/agent-onboarding.md" 'then route `/api/pair/poll`' "QA fallback poll expiry source"
	require_absent "web/src/OnboardingWizard.tsx" "neul enroll --server" "web wizard stale enroll command"
	require_absent "web/src/onboardingWizard.test.tsx" "neul enroll --server" "web wizard stale enroll assertion"
	require_absent "web/e2e/mvp-flow.ts" "neul enroll --server" "E2E stale enroll assertion"
	if grep -Fq "packaged-client command bridge" docs/mvp.md internal/domain/contracts.md README.md docs/qa/agent-onboarding.md web/src/copy.ts; then
		fail "packaged-client command bridge must not be a browser-safe approval handoff"
	fi
	if grep -Fq "allowedPairTokenHandoffs" docs/mvp.md internal/domain/contracts.md README.md docs/qa/agent-onboarding.md web/src/copy.ts; then
		fail "allowedPairTokenHandoffs must not remain in docs or web copy"
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
