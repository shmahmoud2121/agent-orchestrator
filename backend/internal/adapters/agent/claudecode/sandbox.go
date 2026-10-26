package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	aoprocess "github.com/aoagents/agent-orchestrator/backend/internal/process"
	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
)

// ErrSandboxUnavailable is returned when a worker would launch with the OS
// sandbox enabled but the platform cannot enforce it. Callers must not fall
// back to an unsandboxed worker.
var ErrSandboxUnavailable = errors.New("claude-code worker sandbox unavailable")

// ErrSandboxUnavailableWindows is the native-Windows form of
// ErrSandboxUnavailable. Claude Code's OS sandbox does not run on Windows.
var ErrSandboxUnavailableWindows = errors.New("claude-code worker sandbox does not run on Windows")

// ErrSandboxClaudeTooOld is returned when the installed Claude Code binary is
// older than the release that confines worktree sandbox writes to the shared
// .git directory (hooks/ and config still denied). Widening allowWrite ourselves
// would re-open those paths.
var ErrSandboxClaudeTooOld = errors.New("claude-code worker sandbox requires Claude Code 2.1.268 or newer")

// SandboxUnavailableMessage is the actionable error shown in the UI and logs
// when the OS sandbox cannot be enforced (missing bubblewrap or socat, or
// Claude Code too old). Native Windows uses ErrSandboxUnavailableWindows
// instead of this text.
const SandboxUnavailableMessage = "Claude Code worker sandbox cannot be enforced on this system " +
	"(needs macOS Seatbelt, or bubblewrap and socat on Linux/WSL2). " +
	"Install bubblewrap and socat, or turn off the worker sandbox for this project."

// minWorktreeSandboxVersion is Claude Code v2.1.268. v2.1.149 fixed
// linked-worktree sandboxing so writes go only to the shared main-repo .git
// (with hooks/ and config denied); see
// https://github.com/anthropics/claude-code/releases/tag/v2.1.149. The
// protected-file rules need v2.1.224, which stopped a trailing slash from
// opening a denyRead folder, and v2.1.268, which applies Read deny rules at a
// symlinked folder's real location
// (https://code.claude.com/docs/en/settings-reference#sandbox-path-prefixes,
// https://code.claude.com/docs/en/permissions).
var minWorktreeSandboxVersion = [3]int{2, 1, 268}

// sandboxAvailability caches the one-shot platform probe used at daemon start
// and again at each worker launch.
var (
	sandboxAvailOnce   sync.Once
	sandboxAvailErr    error
	sandboxAvailLogged sync.Once
)

// lookPath is swapped in tests.
var lookPath = exec.LookPath

// sandboxAvailabilityProbe is the platform check behind CheckSandboxAvailability.
// Tests outside this package replace it via OverrideSandboxAvailabilityForTest.
var sandboxAvailabilityProbe = defaultSandboxAvailabilityProbe

func defaultSandboxAvailabilityProbe() error {
	return probeSandboxAvailability(runtime.GOOS)
}

// claudeVersionCommand runs `claude --version`; swapped in tests.
var claudeVersionCommand = func(ctx context.Context, binary string) ([]byte, error) {
	return aoprocess.CommandContext(ctx, binary, "--version").Output() //nolint:gosec // binary is adapter-resolved
}

var claudeVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

const claudeVersionProbeTimeout = 15 * time.Second

// CheckSandboxAvailability reports whether Claude Code's OS sandbox can be
// enforced on this host. The result is cached. Native Windows is unsupported;
// macOS uses Seatbelt; Linux/WSL2 need bubblewrap and socat on PATH.
func CheckSandboxAvailability() error {
	sandboxAvailOnce.Do(func() {
		sandboxAvailErr = sandboxAvailabilityProbe()
	})
	return sandboxAvailErr
}

// OverrideSandboxAvailabilityForTest replaces the availability probe and clears
// the cache. Restore undoes both. Intended for session_manager and other
// packages whose tests are not about the sandbox but still build Claude worker
// launch argv.
func OverrideSandboxAvailabilityForTest(probe func() error) (restore func()) {
	prevProbe := sandboxAvailabilityProbe
	resetSandboxAvailabilityForTest()
	if probe == nil {
		sandboxAvailabilityProbe = defaultSandboxAvailabilityProbe
	} else {
		sandboxAvailabilityProbe = probe
	}
	return func() {
		sandboxAvailabilityProbe = prevProbe
		resetSandboxAvailabilityForTest()
	}
}

// ForceSandboxReadyForTest makes the platform probe and the Claude Code
// version gate succeed. session_manager TestMain uses this so fixtures that
// build worker argv with a fake `claude` binary are not blocked by bubblewrap
// or --version.
func ForceSandboxReadyForTest() (restore func()) {
	restoreAvail := OverrideSandboxAvailabilityForTest(func() error { return nil })
	prevVersion := claudeVersionCommand
	claudeVersionCommand = func(context.Context, string) ([]byte, error) {
		return []byte("2.1.308 (Claude Code)\n"), nil
	}
	return func() {
		claudeVersionCommand = prevVersion
		restoreAvail()
	}
}

// LogSandboxAvailabilityOnce logs the platform probe result a single time
// (intended at daemon start).
func LogSandboxAvailabilityOnce(log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if log == nil {
		return
	}
	sandboxAvailLogged.Do(func() {
		if err := CheckSandboxAvailability(); err != nil {
			log.Warn("claude-code worker sandbox unavailable", "error", err.Error())
			return
		}
		log.Info("claude-code worker sandbox available", "goos", runtime.GOOS)
	})
}

func probeSandboxAvailability(goos string) error {
	switch goos {
	case "darwin":
		return nil
	case "linux":
		if _, err := lookPath("bwrap"); err != nil {
			if _, err2 := lookPath("bubblewrap"); err2 != nil {
				return fmt.Errorf("%w: %s", ErrSandboxUnavailable, SandboxUnavailableMessage)
			}
		}
		if _, err := lookPath("socat"); err != nil {
			return fmt.Errorf("%w: %s", ErrSandboxUnavailable, SandboxUnavailableMessage)
		}
		return nil
	default:
		// Native Windows (and anything else) cannot enforce the sandbox.
		return WindowsSandboxUnavailableError()
	}
}

// WindowsSandboxUnavailableError is the probe failure for native Windows.
// Tests fake this with OverrideSandboxAvailabilityForTest.
func WindowsSandboxUnavailableError() error {
	return fmt.Errorf("%w: %w", ErrSandboxUnavailable, ErrSandboxUnavailableWindows)
}

// WorkerSandboxOSState reports whether this host can sandbox workers and the
// 409 body to use when it cannot. available is true iff CheckSandboxAvailability
// succeeds.
func WorkerSandboxOSState() (available bool, message string) {
	err := CheckSandboxAvailability()
	if err == nil {
		return true, ""
	}
	if errors.Is(err, ErrSandboxUnavailableWindows) {
		return false, domain.WorkerSandboxRequiredWindowsMessage
	}
	return false, SandboxUnavailableMessage + " " + domain.WorkerSandboxRequiredSettingSentence
}

// resetSandboxAvailabilityForTest clears the availability cache between tests.
func resetSandboxAvailabilityForTest() {
	sandboxAvailOnce = sync.Once{}
	sandboxAvailErr = nil
	sandboxAvailLogged = sync.Once{}
}

// workerLaunchPolicy remaps a worker's bypass-permissions to accept-edits
// whether or not the OS sandbox is on. bypassPermissions skips deny rules.
// The sandbox argument stays so existing callers keep their signature; the
// sandbox settings themselves are unchanged. See
// https://code.claude.com/docs/en/sandboxing and
// https://code.claude.com/docs/en/permissions.
func workerLaunchPolicy(kind domain.SessionKind, _ domain.WorkerSandboxConfig, policy agentruntime.PermissionPolicy) agentruntime.PermissionPolicy {
	return agentruntime.PermissionPolicy(domain.WorkerLaunchPermission(kind, domain.PermissionMode(policy)))
}

// EffectiveWorkerPermission remaps bypass→acceptEdits for every worker.
// Used by both the terminal CLI launch and Claude chat ACP session mode.
func EffectiveWorkerPermission(kind domain.SessionKind, sandbox domain.WorkerSandboxConfig, permissions ports.PermissionMode) ports.PermissionMode {
	return ports.PermissionMode(workerLaunchPolicy(kind, sandbox, agentruntime.PermissionPolicy(permissions)))
}

// WorkerSandboxSettings returns the settings object passed to Claude Code
// (--settings for TUI, _meta.claudeCode.options.settings for chat ACP) when
// the worker sandbox is enabled. Nil when sandboxing does not apply.
func WorkerSandboxSettings(kind domain.SessionKind, permissions ports.PermissionMode, sandbox domain.WorkerSandboxConfig) (map[string]any, error) {
	if kind != domain.KindWorker || !sandbox.IsEnabled() {
		return nil, nil
	}
	effective := workerLaunchPolicy(kind, sandbox, agentruntime.PermissionPolicy(permissions))
	return buildWorkerSandboxSettings(effective, sandbox)
}

// EnsureWorkerSandboxReady refuses a sandboxed worker when the host cannot
// enforce the OS sandbox or Claude Code is older than the worktree .git fix.
func EnsureWorkerSandboxReady(ctx context.Context, binary string, kind domain.SessionKind, sandbox domain.WorkerSandboxConfig) error {
	if kind != domain.KindWorker || !sandbox.IsEnabled() {
		return nil
	}
	if err := CheckSandboxAvailability(); err != nil {
		return err
	}
	return requireMinClaudeSandboxVersion(ctx, binary)
}

// workerProviderArgs builds the single --settings JSON (sandbox + permissions)
// a sandboxed worker needs. Non-workers keep the existing auto-default nudge
// guard only.
func workerProviderArgs(
	kind domain.SessionKind,
	policy agentruntime.PermissionPolicy,
	sandbox domain.WorkerSandboxConfig,
) ([]string, error) {
	if kind != domain.KindWorker || !sandbox.IsEnabled() {
		return autoDefaultNudgeGuardArgs(policy), nil
	}
	if err := CheckSandboxAvailability(); err != nil {
		return nil, err
	}

	effective := workerLaunchPolicy(kind, sandbox, policy)
	settings, err := buildWorkerSandboxSettings(effective, sandbox)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("claude-code: encode worker sandbox settings: %w", err)
	}
	return []string{"--settings", string(raw)}, nil
}

func buildWorkerSandboxSettings(
	policy agentruntime.PermissionPolicy,
	sandbox domain.WorkerSandboxConfig,
) (map[string]any, error) {
	domains := append([]string(nil), sandbox.AllowedDomains...)
	denyRead, readRules := readDenySettings(workerReadDenyPaths(sandbox), runtime.GOOS)

	sandboxBlock := map[string]any{
		"enabled":                  true,
		"failIfUnavailable":        true,
		"allowUnsandboxedCommands": false,
		"autoAllowBashIfSandboxed": true,
		// ao talks to the loopback daemon. On Linux, bubblewrap gives the
		// sandboxed command a private localhost that cannot reach the host
		// daemon; Claude Code also sets NO_PROXY for localhost so the proxy
		// allowlist never sees those connections
		// (https://code.claude.com/docs/en/sandboxing#a-command-fails-to-reach-a-server-on-localhost).
		// Exclude only the ao CLI so workers keep authenticated daemon access
		// without unsandboxing arbitrary shell. Matching permissions.allow
		// rules below auto-approve those excluded commands under acceptEdits
		// (https://code.claude.com/docs/en/permissions).
		"excludedCommands": []string{"ao", "ao *"},
		"network": map[string]any{
			"allowedDomains": domains,
			// Keep bypass-configured (remapped) and auto sessions from opening
			// hosts outside the allowlist without a prompt.
			"strictAllowlist": true,
		},
		// Shell commands may not read the protected files. The excluded ao CLI
		// runs outside the sandbox, so it still reads the API token.
		"filesystem": map[string]any{
			"denyRead": denyRead,
		},
	}

	// acceptEdits auto-approves Edit/Write only inside the working directory
	// and additionalDirectories; edits elsewhere prompt the user, so they do
	// not escape. Do not pair Edit(//**) with Edit(!…) carve-outs: Claude Code
	// reads a ! pattern relative to the current directory even when /, ~/, or
	// // follows the !, so the negation cannot reach an absolute-anchored deny
	// (https://code.claude.com/docs/en/permissions).
	//
	// disableBypassPermissionsMode keeps Claude itself from entering bypass
	// even if AO's session/set_mode path regresses; the OS sandbox only covers
	// Bash, so bypass would otherwise auto-approve Edit/Write anywhere.
	permissions := map[string]any{
		"allow": []string{"Bash(ao)", "Bash(ao *)"},
		// Deny wins over the ao allow rules. A worker can otherwise clear
		// AO_SESSION_ID and run `ao project set-config` or `ao project trust`
		// outside the sandbox, where the daemon would see no session header.
		"deny": append([]string{
			"Bash(ao project set-config)",
			"Bash(ao project set-config *)",
			"Bash(ao project set-config:*)",
			"Bash(ao project trust)",
			"Bash(ao project trust *)",
			"Bash(ao project trust:*)",
		}, readRules...),
		"disableBypassPermissionsMode": "disable",
	}
	if flag := agentruntime.ClaudePermissionArgs(policy); len(flag) == 2 {
		permissions["defaultMode"] = flag[1]
	}

	return map[string]any{
		"sandbox":     sandboxBlock,
		"permissions": permissions,
	}, nil
}

// requireMinClaudeSandboxVersion refuses sandboxed workers on Claude Code
// older than minWorktreeSandboxVersion. That release added built-in worktree
// .git write support with hooks/config still denied; AO must not widen those
// paths itself.
func requireMinClaudeSandboxVersion(ctx context.Context, binary string) error {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return fmt.Errorf("%w: empty claude binary", ErrSandboxClaudeTooOld)
	}
	ctx, cancel := context.WithTimeout(ctx, claudeVersionProbeTimeout)
	defer cancel()
	out, err := claudeVersionCommand(ctx, binary)
	if err != nil {
		return fmt.Errorf("%w: probe claude --version: %w", ErrSandboxClaudeTooOld, err)
	}
	if !claudeVersionAtLeast(string(out), minWorktreeSandboxVersion) {
		return fmt.Errorf("%w (got %q)", ErrSandboxClaudeTooOld, strings.TrimSpace(string(out)))
	}
	return nil
}

func claudeVersionAtLeast(output string, minVersion [3]int) bool {
	match := claudeVersionPattern.FindStringSubmatch(output)
	if match == nil {
		return false
	}
	for i, want := range minVersion {
		got, err := strconv.Atoi(match[i+1])
		if err != nil {
			return false
		}
		if got != want {
			return got > want
		}
	}
	return true
}

// applyWorkerSandboxToLaunch is shared by GetLaunchCommand and GetRestoreCommand.
func applyWorkerSandboxToLaunch(
	ctx context.Context,
	kind domain.SessionKind,
	permissions ports.PermissionMode,
	sandbox domain.WorkerSandboxConfig,
	binary string,
) (policy agentruntime.PermissionPolicy, providerArgs []string, err error) {
	base := agentruntime.PermissionPolicy(permissions)
	policy = workerLaunchPolicy(kind, sandbox, base)
	if kind == domain.KindWorker && sandbox.IsEnabled() {
		// Availability before the version probe so a missing bubblewrap/socat
		// surfaces as ErrSandboxUnavailable rather than a failed --version exec.
		if err := CheckSandboxAvailability(); err != nil {
			return "", nil, err
		}
		if err := requireMinClaudeSandboxVersion(ctx, binary); err != nil {
			return "", nil, err
		}
	}
	providerArgs, err = workerProviderArgs(kind, base, sandbox)
	if err != nil {
		return "", nil, err
	}
	return policy, providerArgs, nil
}
