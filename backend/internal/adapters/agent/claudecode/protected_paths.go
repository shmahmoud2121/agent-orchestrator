package claudecode

import (
	"path/filepath"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/watchdog"
)

// readDeny is one protected path in OS form. sandboxOnly marks the exact twin
// of a "/**/" glob: Read(...) rules use gitignore matching, where "/**/" also
// matches zero folders, so the twin would only lengthen the settings.
type readDeny struct {
	path        string
	sandboxOnly bool
}

// workerReadDenyPaths expands the built-in list and the project's additions
// into the paths a sandboxed worker may not read. The list itself is built by
// the shared watchdog package so the hook and the sandbox settings cannot drift.
func workerReadDenyPaths(sandbox domain.WorkerSandboxConfig) []readDeny {
	src := watchdog.SessionReadDenyPaths(sandbox)
	out := make([]readDeny, len(src))
	for i, entry := range src {
		out[i] = readDeny{path: entry.Path, sandboxOnly: entry.SandboxOnly}
	}
	return out
}

// claudePath writes an OS path the way Claude Code matches it: forward
// slashes, and on Windows the drive as a leading folder (C:\Users\a becomes
// /c/Users/a), per https://code.claude.com/docs/en/permissions#how-path-patterns-match.
func claudePath(p, goos string) string {
	if goos != "windows" {
		return filepath.ToSlash(p)
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	if len(slashed) >= 2 && slashed[1] == ':' {
		return "/" + strings.ToLower(slashed[:1]) + slashed[2:]
	}
	return slashed
}

// readDenySettings renders the protected paths for both places Claude Code
// checks reads: sandbox.filesystem.denyRead for shell commands, which uses
// plain absolute paths, and permissions.deny Read(...) rules for the Read,
// Grep and Glob tools, which use "//" for an absolute path. Claude Code applies
// Read deny rules to Grep and Glob; it never matches Glob(path) rules
// (https://code.claude.com/docs/en/permissions).
func readDenySettings(entries []readDeny, goos string) (denyRead, readRules []string) {
	denyRead = make([]string, 0, len(entries))
	readRules = make([]string, 0, len(entries))
	for _, entry := range entries {
		p := claudePath(entry.path, goos)
		denyRead = append(denyRead, p)
		if !entry.sandboxOnly {
			readRules = append(readRules, "Read(/"+p+")")
		}
	}
	return denyRead, readRules
}
