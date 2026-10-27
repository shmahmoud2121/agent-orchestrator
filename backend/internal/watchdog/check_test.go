package watchdog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestCheckBlocksProtectedReads(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "project")
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	for _, path := range []string{
		filepath.Join(main, ".env"),
		filepath.Join(main, "src", "main.go"),
		filepath.Join(main, "data", "app.db"),
		filepath.Join(main, "sub", "place"),
		filepath.Join(work, "src", "main.go"),
		filepath.Join(work, ".env"),
	} {
		writeFile(t, path)
	}
	protected := SessionReadDenyPaths(domain.WorkerSandboxConfig{
		ProtectedPaths: []string{"notes/secret.txt"},
		Launch: domain.WorkerSandboxLaunch{
			ProjectPath: main, WorkspacePath: work, HomeDir: home,
		},
	})
	writeFile(t, filepath.Join(main, "notes", "secret.txt"))

	envAbs := filepath.Join(work, ".env")
	cases := []struct {
		name    string
		dir     string
		tool    string
		input   string
		blocked bool
		tamper  bool
		path    string
	}{
		{"read env", work, "Read", `{"file_path":".env"}`, true, false, ".env"},
		{"read parent env", work, "Read", `{"file_path":"./sub/../.env"}`, true, false, ".env"},
		{"read absolute env", work, "Read", jsonInput("file_path", envAbs), true, false, ".env"},
		{"read source", work, "Read", `{"file_path":"src/main.go"}`, false, false, ""},
		{"read added file", work, "Read", jsonInput("file_path", filepath.Join(main, "notes", "secret.txt")), true, false, "secret.txt"},
		{"cat env", work, "Bash", `{"command":"cat .env"}`, true, false, ".env"},
		{"cat quoted env", work, "Bash", `{"command":"cat \"./.env\""}`, true, false, ".env"},
		{"copy main database", main, "Bash", `{"command":"cp data/app.db /tmp/x"}`, true, false, "data/app.db"},
		{"go test", work, "Bash", `{"command":"go test ./..."}`, false, false, ""},
		{"ls", work, "Bash", `{"command":"ls"}`, false, false, ""},
		{"ao command", work, "Bash", `{"command":"ao session send --message hi"}`, false, false, ""},
		{"ao then cat", work, "Bash", `{"command":"ao status && cat .env"}`, true, false, ".env"},
		{"ao then cat semicolon", work, "Bash", `{"command":"ao status; cat .env"}`, true, false, ".env"},
		{"ao then cat pipe", work, "Bash", `{"command":"ao --help | cat .env"}`, true, false, ".env"},
		{"ao then cat or", work, "Bash", `{"command":"ao status || cat .env"}`, true, false, ".env"},
		{"ao then cat background", work, "Bash", `{"command":"ao status & cat .env"}`, true, false, ".env"},
		{"trailing semicolon", work, "Bash", `{"command":"cat .env;"}`, true, false, ".env"},
		{"pipe stuck to path", work, "Bash", `{"command":"cat .env|head"}`, true, false, ".env"},
		{"redirect in", work, "Bash", `{"command":"cat<.env"}`, true, false, ".env"},
		{"redirect out", work, "Bash", `{"command":"cat .env>out"}`, true, false, ".env"},
		{"ao alone", work, "Bash", `{"command":"ao status"}`, false, false, ""},
		{"ao then go test", work, "Bash", `{"command":"ao session ls && go test ./..."}`, false, false, ""},
		{"read hook settings", work, "Read", `{"file_path":".claude/settings.json"}`, false, false, ""},
		{"grep hook settings", work, "Grep", `{"path":".claude/settings.json","pattern":"hooks"}`, false, false, ""},
		{"edit committed hook settings", work, "Edit", `{"file_path":".claude/settings.json"}`, true, true, ".claude/settings.json"},
		{"grep env glob", work, "Grep", `{"path":"src","pattern":"TODO","glob":"**/.env*"}`, true, false, ".env*"},
		{"grep source", work, "Grep", `{"path":"src","pattern":"password"}`, false, false, ""},
		{"edit hook settings", work, "Edit", `{"file_path":".claude/settings.local.json"}`, true, true, ".claude/settings.local.json"},
		{"git internals", work, "Bash", `{"command":"git show .git/config"}`, false, false, ""},
	}
	// The main checkout's database glob applies when the command runs there.
	// Rebuild the list the way a worker outside the main checkout sees it, then
	// point the copy at that checkout.
	mainProtected := SessionReadDenyPaths(domain.WorkerSandboxConfig{
		Launch: domain.WorkerSandboxLaunch{ProjectPath: main, WorkspacePath: work, HomeDir: home},
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := protected
			if tc.name == "copy main database" {
				list = mainProtected
			}
			got := Check(list, tc.dir, tc.tool, json.RawMessage(tc.input))
			if got.Blocked != tc.blocked || got.Tamper != tc.tamper {
				t.Fatalf("Check() = %+v, want blocked=%v tamper=%v", got, tc.blocked, tc.tamper)
			}
			if tc.blocked && got.Path != tc.path {
				t.Fatalf("path = %q, want %q", got.Path, tc.path)
			}
		})
	}
}

func TestCheckFollowsSymlink(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	secret := filepath.Join(root, "secret.env")
	writeFile(t, secret)
	writeFile(t, filepath.Join(work, "src", "main.go"))
	link := filepath.Join(work, "alias.env")
	if err := os.Symlink(secret, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation is not permitted")
		}
		t.Fatal(err)
	}
	protected := []ReadDeny{{Path: secret}}
	got := Check(protected, work, "Read", json.RawMessage(`{"file_path":"alias.env"}`))
	if !got.Blocked {
		t.Fatalf("symlink read = %+v, want blocked", got)
	}
}

func TestCheckShellWriteTamper(t *testing.T) {
	work := t.TempDir()
	got := Check(nil, work, "Bash", json.RawMessage(`{"command":"echo x > .claude/settings.json"}`))
	if !got.Blocked || !got.Tamper {
		t.Fatalf("shell write = %+v, want tamper", got)
	}
	read := Check(nil, work, "Bash", json.RawMessage(`{"command":"type .claude/settings.json"}`))
	if read.Blocked {
		t.Fatalf("shell read of hook settings = %+v, want allowed", read)
	}
	chained := Check(nil, work, "Bash", json.RawMessage(`{"command":"echo hi; rm .claude/settings.local.json"}`))
	if !chained.Blocked || !chained.Tamper {
		t.Fatalf("chained shell write = %+v, want tamper", chained)
	}
}

func jsonInput(key, value string) string {
	body, err := json.Marshal(map[string]string{key: value})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
