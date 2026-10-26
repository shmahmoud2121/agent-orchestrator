package agentbase

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestNativeJSONSessionRequiresExactIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	for _, tc := range []struct {
		name, output string
		want         bool
	}{
		{"export", `{"info":{"id":"original"},"messages":[{"text":"papaya"}]}`, true},
		{"list", `[{"id":"other"},{"id":"original"}]`, true},
		{"different", `{"info":{"id":"other"}}`, false},
		{"missing", `[]`, false},
		{"invalid", `no JSON`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "native")
			script := "#!/bin/sh\n[ \"$AO_RESUME_TEST\" = inherited ] || exit 1\n[ \"$PWD\" = \"$AO_RESUME_CWD\" ] || exit 2\n[ -z \"$FAKE_SERVICE_TOKEN\" ] || exit 3\nprintf '%s' '" + tc.output + "'\n"
			if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FAKE_SERVICE_TOKEN", "CANARY")
			got, err := NativeJSONSessionExists(context.Background(), binary, []string{"export", "original"}, ports.SessionRef{WorkspacePath: dir}, "original", map[string]string{
				"AO_RESUME_TEST": "inherited", "AO_RESUME_CWD": canonicalPath(t, dir),
				domain.WorkerEnvKeepVar: "ANTHROPIC_API_KEY",
			})
			if got != tc.want || (tc.want && err != nil) {
				t.Fatalf("exists=%t, error=%v, want %t", got, err, tc.want)
			}
		})
	}
}

func TestNativeJSONSessionExistsKeepsUnmarkedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "native")
	script := "#!/bin/sh\n[ \"$FAKE_SERVICE_TOKEN\" = CANARY ] || exit 1\nprintf '%s' '{\"info\":{\"id\":\"original\"}}'\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_SERVICE_TOKEN", "CANARY")
	got, err := NativeJSONSessionExists(context.Background(), binary, []string{"export", "original"}, ports.SessionRef{WorkspacePath: dir}, "original", map[string]string{"AO_RESUME_TEST": "inherited"})
	if !got || err != nil {
		t.Fatalf("unmarked probe must inherit the daemon environment: exists=%t err=%v", got, err)
	}
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
