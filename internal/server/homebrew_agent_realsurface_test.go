package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4gly/neul/internal/agent"
)

func TestFakeBrewAgent_RealSurfaceAppliesLatestAndUpdatesDashboard(t *testing.T) {
	// Given
	fixture := newHomebrewRealSurfaceFixture(t, "machine_fake_brew_apply", "mtn_fake_brew_apply")
	resourceID := createPackageResourceWithVersion(t, fixture.router, fixture.cookie, "kubectl", "latest")
	seedQueuedAgentCommand(t, fixture.db, "command_fake_brew_apply", fixture.machineID, "reconcile_now", `{}`, fixture.now)
	fakeBrew := installRealSurfaceFakeBrew(t)
	client := newRealSurfaceAgent(t, fixture.server.URL, fixture.machineID, fixture.machineToken, "darwin")

	// When
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Then
	log := readFakeBrewLog(t, fakeBrew.logPath)
	assertLogContainsInOrder(t, log,
		"list --versions kubectl",
		"install --formula kubectl",
		"list --versions kubectl",
		"outdated --quiet --formula kubectl",
	)
	assertCommandStatus(t, fixture.db, "command_fake_brew_apply", "finished")
	assertLatestResourceEvent(t, fixture.db, resourceID, "in_sync", "brew apply")
	machine := dashboardMachineByID(t, fixture.router, fixture.cookie, fixture.machineID)
	if machine.Status != "healthy" || machine.ResourceCount != 1 || machine.AppliedCount != 1 || machine.BlockedCount != 0 {
		t.Fatalf("dashboard machine = %+v, want healthy with package applied", machine)
	}
	t.Logf("FAKE_BREW_PATH_FIRST path=%s lookup=%s path_env=%s", fakeBrew.path, fakeBrew.lookedPath, fakeBrew.pathEnv)
	t.Logf("FAKE_BREW_ARGV_LOG\n%s", strings.Join(log, "\n"))
	t.Logf("DASHBOARD_ASSERTION machine=%s status=%s applied=%d/%d blocked=%d resource=%s", machine.ID, machine.Status, machine.AppliedCount, machine.ResourceCount, machine.BlockedCount, resourceID)
}

func TestFakeBrewAgent_RealSurfacePinnedVersionBlocksWithoutMutation(t *testing.T) {
	// Given
	fixture := newHomebrewRealSurfaceFixture(t, "machine_fake_brew_pinned", "mtn_fake_brew_pinned")
	resourceID := createPackageResourceWithVersion(t, fixture.router, fixture.cookie, "kubectl", "1.31.0")
	seedQueuedAgentCommand(t, fixture.db, "command_fake_brew_pinned", fixture.machineID, "reconcile_now", `{}`, fixture.now)
	fakeBrew := installRealSurfaceFakeBrew(t)
	client := newRealSurfaceAgent(t, fixture.server.URL, fixture.machineID, fixture.machineToken, "darwin")

	// When
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Then
	log := readFakeBrewLog(t, fakeBrew.logPath)
	assertNoFakeBrewMutation(t, log)
	assertCommandStatus(t, fixture.db, "command_fake_brew_pinned", "finished")
	assertLatestResourceEvent(t, fixture.db, resourceID, "blocked", "brew_pinned_unsupported")
	t.Logf("FAKE_BREW_PATH_FIRST path=%s lookup=%s path_env=%s", fakeBrew.path, fakeBrew.lookedPath, fakeBrew.pathEnv)
	t.Logf("FAKE_BREW_ARGV_LOG\n%s", strings.Join(log, "\n"))
	t.Logf("PINNED_ASSERTION resource=%s status=blocked prefix=brew_pinned_unsupported mutation=false", resourceID)
}

func TestFakeBrewAgent_RealSurfaceUnsupportedHostBlocksWithoutMutation(t *testing.T) {
	// Given
	fixture := newHomebrewRealSurfaceFixture(t, "machine_fake_brew_host", "mtn_fake_brew_host")
	resourceID := createPackageResourceWithVersion(t, fixture.router, fixture.cookie, "kubectl", "latest")
	seedQueuedAgentCommand(t, fixture.db, "command_fake_brew_host", fixture.machineID, "reconcile_now", `{}`, fixture.now)
	fakeBrew := installRealSurfaceFakeBrew(t)
	client := newRealSurfaceAgent(t, fixture.server.URL, fixture.machineID, fixture.machineToken, "linux")

	// When
	if err := client.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Then
	log := readFakeBrewLog(t, fakeBrew.logPath)
	if len(log) != 0 {
		t.Fatalf("fake brew log = %v, want no invocation for unsupported host", log)
	}
	assertCommandStatus(t, fixture.db, "command_fake_brew_host", "finished")
	assertLatestResourceEvent(t, fixture.db, resourceID, "blocked", "unsupported_host")
	t.Logf("FAKE_BREW_PATH_FIRST path=%s lookup=%s path_env=%s", fakeBrew.path, fakeBrew.lookedPath, fakeBrew.pathEnv)
	t.Log("FAKE_BREW_ARGV_LOG empty")
	t.Logf("UNSUPPORTED_HOST_ASSERTION resource=%s status=blocked prefix=unsupported_host mutation=false", resourceID)
}

type homebrewRealSurfaceFixture struct {
	db           *sql.DB
	router       http.Handler
	cookie       *http.Cookie
	server       *httptest.Server
	now          time.Time
	machineID    string
	machineToken string
}

type realSurfaceFakeBrew struct {
	path       string
	logPath    string
	lookedPath string
	pathEnv    string
}

func newHomebrewRealSurfaceFixture(t *testing.T, machineID string, machineToken string) homebrewRealSurfaceFixture {
	t.Helper()
	now := time.Now().UTC()
	db := openServerTestDB(t)
	router, cookie := authenticatedRouterWithConfig(t, Config{DB: db, Clock: func() time.Time { return now }, HomeDir: t.TempDir()})
	seedMachineWithToken(t, db, machineID, machineToken)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return homebrewRealSurfaceFixture{db: db, router: router, cookie: cookie, server: server, now: now, machineID: machineID, machineToken: machineToken}
}

func newRealSurfaceAgent(t *testing.T, serverURL string, machineID string, machineToken string, goos string) *agent.Client {
	t.Helper()
	adapter := agent.NewHomebrewAdapter(agent.WithHomebrewGOOS(goos))
	return agent.NewWithAdapters(agent.Config{ServerURL: serverURL, MachineID: machineID, MachineToken: machineToken}, adapter)
}

func installRealSurfaceFakeBrew(t *testing.T) realSurfaceFakeBrew {
	t.Helper()
	return installRealSurfaceFakeBrewWithScript(t, realSurfaceFakeBrewScript)
}

func createPackageResourceWithVersion(t *testing.T, router http.Handler, cookie *http.Cookie, name string, desiredVersion string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"sourceKind":"brew","desiredVersion":%q,"targetSegment":"base"}`, name, desiredVersion)
	request := httptest.NewRequest(http.MethodPost, "/api/resources/package", strings.NewReader(body))
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create package status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	decodeJSONResponse(t, recorder, &response)
	if response.ID == "" {
		t.Fatal("created package resource id is empty")
	}
	return response.ID
}

func seedQueuedAgentCommand(t *testing.T, db *sql.DB, commandID string, machineID string, commandType string, payloadJSON string, now time.Time) {
	t.Helper()
	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO agent_commands (id, machine_id, command_type, payload_json, idempotency_key, status, created_at) VALUES (?, ?, ?, ?, ?, 'queued', ?)`,
		commandID,
		machineID,
		commandType,
		payloadJSON,
		"seed-"+commandID,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert command error = %v", err)
	}
}

func assertLatestResourceEvent(t *testing.T, db *sql.DB, resourceID string, wantStatus string, wantMessagePrefix string) {
	t.Helper()
	var status string
	var message string
	err := db.QueryRowContext(
		context.Background(),
		`SELECT e.status, e.message
		 FROM reconcile_events e
		 JOIN reconcile_runs r ON r.id = e.run_id
		 WHERE e.resource_id = ?
		 ORDER BY unixepoch(e.created_at) DESC, e.created_at DESC, e.rowid DESC
		 LIMIT 1`,
		resourceID,
	).Scan(&status, &message)
	if err != nil {
		t.Fatalf("query latest event error = %v", err)
	}
	if status != wantStatus || !strings.HasPrefix(message, wantMessagePrefix) {
		t.Fatalf("latest event = status:%s message:%q, want status:%s prefix:%s", status, message, wantStatus, wantMessagePrefix)
	}
}

func readFakeBrewLog(t *testing.T, logPath string) []string {
	t.Helper()
	contents, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	text := strings.TrimSuffix(string(contents), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func assertLogContainsInOrder(t *testing.T, log []string, want ...string) {
	t.Helper()
	cursor := 0
	for _, line := range log {
		if cursor < len(want) && line == want[cursor] {
			cursor++
		}
	}
	if cursor != len(want) {
		t.Fatalf("fake brew log = %v, want ordered entries %v", log, want)
	}
}

func assertNoFakeBrewMutation(t *testing.T, log []string) {
	t.Helper()
	for _, line := range log {
		if strings.HasPrefix(line, "install ") || strings.HasPrefix(line, "upgrade ") {
			t.Fatalf("fake brew log contains mutation command %q", line)
		}
	}
}

const realSurfaceFakeBrewScript = `#!/bin/sh
set -eu
args="$*"
printf '%s\n' "$args" >> "$BREW_ARGV_LOG"
installed_file="$BREW_FAKE_STATE_DIR/kubectl.installed"
case "$args" in
"list --versions kubectl")
	if [ -f "$installed_file" ]; then
		printf 'kubectl 1.31.0\n'
		exit 0
	fi
	exit 1
	;;
"outdated --quiet --formula kubectl")
	exit 0
	;;
"install --formula kubectl")
	printf '1' > "$installed_file"
	exit 0
	;;
*)
	printf 'unexpected invocation: %s\n' "$args" >&2
	exit 98
	;;
esac
`
