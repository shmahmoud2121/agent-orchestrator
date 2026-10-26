package claudecode

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
)

// TestWorkerSecretsCanaryUnits is the always-on half of the worker-secrets
// canary: the settings AO generates must deny the planted secrets and must
// never deny the paths workers need. The live Claude Code run is
// TestWorkerSecretsCanary.
func TestWorkerSecretsCanaryUnits(t *testing.T) {
	f := newProtectedFixture(t)
	extra := filepath.Join(t.TempDir(), "payroll.csv")
	settings, err := buildWorkerSandboxSettings(agentruntime.PermissionAcceptEdits, f.sandbox(extra))
	if err != nil {
		t.Fatal(err)
	}
	denyRead := asStringSlice(t, settings["sandbox"].(map[string]any)["filesystem"].(map[string]any)["denyRead"])
	readRules := asStringSlice(t, settings["permissions"].(map[string]any)["deny"])

	mustDenyRead := []string{
		claudePath(filepath.Join(f.main, ".env"), runtime.GOOS),
		claudePath(filepath.Join(f.home, ".ssh"), runtime.GOOS),
		claudePath(f.token, runtime.GOOS),
		claudePath(extra, runtime.GOOS),
	}
	for _, path := range mustDenyRead {
		if !containsAll(denyRead, path) {
			t.Errorf("denyRead missing %s\n%v", path, denyRead)
		}
	}

	mustRead := []string{
		"Read(/" + claudePath(filepath.Join(f.home, ".ssh"), runtime.GOOS) + ")",
		"Read(/" + claudePath(f.token, runtime.GOOS) + ")",
		"Read(/" + claudePath(extra, runtime.GOOS) + ")",
	}
	envRead := "Read(/" + claudePath(filepath.Join(f.main, "**", ".env"), runtime.GOOS) + ")"
	if !containsAll(readRules, envRead) {
		t.Errorf("Read deny missing main checkout .env glob %s\n%v", envRead, readRules)
	}
	for _, rule := range mustRead {
		if !containsAll(readRules, rule) {
			t.Errorf("Read deny missing %s\n%v", rule, readRules)
		}
	}

	gitDir := claudePath(filepath.Join(f.main, ".git"), runtime.GOOS)
	worktree := claudePath(f.worktree, runtime.GOOS)
	for _, path := range append(append([]string{}, denyRead...), readRules...) {
		if path == gitDir || strings.HasSuffix(path, "Read(/"+gitDir+")") {
			t.Errorf("main .git must never be blocked: %s", path)
		}
		if path == worktree || strings.HasSuffix(path, "Read(/"+worktree+")") {
			t.Errorf("worktree root must never be blocked: %s", path)
		}
	}
}
