package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestWorkerSecretsCanaryEnvFilter(t *testing.T) {
	cfg := ProjectConfig{
		Env:           map[string]string{"PROJECT_TOKEN": "chosen"},
		WorkerEnvKeep: []string{"NPM_TOKEN"},
	}
	overlay := map[string]string{
		WorkerEnvKeepVar: WorkerEnvKeepList(HarnessClaudeCode, cfg, nil),
		"PROJECT_TOKEN":  "chosen",
	}
	kept, removed := FilterWorkerEnv([]string{
		"FAKE_SERVICE_TOKEN=CANARY-7f3a",
		"ANTHROPIC_API_KEY=sk-ant",
		"NPM_TOKEN=npm",
		"PROJECT_TOKEN=old",
		"PWD=/w",
	}, overlay, false)
	if !slices.Contains(removed, "FAKE_SERVICE_TOKEN") {
		t.Fatalf("planted token must be stripped; removed %v", removed)
	}
	for _, name := range removed {
		if strings.Contains(name, "CANARY") || strings.Contains(name, "=") {
			t.Fatalf("removed names must not carry values: %q", name)
		}
	}
	for _, want := range []string{"ANTHROPIC_API_KEY=sk-ant", "NPM_TOKEN=npm", "PROJECT_TOKEN=old", "PWD=/w"} {
		if !slices.Contains(kept, want) {
			t.Errorf("kept %v, missing %s", kept, want)
		}
	}
}
