package domain

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// WorkerProtectedPaths lists files a sandboxed worker may not read, grouped by
// the folder each entry is relative to. An entry is one of:
//
//   - a file name or glob such as ".env" or "*.pem": it matches at any depth;
//   - a directory name ending in "/" such as "secrets/": the whole directory;
//   - for Home, a path relative to the home folder such as ".config/gh".
//
// ".env.*" is special: it never covers the sample files named in
// IsEnvTemplateFile, because Claude Code's "!" negation cannot carve an
// exception out of an absolute deny rule.
type WorkerProtectedPaths struct {
	// MainCheckout is relative to the project's registered path (the person's
	// own checkout), never the worker's worktree.
	MainCheckout []string
	// Worktree is relative to the worker's own worktree.
	Worktree []string
	// Home is relative to the real home folder on every OS.
	Home []string
}

// BuiltinWorkerProtectedPaths is the list AO applies to every sandboxed worker
// with no setup. Projects add to it with workerSandbox.protectedPaths; nothing
// removes an entry here. AO's own API token file is protected too, but its
// location depends on the data dir, so the launch adds it separately.
//
// Two things are never protected, because workers cannot run without them:
// the main checkout's .git directory, which every linked worktree reads, and
// the worker's own worktree as a whole.
func BuiltinWorkerProtectedPaths() WorkerProtectedPaths {
	// Passwords and API keys in dotenv files, and private keys and
	// certificates. These apply to the worktree too: a worktree can hold a copy
	// or a symlink of the person's .env.
	secrets := []string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx"}
	return WorkerProtectedPaths{
		MainCheckout: append(append([]string(nil), secrets...),
			// Folders people keep secrets and copies of real data in.
			"secrets/", "backups/", "backup/",
			// Local databases and dumps hold real customer data. Only the main
			// checkout: tests may create scratch databases in the worktree.
			"*.sqlite", "*.sqlite3", "*.db", "*.dump", "*.bak",
			// Cloud service credentials and Terraform state, which stores
			// secrets in plain text.
			"credentials.json", "service-account*.json", "*.tfstate", "*.tfstate.backup",
		),
		Worktree: secrets,
		Home: []string{
			// SSH keys and cloud CLI logins.
			".ssh", ".aws", ".azure", ".config/gcloud", ".kube",
			// Registry, package-index and git host tokens.
			".docker/config.json", ".npmrc", ".pypirc", ".netrc", ".git-credentials", ".config/gh",
		},
	}
}

// CommonEnvFileNames are the usual ".env.*" names. Read rules name them
// directly so they stay protected when created after a worker starts; other
// ".env.*" files are found by a scan at launch.
var CommonEnvFileNames = []string{
	".env.local", ".env.*.local", ".env.development", ".env.dev",
	".env.production", ".env.prod", ".env.staging", ".env.test",
}

// IsEnvTemplateFile reports whether name is a dotenv sample that holds
// placeholder values and stays readable: .env.example, .env.sample and
// .env.template.
func IsEnvTemplateFile(name string) bool {
	switch strings.ToLower(name) {
	case ".env.example", ".env.sample", ".env.template":
		return true
	}
	return false
}

// MaxWorkerProtectedPaths and MaxWorkerProtectedPathLen bound
// workerSandbox.protectedPaths.
const (
	MaxWorkerProtectedPaths   = 100
	MaxWorkerProtectedPathLen = 512
)

// ValidateProtectedPath accepts one workerSandbox.protectedPaths entry: a path
// relative to the project, a "~/" path, or an absolute path. Globs are
// allowed. It refuses entries that would block the main checkout's .git, the
// whole project, or the whole home folder (which holds the worktrees).
func ValidateProtectedPath(raw string) error {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) != raw {
		return fmt.Errorf("must not be empty or start or end with a space")
	}
	if len(raw) > MaxWorkerProtectedPathLen {
		return fmt.Errorf("must be %d characters or fewer", MaxWorkerProtectedPathLen)
	}
	if strings.ContainsAny(raw, "\x00\r\n") {
		return fmt.Errorf("%q: must be one line", raw)
	}
	if strings.HasPrefix(raw, "!") {
		return fmt.Errorf("%q: exceptions starting with ! are not supported", raw)
	}
	switch kind, rest := ClassifyProtectedPath(raw); kind {
	case ProtectedPathHome:
		clean := path.Clean(filepath.ToSlash(rest))
		if clean == "." || clean == "/" {
			return fmt.Errorf("%q: would block the whole home folder, including the workers' own worktrees", raw)
		}
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%q: must stay inside the home folder; use an absolute path instead", raw)
		}
	case ProtectedPathAbsolute:
		clean := filepath.Clean(raw)
		if clean == filepath.VolumeName(clean)+string(filepath.Separator) || path.Clean(filepath.ToSlash(raw)) == "/" {
			return fmt.Errorf("%q: would block the whole disk", raw)
		}
	default:
		clean := path.Clean(filepath.ToSlash(raw))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%q: must stay inside the project", raw)
		}
		segments := strings.Split(clean, "/")
		if strings.EqualFold(segments[0], ".git") {
			return fmt.Errorf("%q: the project's .git folder cannot be protected; workers need it", raw)
		}
		if clean == "." || allWildcards(segments) {
			return fmt.Errorf("%q: would block the whole project, including its .git folder", raw)
		}
	}
	return nil
}

// ProtectedPathKind says what a protectedPaths entry is relative to.
type ProtectedPathKind int

const (
	// ProtectedPathProject is relative to the project's main checkout.
	ProtectedPathProject ProtectedPathKind = iota
	// ProtectedPathHome starts with "~/".
	ProtectedPathHome
	// ProtectedPathAbsolute is an absolute path on this OS.
	ProtectedPathAbsolute
)

// ClassifyProtectedPath reports what raw is relative to. For a home entry,
// rest is the part after "~/".
func ClassifyProtectedPath(raw string) (kind ProtectedPathKind, rest string) {
	switch {
	case raw == "~":
		return ProtectedPathHome, ""
	case strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, `~\`):
		return ProtectedPathHome, raw[2:]
	case filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`):
		return ProtectedPathAbsolute, raw
	}
	return ProtectedPathProject, raw
}

func allWildcards(segments []string) bool {
	for _, segment := range segments {
		if strings.Trim(segment, "*") != "" {
			return false
		}
	}
	return true
}
