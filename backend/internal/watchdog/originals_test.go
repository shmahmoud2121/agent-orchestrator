package watchdog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckOriginalWrite(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	results := filepath.Join(root, "results")
	task := filepath.Join(results, "task")
	for _, dir := range []string{source, task} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sourceFile := filepath.Join(source, "x")
	if err := os.WriteFile(sourceFile, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(results, "a"), []byte("copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	sources := []string{source}

	shell := func(command string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"command": command})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	fileTool := func(path string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"file_path": path})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	cases := []struct {
		name    string
		tool    string
		input   json.RawMessage
		workDir string
		blocked bool
		windows bool
	}{
		{"edit source", "Edit", fileTool(sourceFile), task, true, false},
		{"rm source", "Bash", shell("rm " + quote(sourceFile)), task, true, false},
		{"mv from source", "Bash", shell("mv " + quote(sourceFile) + " " + quote(filepath.Join(task, "out"))), task, true, false},
		{"mv into source", "Bash", shell("mv " + quote(filepath.Join(task, "out")) + " " + quote(filepath.Join(source, "out"))), task, true, false},
		{"redirect into source", "Bash", shell("echo hi > " + quote(filepath.Join(source, "x"))), task, true, false},
		{"sed in place", "Bash", shell("sed -i 's/a/b/' " + quote(sourceFile)), task, true, false},
		{"cp into source", "Bash", shell("cp " + quote(filepath.Join(results, "a")) + " " + quote(filepath.Join(source, "b"))), root, true, false},
		{"compound rm", "Bash", shell("ls; rm " + quote(filepath.Join(source, "x"))), root, true, false},
		{"cat source", "Bash", shell("cat " + quote(sourceFile)), task, false, false},
		{"cp out of source", "Bash", shell("cp " + quote(sourceFile) + " " + quote(results)), root, false, false},
		{"read source", "Read", fileTool(sourceFile), task, false, false},
		{"write in task", "Bash", shell("echo hi > " + quote(filepath.Join(task, "note.txt"))), task, false, false},
		{"edit in task", "Write", fileTool(filepath.Join(task, "note.txt")), task, false, false},
		{"remove-item", "Bash", shell("Remove-Item " + quote(sourceFile)), task, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.windows && runtime.GOOS != "windows" {
				t.Skip("windows command")
			}
			got := CheckOriginalWrite(sources, tc.workDir, tc.tool, tc.input)
			if got.Blocked != tc.blocked {
				t.Fatalf("blocked = %v, want %v (path %q)", got.Blocked, tc.blocked, got.Path)
			}
		})
	}
}

func quote(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + path + `"`
	}
	return "'" + path + "'"
}
