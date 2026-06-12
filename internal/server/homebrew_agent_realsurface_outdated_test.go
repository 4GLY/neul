package server

import (
	"context"
	"strings"
	"testing"
)

func TestFakeBrewAgent_RealSurfaceOutdatedLatestUpgradesAndUpdatesDashboard(t *testing.T) {
	// Given
	fixture := newHomebrewRealSurfaceFixture(t, "machine_fake_brew_outdated", "mtn_fake_brew_outdated")
	resourceID := createPackageResourceWithVersion(t, fixture.router, fixture.cookie, "kubectl", "latest")
	seedQueuedAgentCommand(t, fixture.db, "command_fake_brew_outdated", fixture.machineID, "reconcile_now", `{}`, fixture.now)
	fakeBrew := installRealSurfaceFakeBrewWithScript(t, realSurfaceOutdatedFakeBrewScript)
	client := newRealSurfaceAgent(t, fixture.server.URL, fixture.machineID, fixture.machineToken, "darwin")

	// When
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Then
	log := readFakeBrewLog(t, fakeBrew.logPath)
	assertLogContainsInOrder(t, log,
		"list --versions kubectl",
		"outdated --quiet --formula kubectl",
		"upgrade --formula kubectl",
		"list --versions kubectl",
		"outdated --quiet --formula kubectl",
	)
	assertCommandStatus(t, fixture.db, "command_fake_brew_outdated", "finished")
	assertLatestResourceEvent(t, fixture.db, resourceID, "in_sync", "brew apply")
	machine := dashboardMachineByID(t, fixture.router, fixture.cookie, fixture.machineID)
	if machine.Status != "healthy" || machine.ResourceCount != 1 || machine.AppliedCount != 1 || machine.BlockedCount != 0 {
		t.Fatalf("dashboard machine = %+v, want healthy with package applied", machine)
	}
	t.Logf("FAKE_BREW_PATH_FIRST path=%s lookup=%s path_env=%s", fakeBrew.path, fakeBrew.lookedPath, fakeBrew.pathEnv)
	t.Logf("FAKE_BREW_ARGV_LOG\n%s", strings.Join(log, "\n"))
	t.Logf("OUTDATED_ASSERTION resource=%s status=in_sync command=upgrade dashboard=applied", resourceID)
}

const realSurfaceOutdatedFakeBrewScript = `#!/bin/sh
set -eu
args="$*"
printf '%s\n' "$args" >> "$BREW_ARGV_LOG"
upgraded_file="$BREW_FAKE_STATE_DIR/kubectl.upgraded"
case "$args" in
"list --versions kubectl")
	if [ -f "$upgraded_file" ]; then
		printf 'kubectl 1.31.0\n'
		exit 0
	fi
	printf 'kubectl 1.30.0\n'
	exit 0
	;;
"outdated --quiet --formula kubectl")
	if [ -f "$upgraded_file" ]; then
		exit 0
	fi
	printf 'kubectl\n'
	exit 1
	;;
"upgrade --formula kubectl")
	printf '1' > "$upgraded_file"
	exit 0
	;;
*)
	printf 'unexpected invocation: %s\n' "$args" >&2
	exit 98
	;;
esac
`
