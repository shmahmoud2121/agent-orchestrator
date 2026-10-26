// Package watchdog decides whether an agent tool call touches a protected file.
// The list builder is shared with Claude Code's sandbox settings so both checks
// cover the same files. The daemon is the only caller that applies the list to
// a live tool call; the list never goes into the worktree or the agent environment.
package watchdog

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

// ReadDeny is one protected path in OS form. SandboxOnly marks the exact twin
// of a "/**/" glob: Claude Code Read(...) rules use gitignore matching, where
// "/**/" also matches zero folders, so the twin would only lengthen the settings.
type ReadDeny struct {
	Path        string
	SandboxOnly bool
}

// SessionReadDenyPaths expands the built-in list and the project's additions
// into the paths a session may not read.
//
// Claude Code accepts globs in sandbox.filesystem.denyRead and Read(...) rules,
// so files matching a glob entry stay protected when created after launch.
// Files found only by the launch scan (an unusual ".env.*" name, a nested
// secrets/ folder) are covered only if they existed at launch; the list is
// rebuilt on every launch and restore.
//
// Deep folder names are found by the scan rather than a "/**/secrets" glob so a
// branch named like backup/x (stored under .git/refs) stays readable to git.
func SessionReadDenyPaths(sandbox domain.WorkerSandboxConfig) []ReadDeny {
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

	var out []ReadDeny
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
			out = append(out, ReadDeny{Path: filepath.Join(home, filepath.FromSlash(entry))})
		}
	}
	if token := cleanOrEmpty(launch.APITokenPath); token != "" {
		out = append(out, ReadDeny{Path: token})
	}
	for _, raw := range sandbox.ProtectedPaths {
		switch kind, rest := domain.ClassifyProtectedPath(raw); kind {
		case domain.ProtectedPathHome:
			if home != "" {
				out = append(out, ReadDeny{Path: filepath.Join(home, filepath.FromSlash(rest))})
			}
		case domain.ProtectedPathAbsolute:
			out = append(out, ReadDeny{Path: filepath.Clean(raw)})
		default:
			if mainDir != "" {
				out = append(out, ReadDeny{Path: filepath.Join(mainDir, filepath.FromSlash(raw))})
			}
		}
	}
	return keepWorkable(out, mainDir, worktree)
}

// rootEntryDenies turns built-in entries into rules under root. Folder entries
// ("secrets/") cover only root's own folder here; scanProtected adds deeper
// ones.
func rootEntryDenies(root string, entries []string) []ReadDeny {
	var out []ReadDeny
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry, "/"); ok {
			out = append(out, ReadDeny{Path: filepath.Join(root, name)})
			continue
		}
		names := []string{entry}
		if entry == ".env.*" {
			names = domain.CommonEnvFileNames
		}
		for _, name := range names {
			out = append(out,
				ReadDeny{Path: filepath.Join(root, name), SandboxOnly: true},
				ReadDeny{Path: filepath.Join(root, "**", name)},
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
func scanProtected(root string, folders map[string]bool) []ReadDeny {
	var out []ReadDeny
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
				out = append(out, ReadDeny{Path: path})
				return fs.SkipDir
			}
			if rel, relErr := filepath.Rel(root, path); relErr == nil && strings.Count(rel, string(filepath.Separator))+1 >= protectedScanMaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(strings.ToLower(name), ".env.") && !domain.IsEnvTemplateFile(name) {
			out = append(out, ReadDeny{Path: path})
		}
		return nil
	})
	return out
}

// keepWorkable drops entries that would block the main checkout's .git or the
// worker's worktree, and removes duplicates. A project entry cannot reach here
// with such a path after validation, but an absolute or home entry checked
// against another machine's paths could.
func keepWorkable(entries []ReadDeny, mainDir, worktree string) []ReadDeny {
	gitDir := ""
	if mainDir != "" {
		gitDir = filepath.Join(mainDir, ".git")
	}
	seen := make(map[string]int, len(entries))
	out := make([]ReadDeny, 0, len(entries))
	for _, entry := range entries {
		if !strings.ContainsAny(entry.Path, "*?[") {
			if (gitDir != "" && domain.PathCovers(entry.Path, gitDir)) || (worktree != "" && domain.PathCovers(entry.Path, worktree)) {
				continue
			}
		}
		key := entry.Path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if i, dup := seen[key]; dup {
			out[i].SandboxOnly = out[i].SandboxOnly && entry.SandboxOnly
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
