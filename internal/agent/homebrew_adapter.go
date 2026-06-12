package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

const outputContextMaxLen = 256

var homebrewFormulaNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

type HomebrewOption func(*homebrewAdapter)

type homebrewAdapter struct {
	goos     string
	lookPath func(string) (string, error)
	run      homebrewRunFunc
	environ  func() []string
}

type homebrewRunFunc func(ctx context.Context, path string, env []string, args ...string) homebrewCommandResult

type homebrewCommandResult struct {
	stdout, stderr string
	exitCode       int
	err            error
}

type homebrewCheckResult struct {
	status    string
	installed bool
}

type homebrewInstallState struct {
	installed bool
	versions  []string
}

func NewHomebrewAdapter(options ...HomebrewOption) PackageAdapter {
	adapter := &homebrewAdapter{
		goos:     runtime.GOOS,
		lookPath: exec.LookPath,
		run:      runHomebrewCommand,
		environ:  os.Environ,
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithHomebrewGOOS(goos string) HomebrewOption {
	return func(adapter *homebrewAdapter) {
		adapter.goos = goos
	}
}

func (a *homebrewAdapter) Check(ctx context.Context, name string, desiredVersion string) (string, error) {
	desiredVersion = normalizeDesiredVersion(desiredVersion)
	check, err := a.check(ctx, name, desiredVersion)
	if err != nil {
		return "", err
	}
	return check.status, nil
}

func (a *homebrewAdapter) Apply(ctx context.Context, name string, desiredVersion string) (string, error) {
	desiredVersion = normalizeDesiredVersion(desiredVersion)
	check, err := a.check(ctx, name, desiredVersion)
	if err != nil {
		return "", err
	}
	if check.status == "in_sync" {
		return "in_sync", nil
	}
	if desiredVersion != "latest" {
		return "", fmt.Errorf("brew_pinned_unsupported: desired version %s cannot be applied for %s", desiredVersion, name)
	}
	path, err := a.discover()
	if err != nil {
		return "", err
	}
	args := []string{"install", "--formula", name}
	if check.installed {
		args = []string{"upgrade", "--formula", name}
	}
	if result := a.run(ctx, path, homebrewEnv(a.environ()), args...); result.err != nil {
		return "", commandContextError("brew_apply_failed", args, result, "mutation failed")
	}
	postCheck, err := a.check(ctx, name, desiredVersion)
	if err != nil {
		return "", fmt.Errorf("brew_apply_failed: post-check failed: %w", err)
	}
	if postCheck.status != "in_sync" {
		return "", fmt.Errorf("brew_apply_failed: post-check status %s for %s", postCheck.status, name)
	}
	return "in_sync", nil
}

func (a *homebrewAdapter) check(ctx context.Context, name string, desiredVersion string) (homebrewCheckResult, error) {
	if err := validateHomebrewFormulaName(name); err != nil {
		return homebrewCheckResult{}, err
	}
	path, err := a.discover()
	if err != nil {
		return homebrewCheckResult{}, err
	}
	state, err := a.installedState(ctx, path, name)
	if err != nil {
		return homebrewCheckResult{}, err
	}
	if !state.installed {
		return homebrewCheckResult{status: "drifted"}, nil
	}
	if desiredVersion != "latest" {
		if containsString(state.versions, desiredVersion) {
			return homebrewCheckResult{status: "in_sync", installed: true}, nil
		}
		return homebrewCheckResult{status: "drifted", installed: true}, nil
	}
	outdated, err := a.outdated(ctx, path, name)
	if err != nil {
		return homebrewCheckResult{}, err
	}
	if outdated {
		return homebrewCheckResult{status: "drifted", installed: true}, nil
	}
	return homebrewCheckResult{status: "in_sync", installed: true}, nil
}

func (a *homebrewAdapter) discover() (string, error) {
	if a.goos != "darwin" {
		return "", fmt.Errorf("unsupported_host: homebrew adapter requires darwin, got %s", a.goos)
	}
	path, err := a.lookPath("brew")
	if err != nil {
		return "", fmt.Errorf("brew_unavailable: brew executable not found on PATH: %w", err)
	}
	return path, nil
}

func (a *homebrewAdapter) installedState(ctx context.Context, path string, name string) (homebrewInstallState, error) {
	args := []string{"list", "--versions", name}
	result := a.run(ctx, path, homebrewEnv(a.environ()), args...)
	if result.err != nil {
		if strings.TrimSpace(result.stdout) == "" && (strings.TrimSpace(result.stderr) == "" || strings.Contains(result.stderr, "No such keg")) {
			return homebrewInstallState{}, nil
		}
		return homebrewInstallState{}, commandContextError("brew_check_failed", args, result, "installed-state check failed")
	}
	if strings.TrimSpace(result.stderr) != "" {
		return homebrewInstallState{}, commandContextError("brew_check_failed", args, result, "unexpected stderr")
	}
	stdout := strings.TrimSpace(result.stdout)
	if stdout == "" {
		return homebrewInstallState{}, nil
	}
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return homebrewInstallState{installed: true, versions: fields[1:]}, nil
		}
	}
	return homebrewInstallState{}, commandContextError("brew_check_failed", args, result, "unexpected installed-state output")
}

func (a *homebrewAdapter) outdated(ctx context.Context, path string, name string) (bool, error) {
	args := []string{"outdated", "--quiet", "--formula", name}
	result := a.run(ctx, path, homebrewEnv(a.environ()), args...)
	stdout := strings.TrimSpace(result.stdout)
	stderr := strings.TrimSpace(result.stderr)
	if stderr == "" && stdout == name && (result.exitCode == 0 || result.exitCode == 1) {
		return true, nil
	}
	if stderr == "" && stdout == "" && result.exitCode == 0 {
		return false, nil
	}
	return false, commandContextError("brew_check_failed", args, result, "outdated check failed")
}

func runHomebrewCommand(ctx context.Context, path string, env []string, args ...string) homebrewCommandResult {
	command := exec.CommandContext(ctx, path, args...)
	command.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return homebrewCommandResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode, err: err}
}

func homebrewEnv(base []string) []string {
	env := append([]string(nil), base...)
	return append(env,
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_INSTALL_CLEANUP=1",
		"HOMEBREW_NO_COLOR=1",
		"HOMEBREW_NO_ENV_HINTS=1",
	)
}

func validateHomebrewFormulaName(name string) error {
	if !homebrewFormulaNamePattern.MatchString(name) {
		return fmt.Errorf("brew_unsupported_name: unsupported Homebrew formula name %q", name)
	}
	return nil
}

func normalizeDesiredVersion(desiredVersion string) string {
	desiredVersion = strings.TrimSpace(desiredVersion)
	if desiredVersion == "" {
		return "latest"
	}
	return desiredVersion
}

func commandContextError(prefix string, args []string, result homebrewCommandResult, detail string) error {
	parts := []string{fmt.Sprintf("%s: %s: command %s", prefix, detail, commandText(args))}
	if result.exitCode >= 0 {
		parts = append(parts, fmt.Sprintf("exit %d", result.exitCode))
	}
	if result.err != nil && result.exitCode < 0 {
		parts = append(parts, fmt.Sprintf("error %v", result.err))
	}
	if stdout := boundedOutput(result.stdout); stdout != "" {
		parts = append(parts, fmt.Sprintf("stdout %q", stdout))
	}
	if stderr := boundedOutput(result.stderr); stderr != "" {
		parts = append(parts, fmt.Sprintf("stderr %q", stderr))
	}
	return errors.New(strings.Join(parts, "; "))
}

func commandText(args []string) string {
	return "brew " + strings.Join(args, " ")
}

func boundedOutput(value string) string {
	if len(value) <= outputContextMaxLen {
		return value
	}
	cut := outputContextMaxLen
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut] + "... [truncated]"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
