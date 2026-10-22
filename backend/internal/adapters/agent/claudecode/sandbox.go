package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
)

// ErrSandboxUnavailable is returned when a worker would launch with the OS
// sandbox enabled but the platform cannot enforce it. Callers must not fall
// back to an unsandboxed worker.
var ErrSandboxUnavailable = errors.New("claude-code worker sandbox unavailable")

// sandboxUnavailableMessage is the actionable error shown in the UI and logs.
const sandboxUnavailableMessage = "Claude Code worker sandbox cannot be enforced on this system " +
	"(needs macOS Seatbelt, or bubblewrap and socat on Linux/WSL2). " +
	"Install bubblewrap and socat, or turn off the worker sandbox for this project."

// sandboxAvailability caches the one-shot platform probe used at daemon start
// and again at each worker launch.
var (
	sandboxAvailOnce   sync.Once
	sandboxAvailErr    error
	sandboxAvailLogged sync.Once
)

// lookPath is swapped in tests.
var lookPath = exec.LookPath

// gitRevParse runs `git rev-parse` in a worktree; swapped in tests.
var gitRevParse = func(worktree string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", worktree}, args...)...) //nolint:gosec // fixed git argv
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckSandboxAvailability reports whether Claude Code's OS sandbox can be
// enforced on this host. The result is cached. Native Windows is unsupported;
// macOS uses Seatbelt; Linux/WSL2 need bubblewrap and socat on PATH.
func CheckSandboxAvailability() error {
	sandboxAvailOnce.Do(func() {
		sandboxAvailErr = probeSandboxAvailability(runtime.GOOS)
	})
	return sandboxAvailErr
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
				return fmt.Errorf("%w: %s", ErrSandboxUnavailable, sandboxUnavailableMessage)
			}
		}
		if _, err := lookPath("socat"); err != nil {
			return fmt.Errorf("%w: %s", ErrSandboxUnavailable, sandboxUnavailableMessage)
		}
		return nil
	default:
		// Native Windows (and anything else) cannot enforce the sandbox.
		return fmt.Errorf("%w: %s", ErrSandboxUnavailable, sandboxUnavailableMessage)
	}
}

// resetSandboxAvailabilityForTest clears the availability cache between tests.
func resetSandboxAvailabilityForTest() {
	sandboxAvailOnce = sync.Once{}
	sandboxAvailErr = nil
	sandboxAvailLogged = sync.Once{}
}

// workerLaunchPolicy remaps bypass-permissions to accept-edits when the
// worker sandbox is on. bypassPermissions skips deny rules and, unless
// strictAllowlist is set, admits hosts outside the sandbox network allowlist;
// it also auto-approves unsandboxed retries when allowUnsandboxedCommands is
// left at its default. See https://code.claude.com/docs/en/sandboxing and
// https://code.claude.com/docs/en/permissions.
func workerLaunchPolicy(kind domain.SessionKind, sandbox domain.WorkerSandboxConfig, policy agentruntime.PermissionPolicy) agentruntime.PermissionPolicy {
	if kind != domain.KindWorker || !sandbox.IsEnabled() {
		return policy
	}
	if agentruntime.NormalizePermissionPolicy(policy) == agentruntime.PermissionBypassPermissions {
		return agentruntime.PermissionAcceptEdits
	}
	return policy
}

// workerProviderArgs builds the single --settings JSON (sandbox + permissions)
// and any --add-dir flags a sandboxed worker needs. Non-workers keep the
// existing auto-default nudge guard only.
func workerProviderArgs(
	kind domain.SessionKind,
	policy agentruntime.PermissionPolicy,
	sandbox domain.WorkerSandboxConfig,
	workspacePath string,
	daemonPort int,
) ([]string, error) {
	if kind != domain.KindWorker || !sandbox.IsEnabled() {
		return autoDefaultNudgeGuardArgs(policy), nil
	}
	if err := CheckSandboxAvailability(); err != nil {
		return nil, err
	}

	effective := workerLaunchPolicy(kind, sandbox, policy)
	settings, addDirs, err := buildWorkerSandboxSettings(effective, sandbox, workspacePath, daemonPort)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("claude-code: encode worker sandbox settings: %w", err)
	}
	args := []string{"--settings", string(raw)}
	for _, dir := range addDirs {
		args = append(args, "--add-dir", dir)
	}
	return args, nil
}

func buildWorkerSandboxSettings(
	policy agentruntime.PermissionPolicy,
	sandbox domain.WorkerSandboxConfig,
	workspacePath string,
	daemonPort int,
) (map[string]any, []string, error) {
	domains := append([]string(nil), sandbox.AllowedDomains...)
	if daemonPort > 0 {
		host := config.LoopbackHost
		domains = appendUniqueDomain(domains, fmt.Sprintf("%s:%d", host, daemonPort))
		domains = appendUniqueDomain(domains, fmt.Sprintf("localhost:%d", daemonPort))
	}

	gitDir, commonDir := resolveGitWritePaths(workspacePath)
	allowWrite := uniqueExistingDirs(gitDir, commonDir)

	sandboxBlock := map[string]any{
		"enabled":                   true,
		"failIfUnavailable":         true,
		"allowUnsandboxedCommands":  false,
		"autoAllowBashIfSandboxed":  true,
		// ao talks to the loopback daemon. On Linux, bubblewrap gives the
		// sandboxed command a private localhost that cannot reach the host
		// daemon; Claude Code also sets NO_PROXY for localhost so the proxy
		// allowlist never sees those connections
		// (https://code.claude.com/docs/en/sandboxing#a-command-fails-to-reach-a-server-on-localhost).
		// Exclude only the ao CLI so workers keep authenticated daemon access
		// without unsandboxing arbitrary shell.
		"excludedCommands": []string{"ao", "ao *"},
		"network": map[string]any{
			"allowedDomains": domains,
			// Keep bypass-configured (remapped) and auto sessions from opening
			// hosts outside the allowlist without a prompt.
			"strictAllowlist": true,
		},
	}
	if len(allowWrite) > 0 {
		sandboxBlock["filesystem"] = map[string]any{
			"allowWrite": allowWrite,
		}
	}

	permissions := map[string]any{}
	if flag := agentruntime.ClaudePermissionArgs(policy); len(flag) == 2 {
		permissions["defaultMode"] = flag[1]
	}
	if deny := workerEditDenyRules(workspacePath); len(deny) > 0 {
		permissions["deny"] = deny
	}

	settings := map[string]any{
		"sandbox": sandboxBlock,
	}
	if len(permissions) > 0 {
		settings["permissions"] = permissions
	}
	return settings, allowWrite, nil
}

func appendUniqueDomain(domains []string, next string) []string {
	for _, d := range domains {
		if d == next {
			return domains
		}
	}
	return append(domains, next)
}

func resolveGitWritePaths(workspacePath string) (gitDir, commonDir string) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return "", ""
	}
	gitDir, err := gitRevParse(workspacePath, "rev-parse", "--git-dir")
	if err != nil {
		return "", ""
	}
	commonDir, err = gitRevParse(workspacePath, "rev-parse", "--git-common-dir")
	if err != nil {
		commonDir = ""
	}
	gitDir = absFromWorktree(workspacePath, gitDir)
	commonDir = absFromWorktree(workspacePath, commonDir)
	return gitDir, commonDir
}

func absFromWorktree(worktree, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(worktree, p))
}

func uniqueExistingDirs(paths ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." || seen[p] {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			// git-dir for a linked worktree is a directory under
			// .git/worktrees/<name>; if it is missing, skip rather than
			// widening the allowlist.
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// workerEditDenyRules block Claude's Edit/Write tools outside the worktree.
// Write(path) rules never match; Edit(path) covers Write
// (https://code.claude.com/docs/en/permissions). Deny beats allow, so a broad
// absolute deny is paired with a gitignore-style negation for the worktree.
func workerEditDenyRules(workspacePath string) []string {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" || !filepath.IsAbs(workspacePath) {
		return nil
	}
	pattern := absoluteEditPattern(workspacePath)
	if pattern == "" {
		return nil
	}
	return []string{
		"Edit(//**)",
		"Edit(!" + pattern + "/**)",
	}
}

func absoluteEditPattern(abs string) string {
	abs = filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		slash := filepath.ToSlash(abs)
		if len(slash) >= 2 && slash[1] == ':' {
			drive := strings.ToLower(string(slash[0]))
			rest := slash[2:]
			return "//" + drive + rest
		}
		if strings.HasPrefix(slash, "/") {
			return "//" + strings.TrimPrefix(slash, "/")
		}
		return ""
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		return ""
	}
	return "//" + strings.TrimPrefix(slash, "/")
}

// applyWorkerSandboxToLaunch is shared by GetLaunchCommand and GetRestoreCommand.
func applyWorkerSandboxToLaunch(
	kind domain.SessionKind,
	permissions ports.PermissionMode,
	sandbox domain.WorkerSandboxConfig,
	workspacePath string,
	daemonPort int,
) (policy agentruntime.PermissionPolicy, providerArgs []string, err error) {
	base := agentruntime.PermissionPolicy(permissions)
	policy = workerLaunchPolicy(kind, sandbox, base)
	providerArgs, err = workerProviderArgs(kind, base, sandbox, workspacePath, daemonPort)
	if err != nil {
		return "", nil, err
	}
	return policy, providerArgs, nil
}
