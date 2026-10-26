package claudecode

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// Launch-time scan limits. The scan only finds names glob rules cannot express
// (".env.*" without its samples, and nested secret folders), so a shallow,
// bounded walk is enough and keeps launch fast on large checkouts.
const (
	protectedScanMaxDepth   = 8
	protectedScanMaxEntries = 20000
)

// readDeny is one protected path in OS form. sandboxOnly marks the exact twin
// of a "/**/" glob: Read(...) rules use gitignore matching, where "/**/" also
// matches zero folders, so the twin would only lengthen the settings.
type readDeny struct {
	path        string
	sandboxOnly bool
}

// workerReadDenyPaths expands the built-in list and the project's additions
// into the paths a sandboxed worker may not read.
//
// Claude Code accepts globs in sandbox.filesystem.denyRead and Read(...) rules
// (https://code.claude.com/docs/en/settings-reference#sandbox-path-prefixes,
// https://code.claude.com/docs/en/permissions#how-path-patterns-match), so files
// matching a glob entry stay protected when created after launch. Files found
// only by the launch scan (an unusual ".env.*" name, a nested secrets/ folder)
// are covered only if they existed at launch; the list is rebuilt on every
// launch and restore.
//
// Deep folder names are found by the scan rather than a "/**/secrets" glob so a
// branch named like backup/x (stored under .git/refs) stays readable to git.
func workerReadDenyPaths(sandbox domain.WorkerSandboxConfig) []readDeny {
	launch := sandbox.Launch
	builtin := domain.BuiltinWorkerProtectedPaths()
	mainDir := cleanOrEmpty(launch.ProjectPath)
	worktree := cleanOrEmpty(launch.WorkspacePath)
	home := cleanOrEmpty(launch.HomeDir)
	if home == "" {
		if dir, err := os.UserHomeDir(); err == nil {
			home = filepath.Clean(dir)
		}
	}

	var out []readDeny
	// A worker running in the main checkout itself gets the worktree rules:
	// its scratch databases are its own.
	sameDir := mainDir != "" && worktree != "" && domain.PathCovers(mainDir, worktree) && domain.PathCovers(worktree, mainDir)
	if mainDir != "" && !sameDir {
		out = append(out, rootEntryDenies(mainDir, builtin.MainCheckout)...)
		out = append(out, scanProtected(mainDir, folderNames(builtin.MainCheckout))...)
	}
	if worktree != "" {
		out = append(out, rootEntryDenies(worktree, builtin.Worktree)...)
		out = append(out, scanProtected(worktree, nil)...)
	}
	if home != "" {
		for _, entry := range builtin.Home {
			out = append(out, readDeny{path: filepath.Join(home, filepath.FromSlash(entry))})
		}
	}
	if token := cleanOrEmpty(launch.APITokenPath); token != "" {
		out = append(out, readDeny{path: token})
	}
	for _, raw := range sandbox.ProtectedPaths {
		switch kind, rest := domain.ClassifyProtectedPath(raw); kind {
		case domain.ProtectedPathHome:
			if home != "" {
				out = append(out, readDeny{path: filepath.Join(home, filepath.FromSlash(rest))})
			}
		case domain.ProtectedPathAbsolute:
			out = append(out, readDeny{path: filepath.Clean(raw)})
		default:
			if mainDir != "" {
				out = append(out, readDeny{path: filepath.Join(mainDir, filepath.FromSlash(raw))})
			}
		}
	}
	return keepWorkable(out, mainDir, worktree)
}

// rootEntryDenies turns built-in entries into rules under root. Folder entries
// ("secrets/") cover only root's own folder here; scanProtected adds deeper
// ones.
func rootEntryDenies(root string, entries []string) []readDeny {
	var out []readDeny
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry, "/"); ok {
			out = append(out, readDeny{path: filepath.Join(root, name)})
			continue
		}
		names := []string{entry}
		if entry == ".env.*" {
			names = domain.CommonEnvFileNames
		}
		for _, name := range names {
			out = append(out,
				readDeny{path: filepath.Join(root, name), sandboxOnly: true},
				readDeny{path: filepath.Join(root, "**", name)},
			)
		}
	}
	return out
}

func folderNames(entries []string) map[string]bool {
	names := map[string]bool{}
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry, "/"); ok {
			names[strings.ToLower(name)] = true
		}
	}
	return names
}

// scanProtected walks root for ".env.*" files that are not samples and, when
// folders is set, for folders with those names. It skips .git and
// node_modules and stops at the depth and entry caps.
func scanProtected(root string, folders map[string]bool) []readDeny {
	var out []readDeny
	seen := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		seen++
		if seen > protectedScanMaxEntries {
			return fs.SkipAll
		}
		name := entry.Name()
		if entry.IsDir() {
			lower := strings.ToLower(name)
			if lower == ".git" || lower == "node_modules" {
				return fs.SkipDir
			}
			if folders[lower] {
				out = append(out, readDeny{path: path})
				return fs.SkipDir
			}
			if rel, relErr := filepath.Rel(root, path); relErr == nil && strings.Count(rel, string(filepath.Separator))+1 >= protectedScanMaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(strings.ToLower(name), ".env.") && !domain.IsEnvTemplateFile(name) {
			out = append(out, readDeny{path: path})
		}
		return nil
	})
	return out
}

// keepWorkable drops entries that would block the main checkout's .git or the
// worker's worktree, and removes duplicates. A project entry cannot reach here
// with such a path after validation, but an absolute or home entry checked
// against another machine's paths could.
func keepWorkable(entries []readDeny, mainDir, worktree string) []readDeny {
	gitDir := ""
	if mainDir != "" {
		gitDir = filepath.Join(mainDir, ".git")
	}
	seen := make(map[string]int, len(entries))
	out := make([]readDeny, 0, len(entries))
	for _, entry := range entries {
		if !strings.ContainsAny(entry.path, "*?[") {
			if (gitDir != "" && domain.PathCovers(entry.path, gitDir)) || (worktree != "" && domain.PathCovers(entry.path, worktree)) {
				continue
			}
		}
		key := entry.path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if i, dup := seen[key]; dup {
			out[i].sandboxOnly = out[i].sandboxOnly && entry.sandboxOnly
			continue
		}
		seen[key] = len(out)
		out = append(out, entry)
	}
	return out
}

func cleanOrEmpty(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	return filepath.Clean(p)
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
