package trustrepro_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/codex"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// TestRepositoryHookPreTrustEvidence exercises adapters only, without launching
// a provider. The manager must gate calls to these intentionally unattended APIs.
func TestRepositoryHookPreTrustEvidence(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "HOOK_EXECUTED")
	data, err := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "touch " + marker}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".claude/settings.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := ports.LaunchConfig{WorkspacePath: repo, Env: map[string]string{"HOME": home}}
	if err := claudecode.New().PreLaunch(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var global struct {
		Projects map[string]struct {
			Accepted bool `json:"hasTrustDialogAccepted"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(raw, &global); err != nil {
		t.Fatal(err)
	}
	if !global.Projects[repo].Accepted {
		t.Fatalf("workspace not pre-trusted: %s", raw)
	}
	cmd, err := codex.New().GetLaunchCommand(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	flags := strings.Join(cmd, " ")
	if !strings.Contains(flags, `trust_level="trusted"`) || !strings.Contains(flags, "--dangerously-bypass-hook-trust") {
		t.Fatalf("missing overrides: %s", flags)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker unexpectedly exists or cannot be inspected: %v", err)
	}
	t.Logf("PROVED: SessionStart touch hook present; temp HOME .claude.json projects[worktree].hasTrustDialogAccepted=true; Codex argv includes trust_level=trusted and --dangerously-bypass-hook-trust")
	t.Log("NOT PROVED: provider hook execution; no real Claude/Codex launched and no marker created")
}
