package domain

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"unicode"
)

// WorkerSandboxConfig controls Claude Code's OS sandbox for worker sessions.
// Orchestrator and reviewer sessions ignore it.
//
// When the field is absent from stored JSON, IsEnabled reports true so existing
// projects gain the sandbox without a migration. Set Enabled to a false bool to
// opt out (unsafe).
type WorkerSandboxConfig struct {
	// Enabled turns the worker OS sandbox on. nil means true (the default).
	Enabled *bool `json:"enabled,omitempty"`
	// AllowedDomains is the sandbox network allowlist. Empty means no outbound
	// network through the sandbox proxy. The ao CLI is excluded from the
	// sandbox (and allowlisted under permissions) so it can still reach the
	// loopback daemon. Entries are hostnames or "*.example.com" only — no
	// schemes, paths, or ports.
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	// ProtectedPaths adds files workers may not read to the built-in list
	// (BuiltinWorkerProtectedPaths). Each entry is relative to the project, a
	// "~/" path, or absolute. The person and the manager may add entries; only
	// the person may remove one.
	ProtectedPaths []string `json:"protectedPaths,omitempty"`
	// RequireSandbox refuses to start or restore a worker unless that worker
	// will actually run sandboxed. nil means on. Only the person can change it.
	RequireSandbox *bool `json:"requireSandbox,omitempty"`

	// Launch is filled in by the session manager for each worker launch and is
	// never stored or sent over the API.
	Launch WorkerSandboxLaunch `json:"-"`
}

// WorkerSandboxLaunch is the per-launch context the read-deny rules are built
// from. Empty fields contribute no rules.
type WorkerSandboxLaunch struct {
	// ProjectPath is the project's registered path (the main checkout).
	ProjectPath string
	// WorkspacePath is the worker's own worktree.
	WorkspacePath string
	// HomeDir is the real home folder. Empty means os.UserHomeDir.
	HomeDir string
	// APITokenPath is AO's install token file (apitoken.Path).
	APITokenPath string
	// WriteAllow is the only folders a Work worker may write. Empty leaves the
	// sandbox's own write rules unchanged, which Code worktrees rely on.
	WriteAllow []string
	// WriteDeny is the Work project's original files and folders. Empty means
	// this launch has no extra write-deny list.
	WriteDeny []string
	// ReadableDirs are folders Claude Code may be given with --add-dir.
	// Empty for every other launch. Codex must not receive them.
	ReadableDirs []string
}

// IsEnabled reports whether worker sandboxing is on. A missing config (nil
// Enabled) defaults to true.
func (c WorkerSandboxConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// IsRequired reports whether workers must actually run sandboxed. A missing
// config (nil RequireSandbox) defaults to true.
func (c WorkerSandboxConfig) IsRequired() bool {
	if c.RequireSandbox == nil {
		return true
	}
	return *c.RequireSandbox
}

// SandboxedWorkerHarnesses is the harnesses that can actually run a worker
// inside an OS sandbox. Only Claude Code qualifies today; add others here
// when they grow an equivalent sandbox.
var SandboxedWorkerHarnesses = map[AgentHarness]struct{}{
	HarnessClaudeCode: {},
}

// ErrWorkerSandboxRequired is returned when a project requires sandboxed
// workers and the launch would not actually be sandboxed.
var ErrWorkerSandboxRequired = errors.New("worker sandbox required")

// WorkerSandboxRequiredMessage is the 409 body for WORKER_SANDBOX_REQUIRED
// when the host can sandbox but this harness would not actually be sandboxed.
const WorkerSandboxRequiredMessage = "This project only runs sandboxed workers. Start the worker with Claude Code, or ask the person to allow unsandboxed workers in project settings."

// WorkerSandboxRequiredRestoreLead is prepended when restore is refused because
// the existing session is not sandboxed.
const WorkerSandboxRequiredRestoreLead = "This session is not sandboxed, so it cannot be restored."

// WorkerSandboxRequiredRestoreMessage is the 409 body when restore is refused
// on a host that can sandbox, because the existing session's harness would not
// actually be sandboxed.
const WorkerSandboxRequiredRestoreMessage = "This project only runs sandboxed workers. " + WorkerSandboxRequiredRestoreLead + " Start the worker with Claude Code, or ask the person to allow unsandboxed workers in project settings."

// WorkerSandboxRequiredSettingSentence is the actionable close on OS-unavailable
// refusals. The person turns the setting off; workers then have no sandbox.
const WorkerSandboxRequiredSettingSentence = `To use workers in this project, the person can turn off "Only allow sandboxed workers" in project settings. Workers will then be able to read any file and reach the internet.`

// WorkerSandboxRequiredWindowsMessage is the 409 body when the harness would
// qualify but Claude Code's OS sandbox does not run on native Windows.
const WorkerSandboxRequiredWindowsMessage = `This computer can't sandbox workers: Claude Code's sandbox does not run on Windows. ` + WorkerSandboxRequiredSettingSentence

// WorkerMeetsSandboxRequirement reports whether this launch is allowed when
// the project requires sandboxed workers. Non-workers always pass. osAvailable
// is whether the host can enforce the OS sandbox.
func WorkerMeetsSandboxRequirement(kind SessionKind, harness AgentHarness, sandbox WorkerSandboxConfig, osAvailable bool) bool {
	if kind != KindWorker || !sandbox.IsRequired() {
		return true
	}
	_, supported := SandboxedWorkerHarnesses[harness]
	return supported && sandbox.IsEnabled() && osAvailable
}

// RefuseUnsandboxedWorker returns ErrWorkerSandboxRequired when the launch
// would not actually run sandboxed and the project requires that. restore
// selects the restore wording. osUnavailableMessage is the OS-level reason
// used when the host cannot sandbox (Windows, missing bubblewrap/socat, or
// an old Claude Code); it is ignored when the host can sandbox.
func RefuseUnsandboxedWorker(kind SessionKind, harness AgentHarness, sandbox WorkerSandboxConfig, osAvailable, restore bool, osUnavailableMessage string) error {
	if WorkerMeetsSandboxRequirement(kind, harness, sandbox, osAvailable) {
		return nil
	}
	msg := workerSandboxRequiredUserMessage(osAvailable, restore, osUnavailableMessage)
	return fmt.Errorf("%w: %s", ErrWorkerSandboxRequired, msg)
}

func workerSandboxRequiredUserMessage(osAvailable, restore bool, osUnavailableMessage string) string {
	if !osAvailable {
		reason := osUnavailableMessage
		if reason == "" {
			reason = WorkerSandboxRequiredWindowsMessage
		}
		if restore {
			return WorkerSandboxRequiredRestoreLead + " " + reason
		}
		return reason
	}
	if restore {
		return WorkerSandboxRequiredRestoreMessage
	}
	return WorkerSandboxRequiredMessage
}

// WorkerSandboxRequiredAPIMessage is the 409 body extracted from a wrapped
// ErrWorkerSandboxRequired. Unknown wrappers fall back to the harness wording.
func WorkerSandboxRequiredAPIMessage(err error) string {
	if err == nil {
		return WorkerSandboxRequiredMessage
	}
	const marker = "worker sandbox required: "
	text := err.Error()
	if i := strings.LastIndex(text, marker); i >= 0 {
		if msg := text[i+len(marker):]; msg != "" {
			return msg
		}
	}
	return WorkerSandboxRequiredMessage
}

// WithDefaults returns c with Enabled set to true when it was unset, so callers
// that persist or display the effective policy see the default explicitly.
func (c WorkerSandboxConfig) WithDefaults() WorkerSandboxConfig {
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.AllowedDomains == nil {
		c.AllowedDomains = []string{}
	}
	return c
}

// Validate rejects domain allowlist entries that are not bare hostnames or a
// single leading "*." wildcard. Schemes, paths, ports, and spaces are refused.
func (c WorkerSandboxConfig) Validate() error {
	for i, raw := range c.AllowedDomains {
		if err := ValidateSandboxDomain(raw); err != nil {
			return fmt.Errorf("workerSandbox.allowedDomains[%d]: %w", i, err)
		}
	}
	if len(c.ProtectedPaths) > MaxWorkerProtectedPaths {
		return fmt.Errorf("workerSandbox.protectedPaths: at most %d entries", MaxWorkerProtectedPaths)
	}
	seen := make(map[string]bool, len(c.ProtectedPaths))
	for i, raw := range c.ProtectedPaths {
		if err := ValidateProtectedPath(raw); err != nil {
			return fmt.Errorf("workerSandbox.protectedPaths[%d]: %w", i, err)
		}
		if seen[raw] {
			return fmt.Errorf("workerSandbox.protectedPaths[%d]: %q is listed twice", i, raw)
		}
		seen[raw] = true
	}
	return nil
}

// ValidateForProject refuses absolute and home protected paths that would
// cover the project's .git folder. Shape checks are in Validate.
func (c WorkerSandboxConfig) ValidateForProject(projectPath, homeDir string) error {
	if strings.TrimSpace(projectPath) == "" {
		return nil
	}
	gitDir := filepath.Join(projectPath, ".git")
	for i, raw := range c.ProtectedPaths {
		kind, rest := ClassifyProtectedPath(raw)
		var abs string
		switch kind {
		case ProtectedPathAbsolute:
			abs = raw
		case ProtectedPathHome:
			if homeDir == "" {
				continue
			}
			abs = filepath.Join(homeDir, rest)
		default:
			continue
		}
		if PathCovers(abs, gitDir) {
			return fmt.Errorf("workerSandbox.protectedPaths[%d]: %q would block the project's .git folder, which workers need", i, raw)
		}
	}
	return nil
}

// PathCovers reports whether parent is child or one of its ancestors. On
// Windows filepath.Rel compares without regard to case, like the filesystem.
func PathCovers(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// ValidateSandboxDomain accepts a hostname or "*.example.com" wildcard only.
func ValidateSandboxDomain(raw string) error {
	domain := strings.TrimSpace(raw)
	if domain == "" || domain != raw {
		return fmt.Errorf("must be a hostname or *.example.com wildcard with no leading/trailing space")
	}
	if strings.ContainsAny(domain, `:/\#?@[] `) || strings.Contains(domain, "://") {
		return fmt.Errorf("%q: schemes, paths, ports, and userinfo are not allowed", domain)
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("%q: invalid hostname", domain)
	}

	rest := domain
	if strings.HasPrefix(domain, "*.") {
		rest = strings.TrimPrefix(domain, "*.")
		if rest == "" || strings.HasPrefix(rest, "*.") {
			return fmt.Errorf("%q: only a single leading *. wildcard is allowed", domain)
		}
	}
	if err := validateHostnameLabels(rest); err != nil {
		return fmt.Errorf("%q: %w", domain, err)
	}
	return nil
}

func validateHostnameLabels(host string) error {
	if host == "" {
		return fmt.Errorf("empty hostname")
	}
	// Reject IP literals: the project setting is a domain allowlist. Daemon
	// access uses excludedCommands for ao, not allowlisted IP:port entries.
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("IP addresses are not allowed; use a hostname")
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid hostname label")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("hostname labels must not start or end with '-'")
		}
		for _, r := range label {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				continue
			}
			return fmt.Errorf("hostname labels may contain only letters, digits, and '-'")
		}
	}
	return nil
}
