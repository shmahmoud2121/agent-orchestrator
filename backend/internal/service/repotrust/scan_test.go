package repotrust_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cursor"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/devin"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/gemini"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/mimocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/vibe"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
)

var ctx = context.Background()

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s %v", args, out, err)
	}
	return string(out)
}
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "-c", "user.name=AO", "-c", "user.email=ao@example.com", "commit", "--allow-empty", "-m", "initial")
	return root
}
func write(t *testing.T, root, path, data string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func scan(t *testing.T, root string) repotrust.Scan {
	t.Helper()
	s, err := repotrust.Detect(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestDetectRepositoryConfigTypes(t *testing.T) {
	for _, path := range []string{"mimocode.json", "mimocode.jsonc", "opencode.json", "opencode.jsonc", ".zed/settings.json", ".kimi-code/mcp.json", ".mimocode/mimocode.json", ".mimocode/mimocode.jsonc", ".mimocode/plugin/local.ts", ".claude/settings.json", ".claude/settings.local.json", ".mcp.json", ".codex/config.toml", ".codex/hooks.json", ".codex/nested/tool.sh", ".gemini/settings.json", ".cursor/hooks.json", ".cursor/mcp.json", ".github/hooks/user.json", ".copilot/mcp-config.json", ".kimi.toml", ".kimi/config.toml", ".kimi/mcp.json", ".muse/settings.json", ".mimocode/config.json", ".mimocode/hooks/user.ts", ".vibe/config.toml", ".vibe/hooks.toml", ".commandcode/settings.json", ".devin/config.json"} {
		t.Run(path, func(t *testing.T) {
			root := repo(t)
			write(t, root, path, `{"command":"touch marker"}`)
			git(t, root, "add", "-f", path)
			s := scan(t, root)
			if len(s.Files) != 1 || s.Files[0].Path != path || len(s.Fingerprint) != 64 || !strings.Contains(strings.Join(s.Files[0].Summary, " "), "touch marker") {
				t.Fatalf("scan: %+v", s)
			}
			if scan(t, root).Fingerprint != s.Fingerprint {
				t.Fatal("unstable fingerprint")
			}
			write(t, root, path, `{"command":"changed"}`)
			if scan(t, root).Fingerprint == s.Fingerprint {
				t.Fatal("edited config retained fingerprint")
			}
		})
	}
}
func TestOwnHooksIgnoredExactly(t *testing.T) {
	for name, agent := range map[string]ports.Agent{"claude": claudecode.New(), "gemini": gemini.New(), "cursor": cursor.New(), "copilot": copilot.New(), "vibe": vibe.New(), "mimo": mimocode.New(), "command": commandcode.New(), "devin": devin.New()} {
		t.Run(name, func(t *testing.T) {
			root := repo(t)
			if err := agent.GetAgentHooks(ctx, ports.WorkspaceHookConfig{SessionID: "p-1", WorkspacePath: root, DataDir: t.TempDir()}); err != nil {
				t.Fatal(err)
			}
			if s := scan(t, root); len(s.Files) != 0 {
				t.Fatalf("AO own hooks detected: %+v", s)
			}
		})
	}
	root := repo(t)
	write(t, root, ".claude/settings.local.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"ao hooks claude-code session-start; touch marker","timeout":30}]}]}}`)
	if len(scan(t, root).Files) != 1 {
		t.Fatal("command prefix allowed foreign shell code")
	}
}
func TestManagedMergeDoesNotChangeApproval(t *testing.T) {
	root := repo(t)
	write(t, root, ".claude/settings.local.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"touch user-marker"}]}]},"statusLine":{"type":"command","command":"echo user"}}`)
	before := scan(t, root)
	if err := claudecode.New().GetAgentHooks(ctx, ports.WorkspaceHookConfig{WorkspacePath: root}); err != nil {
		t.Fatal(err)
	}
	if after := scan(t, root); after.Fingerprint != before.Fingerprint {
		t.Fatalf("AO injection changed approval: before=%+v after=%+v", before, after)
	}
}
func TestGateAndBranch(t *testing.T) {
	base := repo(t)
	if err := repotrust.Require(ctx, "p", base, ""); err != nil {
		t.Fatal(err)
	}
	write(t, base, ".mcp.json", `{"mcpServers":{"evil":{"command":"touch marker"}}}`)
	s := scan(t, base)
	var api *apierr.Error
	err := repotrust.Require(ctx, "p", base, "")
	if !errors.As(err, &api) || api.Code != "REPO_TRUST_REQUIRED" {
		t.Fatalf("want trust required: %v", err)
	}
	t.Log(api.Message)
	if err := repotrust.Require(ctx, "p", base, s.Fingerprint); err != nil {
		t.Fatal(err)
	}
	write(t, base, ".mcp.json", `{"command":"changed"}`)
	if err := repotrust.Require(ctx, "p", base, s.Fingerprint); err == nil {
		t.Fatal("changed hook allowed")
	}
	git(t, base, "add", ".")
	git(t, base, "-c", "user.name=AO", "-c", "user.email=ao@example.com", "commit", "-m", "config")
	worker := filepath.Join(t.TempDir(), "worker")
	git(t, base, "worktree", "add", "-b", "worker", worker)
	write(t, worker, ".codex/hooks.json", `{"command":"branch hook"}`)
	approved := scan(t, base).Fingerprint
	if err := repotrust.Check(ctx, "p", base, worker, approved); err == nil {
		t.Fatal("worker branch additions allowed")
	}
	git(t, worker, "add", ".")
	git(t, worker, "-c", "user.name=AO", "-c", "user.email=ao@example.com", "commit", "-m", "worker hooks")
	fromRef, err := repotrust.DetectBranch(ctx, base, "worker")
	if err != nil {
		t.Fatal(err)
	}
	combined := repotrust.Combine(scan(t, base), fromRef)
	if err := repotrust.Check(ctx, "p", base, worker, combined.Fingerprint); err != nil {
		t.Fatal(err)
	}
}
func TestIgnoredConfigAndHarmlessSettings(t *testing.T) {
	root := repo(t)
	write(t, root, ".gitignore", ".mcp.json\n.claude/\n")
	write(t, root, ".claude/settings.json", `{"permissions":{"defaultMode":"default"}}`)
	if s := scan(t, root); len(s.Files) != 0 {
		t.Fatalf("harmless settings: %+v", s)
	}
	write(t, root, ".mcp.json", `{"command":"hidden"}`)
	if len(scan(t, root).Files) != 1 {
		t.Fatal("gitignore concealed config")
	}
}

func TestConfigurationSymlinksCannotReadOutsideCheckout(t *testing.T) {
	for _, dir := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[dir], func(t *testing.T) {
			root := repo(t)
			outside := t.TempDir()
			write(t, outside, "private.json", "SECRET_NOT_REPOSITORY_CONFIG")
			target, link := filepath.Join(outside, "private.json"), filepath.Join(root, ".mcp.json")
			if dir {
				target = outside
				link = filepath.Join(root, ".codex")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			result, err := repotrust.Detect(ctx, root)
			if err == nil {
				t.Fatalf("outside-checkout symlink accepted: %+v", result)
			}
			for _, f := range result.Files {
				if strings.Contains(strings.Join(f.Summary, " "), "SECRET_NOT_REPOSITORY_CONFIG") {
					t.Fatal("outside file contents disclosed")
				}
			}
		})
	}
}
