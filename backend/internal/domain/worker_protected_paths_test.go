package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The settings page shows a copy of the built-in list; keep it identical.
func TestBuiltinWorkerProtectedPathsMatchesSettingsCopy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "frontend", "src", "renderer", "lib", "worker-protected-paths.ts"))
	if err != nil {
		t.Fatal(err)
	}
	group := func(name string) []string {
		block := regexp.MustCompile(`(?s)` + name + `: \[(.*?)\]`).FindSubmatch(data)
		if block == nil {
			t.Fatalf("no %s list in the settings copy", name)
		}
		var out []string
		for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(block[1], -1) {
			out = append(out, string(m[1]))
		}
		return out
	}
	list := BuiltinWorkerProtectedPaths()
	for name, want := range map[string][]string{"mainCheckout": list.MainCheckout, "worktree": list.Worktree, "home": list.Home} {
		if got := group(name); !slices.Equal(got, want) {
			t.Errorf("%s: settings copy %v, domain %v", name, got, want)
		}
	}
}

func TestValidateProtectedPath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "exports")
	for _, ok := range []string{
		"config/prod.yml", "data/*.csv", "private/", "~/.config/stripe", abs, "a/../b",
	} {
		if err := ValidateProtectedPath(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", " x", "x ", "!.env.example", "../outside", "a/../../outside", ".git", ".git/config", ".GIT/hooks",
		".", "*", "**", "**/*", "~", "~/", "~/..", "/", "line\nbreak", strings.Repeat("a", MaxWorkerProtectedPathLen+1),
	} {
		if err := ValidateProtectedPath(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestWorkerSandboxValidateProtectedPaths(t *testing.T) {
	if err := (WorkerSandboxConfig{ProtectedPaths: []string{"a", "a"}}).Validate(); err == nil {
		t.Fatal("duplicate entries must be refused")
	}
	tooMany := make([]string, MaxWorkerProtectedPaths+1)
	for i := range tooMany {
		tooMany[i] = "f" + strings.Repeat("x", i)
	}
	if err := (WorkerSandboxConfig{ProtectedPaths: tooMany}).Validate(); err == nil {
		t.Fatal("too many entries must be refused")
	}
}

func TestWorkerSandboxValidateForProject(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proj")
	home := filepath.Dir(project)
	for _, bad := range []string{project, filepath.Join(project, ".git"), "~/proj"} {
		cfg := WorkerSandboxConfig{ProtectedPaths: []string{bad}}
		if err := cfg.ValidateForProject(project, home); err == nil {
			t.Errorf("%q covers .git: want error", bad)
		}
	}
	cfg := WorkerSandboxConfig{ProtectedPaths: []string{filepath.Join(project, "dump.sql"), "~/proj/notes"}}
	if err := cfg.ValidateForProject(project, home); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinWorkerProtectedPaths(t *testing.T) {
	list := BuiltinWorkerProtectedPaths()
	for _, entry := range list.Worktree {
		if strings.Contains(entry, ".db") || strings.Contains(entry, "sqlite") {
			t.Fatalf("worktree must keep scratch databases readable: %q", entry)
		}
	}
	for _, entry := range append(append([]string(nil), list.MainCheckout...), list.Worktree...) {
		if strings.HasPrefix(entry, ".git") {
			t.Fatalf(".git must never be protected: %q", entry)
		}
	}
	for _, name := range []string{".env.example", ".ENV.Sample", ".env.template"} {
		if !IsEnvTemplateFile(name) {
			t.Errorf("%s is a sample", name)
		}
	}
	if IsEnvTemplateFile(".env.production") {
		t.Error(".env.production is not a sample")
	}
}
