package process

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafeGitDir reports a checkout whose .git file points Git at a
// repository inside the checkout itself.
var ErrUnsafeGitDir = errors.New("refusing to run git: .git file points inside the worktree")

// gitSafetyArgs disable repository-config settings that make a read-only Git
// call execute a program (fsmonitor, external diff). Command-line -c beats
// every config file. Hooks stay enabled so the user's own commit hooks still
// run; a planted hooksPath needs a planted config, which checkGitLink refuses.
func gitSafetyArgs() []string {
	return []string{
		"-c", "core.fsmonitor=false",
		"-c", "diff.external=",
	}
}

func isGit(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	return base == "git" || base == "git.exe"
}

// hardenGit prepends the safety flags and validates the worktree a `-C <dir>`
// call targets. The returned error, when set, is stored on the command so
// Start/Run/Output fail without launching git.
func hardenGit(args []string) ([]string, error) {
	hardened := append(gitSafetyArgs(), args...)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-C" {
			if err := checkGitLink(args[i+1]); err != nil {
				return hardened, err
			}
		}
	}
	return hardened, nil
}

// checkGitLink accepts a directory without a .git file (a main checkout, a
// bare path, or a path that does not exist yet). A .git file (linked worktree,
// submodule, --separate-git-dir) must point at an existing directory outside
// the checkout. Agents edit files inside their worktree; a .git file swapped
// for "gitdir: ./planted" would hand Git a config they wrote, whose
// fsmonitor/filter/hooks settings then run on the daemon's next git call.
func checkGitLink(dir string) error {
	link := filepath.Join(dir, ".git")
	info, err := os.Lstat(link)
	if err != nil || info.IsDir() {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafeGitDir
	}
	raw, err := os.ReadFile(link)
	if err != nil {
		return ErrUnsafeGitDir
	}
	text := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(text, "gitdir: ") {
		return ErrUnsafeGitDir
	}
	target := strings.TrimSpace(strings.TrimPrefix(text, "gitdir: "))
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return ErrUnsafeGitDir
	}
	checkout, err := filepath.EvalSymlinks(dir)
	if err != nil || within(target, checkout) {
		return ErrUnsafeGitDir
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return ErrUnsafeGitDir
	}
	return nil
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
