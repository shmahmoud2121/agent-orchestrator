package sessionmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
)

func trustHook(t *testing.T, root, cmd string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude/settings.json"), []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"`+cmd+`"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
}
func requireTrustError(t *testing.T, err error) {
	t.Helper()
	var api *apierr.Error
	if !errors.As(err, &api) || api.Code != "REPO_TRUST_REQUIRED" {
		t.Fatalf("want REPO_TRUST_REQUIRED: %v", err)
	}
}
func TestSpawnRepoTrustAndChangedConfig(t *testing.T) {
	m, st, _, ws := newManager()
	root := newManagerGitRepo(t)
	ws.path = root
	p := st.projects["mer"]
	p.Path = root
	st.projects[p.ID] = p
	// Config-free repositories keep their existing launch behavior.
	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker}); err != nil {
		t.Fatal(err)
	}
	trustHook(t, root, "touch marker")
	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker})
	requireTrustError(t, err)
	scan, err := repotrust.Detect(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	p.Config.TrustedRepoConfigFingerprint = scan.Fingerprint
	st.projects[p.ID] = p
	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker}); err != nil {
		t.Fatal(err)
	}
	trustHook(t, root, "touch changed")
	_, _, _, err = m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker})
	requireTrustError(t, err)
	_, _, _, err = m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindOrchestrator})
	requireTrustError(t, err)
}
func TestRefusedWorkspaceDoesNotTouchGlobalClaudeConfig(t *testing.T) {
	m, st, _, _ := newManager()
	base := newManagerGitRepo(t)
	worker := filepath.Join(t.TempDir(), "worker")
	runManagerGit(t, base, "worktree", "add", "-b", "evil", worker)
	trustHook(t, worker, "touch marker")
	runManagerGit(t, worker, "add", ".")
	runManagerGit(t, worker, "commit", "-m", "branch hook")
	p := st.projects["mer"]
	p.Path = base
	st.projects[p.ID] = p
	st.sessions["mer-1"] = domain.SessionRecord{ID: "mer-1", ProjectID: "mer"}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	configPath := filepath.Join(home, ".claude.json")
	seed := []byte(`{"userID":"untouched"}`)
	if err := os.WriteFile(configPath, seed, 0600); err != nil {
		t.Fatal(err)
	}
	_, _, _, spawnErr := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Branch: "evil"})
	requireTrustError(t, spawnErr)
	err := m.prepareWorkspace(context.Background(), claudecode.New(), "mer-1", "mer", worker, "", "", ports.AgentConfig{}, map[string]string{"HOME": home})
	requireTrustError(t, err)
	after, err := os.ReadFile(configPath)
	if err != nil || string(after) != string(seed) {
		t.Fatalf("global config changed: %s %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(worker, ".claude/settings.local.json")); !os.IsNotExist(err) {
		t.Fatalf("AO hooks installed before gate: %v", err)
	}
	// Matching approval permits the real pre-launch trust write and AO hooks.
	scan, err := repotrust.Detect(ctx, worker)
	if err != nil {
		t.Fatal(err)
	}
	p.Config.TrustedRepoConfigFingerprint = scan.Fingerprint
	st.projects[p.ID] = p
	if err := m.prepareWorkspace(ctx, claudecode.New(), "mer-1", "mer", worker, "", "", ports.AgentConfig{}, map[string]string{"HOME": home}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(configPath)
	if err != nil || string(after) == string(seed) {
		t.Fatalf("approved pre-launch not run: %s %v", after, err)
	}
}

func TestRestoreRepoTrustRecheckedBeforeLaunch(t *testing.T) {
	m, st, _, ws := newManager()
	root := newManagerGitRepo(t)
	trustHook(t, root, "touch original")
	scan, err := repotrust.Detect(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	p := st.projects["mer"]
	p.Path = root
	p.Config.TrustedRepoConfigFingerprint = scan.Fingerprint
	st.projects[p.ID] = p
	ws.path = root
	st.sessions["mer-1"] = domain.SessionRecord{ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode, IsTerminated: true, Metadata: domain.SessionMetadata{WorkspacePath: root, Branch: "main", Prompt: "continue"}}
	trustHook(t, root, "touch changed")
	_, err = m.RestoreWithMode(ctx, "mer-1")
	requireTrustError(t, err)
	if !st.sessions["mer-1"].IsTerminated {
		t.Fatal("refused restore resurrected session")
	}
}

func TestSpawnRepoTrustScansConfiguredBaseForNewBranch(t *testing.T) {
	m, st, _, ws := newManager()
	base := newManagerGitRepo(t)
	release := filepath.Join(t.TempDir(), "release")
	runManagerGit(t, base, "worktree", "add", "-b", "release", release)
	trustHook(t, release, "touch release-marker")
	runManagerGit(t, release, "add", ".")
	runManagerGit(t, release, "commit", "-m", "release hook")
	p := st.projects["mer"]
	p.Path = base
	p.Config.DefaultBranch = "release"
	st.projects[p.ID] = p
	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Branch: "new-worker"})
	requireTrustError(t, err)
	if ws.createCount != 0 {
		t.Fatal("created workspace before trust preflight")
	}
}
