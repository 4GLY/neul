#!/bin/sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
doc="$repo_root/docs/secrets-threat-model.md"
mvp_doc="$repo_root/docs/mvp.md"
tmp_root="${TMPDIR:-/tmp}"
tmp_root="${tmp_root%/}"

fail() {
  printf 'docs validation failed: %s\n' "$1" >&2
  exit 1
}

require_file() {
  path="$1"
  [ -f "$path" ] || fail "missing required file: ${path#$repo_root/}"
}

require_text() {
  path="$1"
  text="$2"
  grep -Fq "$text" "$path" || fail "${path#$repo_root/} missing required text: $text"
}

require_file "$doc"
require_file "$mvp_doc"

require_text "$doc" "server compromise"
require_text "$doc" "agent compromise"
require_text "$doc" "local disk exposure"
require_text "$doc" "browser key handling"
require_text "$doc" "age recipients"
require_text "$doc" "device public keys"
require_text "$doc" "owner-held master key"
require_text "$doc" "dashboard-safe secret metadata"
require_text "$doc" "Secret value handling remains disabled"
require_text "$doc" "MVP non-goals"
require_text "$doc" "Post-MVP architecture"
require_text "$doc" "Follow-up implementation issues"
require_text "$doc" "Owner:"
require_text "$doc" "Review-by:"
require_text "$doc" "Recovery flow"
require_text "$doc" "Audit and access log"
require_text "$doc" "Transport security"
require_text "$doc" "Revocation timeline"
require_text "$doc" "Backups"
require_text "$doc" "Crypto agility"
require_text "$doc" "Document governance"
require_text "$doc" "4GL-88"
require_text "$mvp_doc" "docs/secrets-threat-model.md"

tmp_links="$(mktemp "$tmp_root/neul-docs-links.XXXXXX")" || fail "mktemp failed"
tmp_runtime="$(mktemp "$tmp_root/neul-docs-runtime.XXXXXX")" || fail "mktemp failed"
trap 'rm -f "$tmp_links" "$tmp_runtime"' EXIT HUP INT TERM

find "$repo_root" \
  -path "$repo_root/.git" -prune -o \
  -path "$repo_root/.omo" -prune -o \
  -path "$repo_root/evidence" -prune -o \
  -path "$repo_root/web/node_modules" -prune -o \
  -path "$repo_root/web/dist" -prune -o \
  -name '*.md' -type f -print |
while IFS= read -r markdown_file; do
  awk '
    /^[[:space:]]*(```|~~~)/ { in_fence = !in_fence; next }
    in_fence { next }
    {
      line = $0
      if (match(line, /\[[^][]+\]\[[^][]+\]/)) {
        printf "%s\t%s\n", FILENAME, "__UNSUPPORTED_REFERENCE_LINK__"
      }
      while (match(line, /\[[^][]+\]\([^()]*\)/)) {
        token = substr(line, RSTART, RLENGTH)
        target = token
        sub(/^.*\]\(/, "", target)
        sub(/\)$/, "", target)
        printf "%s\t%s\n", FILENAME, target
        line = substr(line, RSTART + RLENGTH)
      }
    }
  ' "$markdown_file"
done |
while IFS='	' read -r markdown_file target; do
  case "$target" in
    __UNSUPPORTED_REFERENCE_LINK__)
      printf '%s uses unsupported reference-style markdown links\n' "${markdown_file#$repo_root/}" >> "$tmp_links"
      continue
      ;;
  esac
  case "$target" in
    ''|'#'*|http://*|https://*|mailto:*)
      continue
      ;;
  esac
  target_path="${target%%#*}"
  target_path="${target_path%%\?*}"
  case "$target_path" in
    /*)
      resolved="$repo_root$target_path"
      ;;
    *)
      resolved="$(dirname "$markdown_file")/$target_path"
      ;;
  esac
  [ -e "$resolved" ] || printf '%s -> %s\n' "${markdown_file#$repo_root/}" "$target" >> "$tmp_links"
done

if [ -s "$tmp_links" ]; then
  printf 'broken local markdown links:\n' >&2
  cat "$tmp_links" >&2
  exit 1
fi

if ! grep -Eq '^상태: Accepted([[:space:]]|$)' "$doc"; then
  find "$repo_root/cmd" "$repo_root/internal" "$repo_root/web/src" \
    -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) \
    ! -name '*_test.go' ! -name '*.test.ts' ! -name '*.test.tsx' -print |
  while IFS= read -r source_file; do
    grep -nE '/api/secrets|SecretAdapter|createSecret|decryptSecret|encryptSecret|secret value input|secret value editing|secret value form' "$source_file" || true
  done > "$tmp_runtime"
  if [ -s "$tmp_runtime" ]; then
    printf 'secret runtime surface found while threat model is not accepted:\n' >&2
    cat "$tmp_runtime" >&2
    exit 1
  fi
fi

printf 'docs validation passed\n'
