#!/usr/bin/env sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

usage() {
	printf 'usage: %s [output-dir]\n' "$0" >&2
}

if [ "$#" -gt 1 ]; then
	usage
	exit 2
fi

host_os="$(uname -s)"
if [ "$host_os" != "Darwin" ]; then
	printf 'unsupported: macOS dev package build requires macOS (Darwin); current host: %s\n' "$host_os" >&2
	exit 1
fi

if ! command -v pkgbuild >/dev/null 2>&1; then
	printf 'unsupported: pkgbuild is required to create the macOS dev package\n' >&2
	exit 1
fi

out_dir=${1:-"$repo_root/dist/macos-dev-pkg"}
version=${VERSION:-0.0.0-dev}
arch=${GOARCH:-$(go env GOARCH)}
pkg_id=${PKG_IDENTIFIER:-dev.neul.pkg}
pkg_path="$out_dir/neul-$version-$arch-unsigned-dev.pkg"

case "$out_dir" in
	'')
		printf 'output directory must not be empty\n' >&2
		exit 2
		;;
esac

if [ -e "$out_dir" ] && [ ! -d "$out_dir" ]; then
	printf 'output path exists and is not a directory: %s\n' "$out_dir" >&2
	exit 1
fi

mkdir -p "$out_dir" || {
	printf 'failed to create output directory: %s\n' "$out_dir" >&2
	exit 1
}

tmp_root=${TMPDIR:-/tmp}
tmp_root=${tmp_root%/}
work_dir="$(mktemp -d "$tmp_root/neul-macos-dev-pkg.XXXXXX")"
cleanup() {
	rm -rf "$work_dir"
}
trap cleanup EXIT HUP INT TERM

payload="$work_dir/payload"
mkdir -p "$payload/usr/local/bin" "$payload/usr/local/libexec"

printf 'building neul for darwin/%s\n' "$arch"
(
	cd "$repo_root"
	GOOS=darwin GOARCH="$arch" go build -trimpath -o "$payload/usr/local/bin/neul" ./cmd/neul
	GOOS=darwin GOARCH="$arch" go build -trimpath -o "$payload/usr/local/libexec/neul-agent" ./cmd/neul-agent
)
chmod 0755 "$payload/usr/local/bin/neul" "$payload/usr/local/libexec/neul-agent"

rm -f "$pkg_path"
COPYFILE_DISABLE=1 pkgbuild \
	--root "$payload" \
	--identifier "$pkg_id" \
	--version "$version" \
	--install-location / \
	"$pkg_path"

if [ ! -s "$pkg_path" ]; then
	printf 'pkgbuild did not create a non-empty package: %s\n' "$pkg_path" >&2
	exit 1
fi

payload_files="$(pkgutil --payload-files "$pkg_path")"
printf '%s\n' "$payload_files" | grep -Eq '^(\./)?usr/local/bin/neul$' || {
	printf 'package payload missing usr/local/bin/neul\n' >&2
	exit 1
}
printf '%s\n' "$payload_files" | grep -Eq '^(\./)?usr/local/libexec/neul-agent$' || {
	printf 'package payload missing usr/local/libexec/neul-agent\n' >&2
	exit 1
}

signature_output="$(pkgutil --check-signature "$pkg_path" 2>&1 || true)"
if printf '%s\n' "$signature_output" | grep -Fq 'Status: signed'; then
	printf 'package is signed, but this script must create unsigned dev packages only: %s\n' "$pkg_path" >&2
	exit 1
fi

printf 'created unsigned dev package: %s\n' "$pkg_path"
printf 'payload: /usr/local/bin/neul, /usr/local/libexec/neul-agent\n'
printf 'signature: unsigned\n'
