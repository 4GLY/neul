package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHomebrewAdapter_CheckContracts(t *testing.T) {
	tests := []struct {
		name, version, want, prefix string
		steps                       []brewStep
		contains, log               []string
	}{
		{name: "latest in sync when installed and not outdated", version: "latest", want: "in_sync", steps: steps(step("list --versions kubectl", 1, "kubectl 1.31.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "", "", 0)), log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "latest drifted when installed but outdated", version: "latest", want: "drifted", steps: steps(step("list --versions kubectl", 1, "kubectl 1.30.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "kubectl\n", "", 1)), log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "latest drifted when outdated exits zero with name output", version: "latest", want: "drifted", steps: steps(step("list --versions kubectl", 1, "kubectl 1.29.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "kubectl\n", "", 0)), log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "latest drifted when missing with empty output", version: "latest", want: "drifted", steps: steps(step("list --versions kubectl", 1, "", "", 1)), log: []string{"list --versions kubectl"}},
		{name: "latest drifted when missing with no such keg", version: "latest", want: "drifted", steps: steps(step("list --versions kubectl", 1, "", "Error: No such keg: /opt/homebrew/Cellar/kubectl\n", 1)), log: []string{"list --versions kubectl"}},
		{name: "list failure blocks with command and exit reason", version: "latest", want: "blocked", prefix: "brew_check_failed", steps: steps(step("list --versions kubectl", 1, "", "permission denied while reading Cellar\n", 42)), contains: []string{"brew list --versions kubectl", "exit 42"}, log: []string{"list --versions kubectl"}},
		{name: "outdated failure blocks with command and exit reason", version: "latest", want: "blocked", prefix: "brew_check_failed", steps: steps(step("list --versions kubectl", 1, "kubectl 1.31.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "", "network timeout while checking formula\n", 23)), contains: []string{"brew outdated --quiet --formula kubectl", "exit 23"}, log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "pinned in sync on exact installed version", version: "1.31.0", want: "in_sync", steps: steps(step("list --versions kubectl", 1, "kubectl 1.30.0 1.31.0\n", "", 0)), log: []string{"list --versions kubectl"}},
		{name: "pinned drifted on version mismatch", version: "1.31.0", want: "drifted", steps: steps(step("list --versions kubectl", 1, "kubectl 1.30.0\n", "", 0)), log: []string{"list --versions kubectl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			adapter, logPath := fakeHomebrewAdapter(t, tt.steps...)

			// When
			event := CheckPackage(context.Background(), adapter, brewResource("kubectl", tt.version))

			// Then
			assertEvent(t, event, tt.want, tt.prefix, tt.contains...)
			assertLog(t, logPath, tt.log...)
		})
	}
}

func TestHomebrewAdapter_ApplyContracts(t *testing.T) {
	tests := []struct {
		name, version, want, prefix string
		steps                       []brewStep
		log                         []string
		noMutation                  bool
	}{
		{name: "already in sync skips mutation", version: "latest", want: "in_sync", steps: steps(step("list --versions kubectl", 1, "kubectl 1.31.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "", "", 0)), log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl"}, noMutation: true},
		{name: "missing latest installs with formula flag then rechecks", version: "latest", want: "in_sync", steps: steps(step("list --versions kubectl", 1, "", "", 1), step("install --formula kubectl", 1, "", "", 0), step("list --versions kubectl", 2, "kubectl 1.31.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "", "", 0)), log: []string{"list --versions kubectl", "install --formula kubectl", "list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "outdated latest upgrades with formula flag then rechecks", version: "latest", want: "in_sync", steps: steps(step("list --versions kubectl", 1, "kubectl 1.30.0\n", "", 0), step("outdated --quiet --formula kubectl", 1, "kubectl\n", "", 1), step("upgrade --formula kubectl", 1, "", "", 0), step("list --versions kubectl", 2, "kubectl 1.31.0\n", "", 0), step("outdated --quiet --formula kubectl", 2, "", "", 0)), log: []string{"list --versions kubectl", "outdated --quiet --formula kubectl", "upgrade --formula kubectl", "list --versions kubectl", "outdated --quiet --formula kubectl"}},
		{name: "missing pinned version blocks without mutation", version: "1.31.0", want: "blocked", prefix: "brew_pinned_unsupported", steps: steps(step("list --versions kubectl", 1, "", "", 1)), log: []string{"list --versions kubectl"}, noMutation: true},
		{name: "drifted pinned version blocks without mutation", version: "1.31.0", want: "blocked", prefix: "brew_pinned_unsupported", steps: steps(step("list --versions kubectl", 1, "kubectl 1.30.0\n", "", 0)), log: []string{"list --versions kubectl"}, noMutation: true},
		{name: "post apply mismatch blocks", version: "latest", want: "blocked", prefix: "brew_apply_failed", steps: steps(step("list --versions kubectl", 1, "", "", 1), step("install --formula kubectl", 1, "", "", 0), step("list --versions kubectl", 2, "", "", 1)), log: []string{"list --versions kubectl", "install --formula kubectl", "list --versions kubectl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			adapter, logPath := fakeHomebrewAdapter(t, tt.steps...)

			// When
			event := ApplyPackage(context.Background(), adapter, brewResource("kubectl", tt.version))

			// Then
			assertEvent(t, event, tt.want, tt.prefix)
			assertLog(t, logPath, tt.log...)
			if tt.noMutation {
				assertNoInstallOrUpgrade(t, logPath)
			}
		})
	}
}

func TestHomebrewAdapter_RejectsUnsupportedNamesWithoutCallingBrew(t *testing.T) {
	for _, name := range []string{"homebrew/cask/docker", "kubectl/1.31", "bad name", "-kubectl", "kubectl@1.31"} {
		t.Run(name, func(t *testing.T) {
			// Given
			adapter, logPath := fakeHomebrewAdapter(t)

			// When
			event := CheckPackage(context.Background(), adapter, brewResource(name, "latest"))

			// Then
			assertEvent(t, event, "blocked", "brew_unsupported_name")
			assertLog(t, logPath)
		})
	}
}

func TestHomebrewAdapter_BlocksUnsupportedHostAndMissingBrew(t *testing.T) {
	t.Run("unsupported host", func(t *testing.T) {
		// Given
		adapter := NewHomebrewAdapter(WithHomebrewGOOS("linux"))

		// When
		event := CheckPackage(context.Background(), adapter, brewResource("kubectl", "latest"))

		// Then
		assertEvent(t, event, "blocked", "unsupported_host")
	})

	t.Run("darwin without brew on path", func(t *testing.T) {
		// Given
		t.Setenv("PATH", t.TempDir())
		adapter := NewHomebrewAdapter(WithHomebrewGOOS("darwin"))

		// When
		event := CheckPackage(context.Background(), adapter, brewResource("kubectl", "latest"))

		// Then
		assertEvent(t, event, "blocked", "brew_unavailable")
	})
}

func TestHomebrewAdapter_TruncatesFailureContext(t *testing.T) {
	// Given
	adapter, _ := fakeHomebrewAdapter(t, step("list --versions kubectl", 1, "", strings.Repeat("x", 320), 42))

	// When
	event := CheckPackage(context.Background(), adapter, brewResource("kubectl", "latest"))

	// Then
	assertEvent(t, event, "blocked", "brew_check_failed", "brew list --versions kubectl", "exit 42", strings.Repeat("x", 256)+"... [truncated]")
	if strings.Contains(event.Message, strings.Repeat("x", 257)) {
		t.Fatalf("message contains more than the 256-byte stderr cap: %q", event.Message)
	}
}

type brewStep struct {
	args, stdout, stderr string
	call, exitCode       int
}

func step(args string, call int, stdout string, stderr string, exitCode int) brewStep {
	return brewStep{args: args, call: call, stdout: stdout, stderr: stderr, exitCode: exitCode}
}

func steps(items ...brewStep) []brewStep {
	return items
}

func fakeHomebrewAdapter(t *testing.T, steps ...brewStep) (PackageAdapter, string) {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	must(t, os.Mkdir(stateDir, 0o755))
	logPath := filepath.Join(dir, "argv.log")
	must(t, os.WriteFile(filepath.Join(dir, "brew"), []byte(fakeBrewScript), 0o755))
	for _, item := range steps {
		base := filepath.Join(stateDir, item.args+"."+strconv.Itoa(item.call))
		if item.stdout != "" {
			must(t, os.WriteFile(base+".stdout", []byte(item.stdout), 0o644))
		}
		if item.stderr != "" {
			must(t, os.WriteFile(base+".stderr", []byte(item.stderr), 0o644))
		}
		must(t, os.WriteFile(base+".exit", []byte(strconv.Itoa(item.exitCode)), 0o644))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BREW_ARGV_LOG", logPath)
	t.Setenv("BREW_FAKE_STATE_DIR", stateDir)
	return NewHomebrewAdapter(WithHomebrewGOOS("darwin")), logPath
}

func brewResource(name string, desiredVersion string) DesiredResource {
	return DesiredResource{ID: "resource_" + strings.NewReplacer("/", "_", " ", "_", "@", "_").Replace(name), Kind: "package", Name: name, DesiredVersion: 1, Spec: map[string]interface{}{"sourceKind": "brew", "name": name, "desiredVersion": desiredVersion}}
}

func assertEvent(t *testing.T, event ResourceEvent, want string, prefix string, contains ...string) {
	t.Helper()
	if event.Status != want {
		t.Fatalf("status = %q, want %q; event = %+v", event.Status, want, event)
	}
	if prefix != "" && !strings.HasPrefix(event.Message, prefix) {
		t.Fatalf("message = %q, want prefix %q; event = %+v", event.Message, prefix, event)
	}
	for _, wantText := range contains {
		if !strings.Contains(event.Message, wantText) {
			t.Fatalf("message = %q, want substring %q", event.Message, wantText)
		}
	}
}

func assertLog(t *testing.T, logPath string, want ...string) {
	t.Helper()
	got := readLog(t, logPath)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("brew argv log = %#v, want %#v", got, want)
	}
}

func assertNoInstallOrUpgrade(t *testing.T, logPath string) {
	t.Helper()
	for _, line := range readLog(t, logPath) {
		if strings.HasPrefix(line, "install ") || strings.HasPrefix(line, "upgrade ") {
			t.Fatalf("brew argv log contains mutation command %q", line)
		}
	}
}

func readLog(t *testing.T, logPath string) []string {
	t.Helper()
	contents, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	must(t, err)
	text := strings.TrimSuffix(string(contents), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

const fakeBrewScript = `#!/bin/sh
set -eu
args="$*"
printf '%s\n' "$args" >> "$BREW_ARGV_LOG"
count_file="$BREW_FAKE_STATE_DIR/$args.count"
count=0
if [ -f "$count_file" ]; then
	count=$(cat "$count_file")
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
base="$BREW_FAKE_STATE_DIR/$args.$count"
if [ -f "$base.stdout" ]; then
	cat "$base.stdout"
fi
if [ -f "$base.stderr" ]; then
	cat "$base.stderr" >&2
fi
if [ -f "$base.exit" ]; then
	exit "$(cat "$base.exit")"
fi
printf 'unexpected invocation: %s\n' "$args" >&2
exit 98
`
