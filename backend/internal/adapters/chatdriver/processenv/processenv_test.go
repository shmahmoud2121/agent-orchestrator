package processenv

import (
	"slices"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestMergeInheritsDaemonEnvironmentAndAppliesOverlay(t *testing.T) {
	// Windows has prefix-related names such as PROGRAMFILES and PROGRAMFILES(X86).
	t.Setenv("AO_PROCESSENV", "prefix")
	t.Setenv("AO_PROCESSENV(X86)", "related")
	t.Setenv("AO_PROCESSENV_INHERITED", "parent")
	t.Setenv("AO_PROCESSENV_REPLACED", "old")

	got := Merge(map[string]string{
		"AO_PROCESSENV_REPLACED": "new",
		"AO_PROCESSENV_SESSION":  "session",
	})
	if !slices.IsSorted(got) {
		t.Fatalf("environment is not sorted: %v", got)
	}
	want := map[string]string{
		"AO_PROCESSENV_INHERITED": "parent",
		"AO_PROCESSENV_REPLACED":  "new",
		"AO_PROCESSENV_SESSION":   "session",
	}
	for _, entry := range got {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			if expected, exists := want[key]; exists {
				if value != expected {
					t.Fatalf("%s = %q, want %q", key, value, expected)
				}
				delete(want, key)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing environment values: %v", want)
	}
}

func TestMergeWorkerOverlayDropsInheritedSecrets(t *testing.T) {
	environ := []string{"HOME=/h", "FAKE_SERVICE_TOKEN=CANARY", "OPENAI_API_KEY=sk", "PROJECT_TOKEN=inherited"}
	worker := merge(environ, map[string]string{
		domain.WorkerEnvKeepVar: "OPENAI_API_KEY,CODEX_*",
		"PROJECT_TOKEN":         "chosen",
	}, false)
	for _, entry := range worker {
		if strings.Contains(entry, "CANARY") || entry == "PROJECT_TOKEN=inherited" {
			t.Fatalf("worker env leaked %q: %v", entry, worker)
		}
	}
	if !slices.Contains(worker, "OPENAI_API_KEY=sk") || !slices.Contains(worker, "PROJECT_TOKEN=chosen") || !slices.Contains(worker, "HOME=/h") {
		t.Fatalf("worker env lost a kept variable: %v", worker)
	}
	if manager := merge(environ, map[string]string{"AO_SESSION_ID": "m"}, false); !slices.Contains(manager, "FAKE_SERVICE_TOKEN=CANARY") {
		t.Fatalf("non-worker env must stay whole: %v", manager)
	}
}

func TestMergeWindowsExactPATHWinsConflictingOverlaySpelling(t *testing.T) {
	for range 1000 {
		got := merge(
			[]string{"Path=inherited"},
			map[string]string{"Path": "project", "PATH": "ao-pinned"},
			true,
		)
		var paths []string
		for _, entry := range got {
			key, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(key, "PATH") {
				paths = append(paths, entry)
			}
		}
		if !slices.Equal(paths, []string{"PATH=ao-pinned"}) {
			t.Fatalf("PATH entries = %v, want protected AO PATH", paths)
		}
	}
}
