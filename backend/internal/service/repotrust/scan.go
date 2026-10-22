// Package repotrust inventories repository configuration before unattended agents
// are allowed to load it. It never reads or changes user-level agent settings.
package repotrust

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// File describes an executable configuration source for human review.
type File struct {
	Path    string   `json:"path"`
	Summary []string `json:"summary"`
}

// Scan is an exact approval candidate. Empty inventories need no approval.
type Scan struct {
	contents    map[string][]byte
	Fingerprint string `json:"fingerprint"`
	Files       []File `json:"files"`
	Approved    bool   `json:"approved"`
}

// Detect scans repository configuration, including ignored files that agent
// loaders will still read. Only exact AO-owned hook entries are removed.
func Detect(ctx context.Context, root string) (Scan, error) {
	out := Scan{Files: []File{}}
	if root == "" {
		return out, nil
	}
	checkout, err := os.OpenRoot(root)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer func() { _ = checkout.Close() }()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return out, err
	}
	paths := map[string]bool{}
	// Agent loaders do not honor .gitignore. Inspect ignored config too; only
	// exact AO-owned content is removed below, never a name or command prefix.
	for _, dir := range []string{".kimi-code", ".zed", ".commandcode", ".devin", ".claude", ".codex", ".gemini", ".cursor", ".github/hooks", ".copilot", ".muse", ".mimocode", ".vibe", ".kimi"} {
		walkRoot := filepath.Join(root, filepath.FromSlash(dir))
		if _, err := os.Lstat(walkRoot); os.IsNotExist(err) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(walkRoot)
		if err != nil {
			return out, err
		}
		rel, err := filepath.Rel(canonicalRoot, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return out, fmt.Errorf("repository configuration directory escapes checkout: %s", dir)
		}
		err = filepath.WalkDir(resolved, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(resolved, path)
			if err != nil {
				return err
			}
			p := dir
			if rel != "." {
				p += "/" + filepath.ToSlash(rel)
			}
			if d.Type()&os.ModeSymlink != 0 {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if info.IsDir() {
					return fmt.Errorf("repository configuration contains directory symlink: %s", p)
				}
			}
			if !d.IsDir() && candidate(p) {
				paths[p] = true
			}
			return nil
		})
		if err != nil {
			return out, err
		}
	}
	for _, p := range []string{".mcp.json", "mcp.json", ".kimi.toml", "mimocode.json", "mimocode.jsonc", "opencode.json", "opencode.jsonc"} {
		if _, err := os.Lstat(filepath.Join(root, p)); err == nil {
			paths[p] = true
		}
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	contents := map[string][]byte{}
	for _, p := range sorted {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		data, err := checkout.ReadFile(filepath.FromSlash(p))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return out, fmt.Errorf("read repository configuration %s: %w", p, err)
		}
		data, ignore := repositoryContent(p, data)
		if ignore {
			continue
		}
		contents[p] = data
	}
	out = inventory(contents)
	return out, nil
}

func candidate(p string) bool {
	switch p {
	case "mimocode.json", "mimocode.jsonc", "opencode.json", "opencode.jsonc", ".zed/settings.json", ".mimocode/mimocode.json", ".mimocode/mimocode.jsonc", ".claude/mcp_servers.json", ".claude/settings.json", ".claude/settings.local.json", ".mcp.json", ".gemini/settings.json", ".cursor/hooks.json", ".cursor/mcp.json", ".kimi.toml", ".kimi/config.toml", ".kimi/mcp.json", "mcp.json", ".mimocode/config.json", ".mimocode/config.toml":
		return true
	}
	for _, dir := range []string{".kimi-code/", ".claude/hooks/", ".gemini/hooks/", ".mimocode/plugin/", ".mimocode/plugins/", ".mimocode/tool/", ".mimocode/tools/", ".commandcode/", ".devin/", ".codex/", ".github/hooks/", ".copilot/", ".muse/", ".mimocode/hooks/", ".vibe/", ".kimi/"} {
		if strings.HasPrefix(p, dir) && filepath.Base(p) != ".gitignore" {
			return true
		}
	}
	return false
}
func summarize(data []byte) []string {
	var v any
	if json.Unmarshal(data, &v) != nil {
		lines := []string{}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				lines = append(lines, short(line))
			}
		}
		if len(lines) == 0 {
			lines = append(lines, "Agent configuration (review file contents)")
		}
		return lines
	}
	lines := []string{}
	var walk func(any, string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k], strings.TrimPrefix(path+"."+k, "."))
			}
		case []any:
			for i, y := range x {
				walk(y, fmt.Sprintf("%s[%d]", path, i))
			}
		default:
			lines = append(lines, short(fmt.Sprintf("%s: %v", path, v)))
		}
	}
	walk(v, "")
	if len(lines) == 0 {
		lines = append(lines, "Agent configuration (review file contents)")
	}
	return lines
}
func short(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '\uFFFD'
		}
		return r
	}, s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// Require is deliberately shared by all harnesses, not selected by agent name.
func Require(ctx context.Context, projectID, root, approved string) error {
	scan, err := Detect(ctx, root)
	if err != nil {
		return err
	}
	if scan.Fingerprint == "" || scan.Fingerprint == approved {
		return nil
	}
	return Required(projectID, scan)
}

// Required carries the approval candidate in the standard API error envelope.
func Required(projectID string, scan Scan) error {
	paths := make([]string, 0, len(scan.Files))
	for _, f := range scan.Files {
		paths = append(paths, short(f.Path)+": "+strings.Join(f.Summary, "; "))
	}
	return apierr.Conflict("REPO_TRUST_REQUIRED", fmt.Sprintf("Repository configuration requires approval for project %s (fingerprint %s): %s. Review these files, then run `ao project trust %s` (add `--branch <worker-branch>` for branch changes) or POST /api/v1/projects/%s/repo-trust with this fingerprint and optional branch.", projectID, scan.Fingerprint, strings.Join(paths, "; "), projectID, projectID), map[string]any{"fingerprint": scan.Fingerprint, "files": scan.Files})
}
