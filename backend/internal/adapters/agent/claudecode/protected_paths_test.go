package claudecode

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
)

type protectedFixture struct {
	main, worktree, home, token string
}

func newProtectedFixture(t *testing.T) protectedFixture {
	t.Helper()
	root := t.TempDir()
	f := protectedFixture{
		main:     filepath.Join(root, "project"),
		worktree: filepath.Join(root, "home", ".ao", "worktrees", "w1"),
		home:     filepath.Join(root, "home"),
		token:    filepath.Join(root, "home", ".ao", "api-token"),
	}
	for _, rel := range []string{
		".env", ".env.example", ".env.sample", ".env.template", ".env.qa", "app/.env.custom",
		"config/secrets/prod.json", "node_modules/pkg/.env.leak", ".git/refs/heads/backup/x",
	} {
		writeFixtureFile(t, filepath.Join(f.main, filepath.FromSlash(rel)))
	}
	writeFixtureFile(t, filepath.Join(f.worktree, ".env.review"))
	writeFixtureFile(t, filepath.Join(f.worktree, "scratch.db"))
	return f
}

func writeFixtureFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f protectedFixture) sandbox(extra ...string) domain.WorkerSandboxConfig {
	return domain.WorkerSandboxConfig{
		ProtectedPaths: extra,
		Launch: domain.WorkerSandboxLaunch{
			ProjectPath: f.main, WorkspacePath: f.worktree, HomeDir: f.home, APITokenPath: f.token,
		},
	}
}

func denyPaths(t *testing.T, sandbox domain.WorkerSandboxConfig) []string {
	t.Helper()
	out := []string{}
	for _, entry := range workerReadDenyPaths(sandbox) {
		out = append(out, entry.path)
	}
	return out
}

func TestWorkerReadDenyPathsBuiltinList(t *testing.T) {
	f := newProtectedFixture(t)
	got := denyPaths(t, f.sandbox())
	want := []string{
		filepath.Join(f.main, ".env"),
		filepath.Join(f.main, "**", ".env"),
		filepath.Join(f.main, "**", "*.pem"),
		filepath.Join(f.main, "**", "*.db"),
		filepath.Join(f.main, "**", "credentials.json"),
		filepath.Join(f.main, "secrets"),
		filepath.Join(f.main, "backups"),
		filepath.Join(f.main, "**", ".env.production"),
		filepath.Join(f.main, ".env.qa"),
		filepath.Join(f.main, "app", ".env.custom"),
		filepath.Join(f.main, "config", "secrets"),
		filepath.Join(f.worktree, "**", ".env"),
		filepath.Join(f.worktree, "**", "*.key"),
		filepath.Join(f.worktree, ".env.review"),
		filepath.Join(f.home, ".ssh"),
		filepath.Join(f.home, ".config", "gcloud"),
		filepath.Join(f.home, ".docker", "config.json"),
		filepath.Join(f.home, ".config", "gh"),
		f.token,
	}
	for _, w := range want {
		if !containsAll(got, w) {
			t.Errorf("missing %s", w)
		}
	}
	for _, path := range got {
		base := filepath.Base(path)
		if domain.IsEnvTemplateFile(base) {
			t.Errorf("sample file must stay readable: %s", path)
		}
		if strings.Contains(path, "node_modules") {
			t.Errorf("scan must skip node_modules: %s", path)
		}
		if strings.HasPrefix(path, f.worktree) && (strings.HasSuffix(path, ".db") || strings.HasSuffix(path, ".sqlite")) {
			t.Errorf("worktree scratch databases must stay readable: %s", path)
		}
		if path == filepath.Join(f.main, ".git") || strings.HasPrefix(path, filepath.Join(f.main, ".git")+string(filepath.Separator)) {
			t.Errorf("main .git must never be blocked: %s", path)
		}
		if path == f.worktree {
			t.Errorf("worktree root must never be blocked")
		}
	}
}

func TestWorkerReadDenyPathsProjectEntries(t *testing.T) {
	f := newProtectedFixture(t)
	abs := filepath.Join(t.TempDir(), "customer-export.csv")
	got := denyPaths(t, f.sandbox("config/prod.yml", "~/.config/stripe", abs, "~", f.worktree, filepath.Join(f.main, ".git")))
	for _, w := range []string{
		filepath.Join(f.main, "config", "prod.yml"),
		filepath.Join(f.home, ".config", "stripe"),
		abs,
	} {
		if !containsAll(got, w) {
			t.Errorf("missing project entry %s in %v", w, got)
		}
	}
	for _, path := range got {
		if path == f.home || path == f.worktree || path == filepath.Join(f.main, ".git") {
			t.Errorf("entry covering the worktree or .git must be dropped: %s", path)
		}
	}
}

func TestWorkerReadDenyPathsWorkerInMainCheckout(t *testing.T) {
	f := newProtectedFixture(t)
	sandbox := f.sandbox()
	sandbox.Launch.WorkspacePath = f.main
	for _, path := range denyPaths(t, sandbox) {
		if strings.HasSuffix(path, "*.db") {
			t.Fatalf("a worker in the main checkout keeps its scratch databases: %s", path)
		}
	}
}

func TestClaudePathWindowsDrive(t *testing.T) {
	if got := claudePath(`C:\Users\alice\.ssh`, "windows"); got != "/c/Users/alice/.ssh" {
		t.Fatalf("windows path = %q", got)
	}
	if got := claudePath("/home/alice/.ssh", "linux"); got != "/home/alice/.ssh" {
		t.Fatalf("unix path = %q", got)
	}
	_, rules := readDenySettings([]readDeny{{path: `D:\proj\**\.env`}}, "windows")
	if rules[0] != "Read(//d/proj/**/.env)" {
		t.Fatalf("windows read rule = %q", rules[0])
	}
}

func TestReadDenySettingsBothPlaces(t *testing.T) {
	f := newProtectedFixture(t)
	settings, err := buildWorkerSandboxSettings(agentruntime.PermissionAcceptEdits, f.sandbox())
	if err != nil {
		t.Fatal(err)
	}
	denyRead := asStringSlice(t, settings["sandbox"].(map[string]any)["filesystem"].(map[string]any)["denyRead"])
	deny := asStringSlice(t, settings["permissions"].(map[string]any)["deny"])
	envGlob := claudePath(filepath.Join(f.main, "**", ".env"), runtime.GOOS)
	if !containsAll(denyRead, envGlob, claudePath(filepath.Join(f.main, ".env"), runtime.GOOS)) {
		t.Fatalf("denyRead = %v", denyRead)
	}
	if !containsAll(deny, "Read(/"+envGlob+")", "Read(/"+claudePath(f.token, runtime.GOOS)+")") {
		t.Fatalf("Read deny rules = %v", deny)
	}
	if containsAll(deny, "Read(/"+claudePath(filepath.Join(f.main, ".env"), runtime.GOOS)+")") {
		t.Fatal("the exact twin of a /**/ glob is redundant in a Read rule")
	}
}

// The ao CLI keeps reading the API token: it is excluded from the sandbox, so
// denyRead does not apply to it, and no Bash rule denies plain ao commands.
// Only the Read tool and sandboxed shell commands are refused the token.
func TestAPITokenDeniedButAOCLIExcluded(t *testing.T) {
	f := newProtectedFixture(t)
	settings, err := buildWorkerSandboxSettings(agentruntime.PermissionAcceptEdits, f.sandbox())
	if err != nil {
		t.Fatal(err)
	}
	sandbox := settings["sandbox"].(map[string]any)
	token := claudePath(f.token, runtime.GOOS)
	if !containsAll(asStringSlice(t, sandbox["filesystem"].(map[string]any)["denyRead"]), token) {
		t.Fatal("API token must be in denyRead")
	}
	if !containsAll(asStringSlice(t, sandbox["excludedCommands"]), "ao", "ao *") {
		t.Fatal("ao must be excluded from the sandbox so it can read its token")
	}
	perms := settings["permissions"].(map[string]any)
	if !containsAll(asStringSlice(t, perms["allow"]), "Bash(ao)", "Bash(ao *)") {
		t.Fatal("ao commands must stay allowed")
	}
	for _, rule := range asStringSlice(t, perms["deny"]) {
		if rule == "Bash(ao)" || rule == "Bash(ao *)" || rule == "Bash(ao:*)" {
			t.Fatalf("plain ao commands must not be denied: %s", rule)
		}
	}
}
