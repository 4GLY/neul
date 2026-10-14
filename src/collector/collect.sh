# neul collector (POSIX sh). Sent over stdin: `ssh <host> sh -s -- <path-pattern>...`.
# Prints `@@ <section>` headers followed by section lines. Read-only: never mutates the host.
# Path patterns are relative to $HOME and may contain shell globs (e.g. .ssh/config.d/*.conf).

export LC_ALL=C
export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_ANALYTICS=1

# Non-interactive shells may not carry the user's PATH; add the usual tool locations.
PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
export PATH

have() { command -v "$1" >/dev/null 2>&1; }
kv() { printf '%s=%s\n' "$1" "$2"; }

os=$(uname -s)

echo "@@ meta"
kv schema 1
kv hostname "$(hostname 2>/dev/null || uname -n)"
kv os "$os"
kv kernel "$(uname -r)"
kv arch "$(uname -m)"
kv user "$(id -un)"
kv collected_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
case "$os" in
  Darwin)
    kv os_name "macOS $(sw_vers -productVersion 2>/dev/null)"
    kv os_build "$(sw_vers -buildVersion 2>/dev/null)"
    kv model "$(sysctl -n hw.model 2>/dev/null)"
    kv cpu_count "$(sysctl -n hw.ncpu 2>/dev/null)"
    # "{ sec = 1760412345, usec = 0 } ..." — anchor on "{ sec" so "usec" is not matched.
    boot=$(sysctl -n kern.boottime 2>/dev/null | sed -n 's/^{ sec = \([0-9]*\),.*/\1/p')
    [ -n "$boot" ] && kv uptime_s "$(( $(date +%s) - boot ))"
    kv load "$(sysctl -n vm.loadavg 2>/dev/null | tr -d '{}' | awk '{print $1, $2, $3}')"
    kv mem_total_b "$(sysctl -n hw.memsize 2>/dev/null)"
    # Available ≈ free + inactive + speculative + purgeable pages (what Activity Monitor treats as reclaimable).
    vm_stat 2>/dev/null | awk -v ps="$(sysctl -n hw.pagesize 2>/dev/null)" '
      /page size of/ { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+$/) ps = $i }
      /Pages free|Pages inactive|Pages speculative|Pages purgeable/ { gsub(/\./, "", $NF); n += $NF }
      END { if (n > 0) printf "mem_available_b=%.0f\n", n * ps }'
    disk_path=/System/Volumes/Data
    [ -d "$disk_path" ] || disk_path=/
    if have pmset; then
      batt=$(pmset -g batt 2>/dev/null | sed -n 's/.*[[:space:]]\([0-9]*\)%; \([^;]*\);.*/\1 \2/p' | head -n 1)
      [ -n "$batt" ] && kv battery "$batt"
    fi
    ;;
  Linux)
    if [ -r /etc/os-release ]; then
      kv os_name "$(. /etc/os-release && printf '%s' "${PRETTY_NAME:-$NAME}")"
    fi
    kv model "$(cat /sys/devices/virtual/dmi/id/product_name 2>/dev/null)"
    kv cpu_count "$(getconf _NPROCESSORS_ONLN 2>/dev/null)"
    kv uptime_s "$(cut -d. -f1 /proc/uptime 2>/dev/null)"
    kv load "$(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null)"
    awk '/^MemTotal:/ { printf "mem_total_b=%.0f\n", $2 * 1024 }
         /^MemAvailable:/ { printf "mem_available_b=%.0f\n", $2 * 1024 }' /proc/meminfo 2>/dev/null
    disk_path=/
    for b in /sys/class/power_supply/BAT*; do
      [ -r "$b/capacity" ] || continue
      kv battery "$(cat "$b/capacity") $(tr 'A-Z' 'a-z' < "$b/status" 2>/dev/null)"
      break
    done
    ;;
  *) disk_path=/ ;;
esac
df -kP "$disk_path" 2>/dev/null | awk -v p="$disk_path" 'NR == 2 {
  printf "disk_path=%s\ndisk_total_b=%.0f\ndisk_used_b=%.0f\ndisk_available_b=%.0f\n", p, $2 * 1024, $3 * 1024, $4 * 1024 }'

if have brew; then
  echo "@@ brew_formulae"
  brew list --formula --versions 2>/dev/null
  echo "@@ brew_leaves"
  brew leaves --installed-on-request 2>/dev/null
  echo "@@ brew_casks"
  # Read the Caskroom directly: `brew list --cask --versions` aborts on the first broken cask.
  caskroom=$(brew --caskroom 2>/dev/null)
  for d in "$caskroom"/*/; do
    [ -d "$d" ] || continue
    printf '%s' "$(basename "$d")"
    for v in "$d"*; do [ -d "$v" ] && printf ' %s' "$(basename "$v")"; done
    printf '\n'
  done
fi

if have pacman; then
  echo "@@ pacman_explicit"
  pacman -Qe 2>/dev/null
  echo "@@ pacman_count"
  pacman -Q 2>/dev/null | wc -l | tr -d ' '
fi

if have mise; then
  echo "@@ mise_json"
  mise ls --json 2>/dev/null
fi

if [ "$#" -gt 0 ]; then
  echo "@@ files"
  if have sha256sum; then sha() { sha256sum "$1" | cut -d' ' -f1; }
  else sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
  fi
  cd "$HOME" || exit 0
  # One tab-separated line per path: kind, sha256, size, link target, path.
  IFS='' # glob the pattern but never split it, so paths with spaces survive
  for pattern in "$@"; do
    matched=0
    for f in $pattern; do
      [ -e "$f" ] || [ -L "$f" ] || continue
      matched=1
      target=""
      kind=file
      if [ -L "$f" ]; then kind=symlink; target=$(readlink "$f"); fi
      if [ -f "$f" ]; then
        printf '%s\t%s\t%s\t%s\t%s\n' "$kind" "$(sha "$f")" "$(wc -c < "$f" | tr -d ' ')" "$target" "$f"
      elif [ -d "$f" ]; then
        printf 'dir\t\t\t%s\t%s\n' "$target" "$f"
      else
        printf 'broken\t\t\t%s\t%s\n' "$target" "$f"
      fi
    done
    [ "$matched" = 1 ] || printf 'missing\t\t\t\t%s\n' "$pattern"
  done
fi

echo "@@ end"
