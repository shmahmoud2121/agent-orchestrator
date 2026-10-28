package watchdog

import (
	"encoding/base64"
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
			got := CheckOriginalWrite(sources, "", tc.workDir, tc.tool, tc.input)
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

func TestCheckOriginalWriteShellShapes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	task := filepath.Join(root, "results", "task")
	for _, dir := range []string{source, task} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	notes := filepath.Join(source, "notes.txt")
	if err := os.WriteFile(notes, []byte("receive"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := quote
	encode := func(text string) string {
		var raw []byte
		for _, r := range text {
			raw = append(raw, byte(r), byte(r>>8))
		}
		return base64.StdEncoding.EncodeToString(raw)
	}

	cases := []struct {
		name    string
		command string
		blocked bool
	}{
		{"powershell -Command", `powershell -Command "Set-Content -Path '` + notes + `' -Value hi"`, true},
		{"powershell.exe -NoProfile -Command", `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Remove-Item '` + notes + `'"`, true},
		{"pwsh -c", `pwsh -c "Remove-Item ` + q(notes) + `"`, true},
		{"powershell statements", `powershell -Command "Get-Content x; Set-Content -Path '` + notes + `' -Value hi"`, true},
		{"powershell encoded", "powershell -EncodedCommand " + encode("Remove-Item '"+notes+"'"), true},
		{"powershell bad encoded", "powershell -enc not-base64!!", true},
		{"bash -c", `bash -c "rm ` + q(notes) + `"`, true},
		{"sh -lc", `sh -lc "echo hi > ` + q(notes) + `"`, true},
		{"cmd /c", `cmd /c del ` + q(notes), true},
		{"nested wrappers", `bash -c "powershell -Command \"Remove-Item ` + notes + `\""`, true},
		{"cd then rm", "cd " + q(source) + " && rm notes.txt", true},
		{"Set-Location then Set-Content", "Set-Location " + q(source) + "; Set-Content notes.txt hi", true},
		{"cd then cd back", "cd " + q(source) + " && cd - && rm notes.txt", false},
		{"perl -pi", "perl -pi -e 's/a/b/' " + q(notes), true},
		{"perl -i.bak", "perl -i.bak -pe 's/a/b/' " + q(notes), true},
		{"ruby -i", "ruby -i -pe 'x' " + q(notes), true},
		{"find -delete", "find " + q(source) + " -name '*.txt' -delete", true},
		{"find -exec rm", "find " + q(source) + " -name '*.txt' -exec rm {} +", true},
		{"find parent -delete", "find " + q(root) + " -name '*.txt' -delete", true},
		{"xargs rm", "find " + q(source) + " -name '*.txt' | xargs rm", true},
		{"git -C checkout", "git -C " + q(source) + " checkout -- .", true},
		{"cd then git reset", "cd " + q(source) + " && git reset --hard", true},
		{"git clean in parent", "git -C " + q(root) + " clean -fdx", true},
		{"rm parent folder", "rm -rf " + q(root), true},
		{"Remove-Item parent", "Remove-Item -Recurse " + q(root), true},
		{"mv parent folder", "mv " + q(root) + " " + q(filepath.Join(task, "gone")), true},

		{"cat", "cat " + q(notes), false},
		{"cp out", "cp " + q(notes) + " " + q(filepath.Join(task, "copy.txt")), false},
		{"Get-Content", "Get-Content " + q(notes), false},
		{"powershell read", `powershell -Command "Get-Content '` + notes + `'"`, false},
		{"cd then ls", "cd " + q(source) + " && ls", false},
		{"git log", "git -C " + q(source) + " log", false},
		{"git status", "git -C " + q(source) + " status", false},
		{"git diff", "git -C " + q(source) + " diff", false},
		{"python reads", "python read.py " + q(notes), false},
		{"find lists", "find " + q(source) + " -name '*.txt'", false},
		{"xargs cat", "find " + q(source) + " -name '*.txt' | xargs cat", false},
		{"mv into parent", "mv " + q(filepath.Join(task, "out.txt")) + " " + q(root), false},
		{"rm in task", "rm -rf " + q(filepath.Join(task, "old")), false},
		{"bash script in task", `bash -c "echo hi > out.txt"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"command": tc.command})
			if err != nil {
				t.Fatal(err)
			}
			got := CheckOriginalWrite([]string{source}, "", task, "Bash", raw)
			if got.Blocked != tc.blocked {
				t.Fatalf("blocked = %v, want %v (path %q) for %s", got.Blocked, tc.blocked, got.Path, tc.command)
			}
		})
	}
}

func TestCheckOriginalWriteResultsInsideOriginal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "06_Archive")
	results := filepath.Join(source, "Output")
	task := filepath.Join(results, "task")
	if err := os.MkdirAll(task, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(source, "contract.pdf")
	if err := os.WriteFile(sourceFile, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := func(command string) json.RawMessage {
		raw, _ := json.Marshal(map[string]string{"command": command})
		return raw
	}
	write := func(path string) json.RawMessage {
		raw, _ := json.Marshal(map[string]string{"file_path": path})
		return raw
	}
	for _, tc := range []struct {
		name    string
		tool    string
		input   json.RawMessage
		blocked bool
	}{
		{"write in task", "Write", write(filepath.Join(task, "summary.md")), false},
		{"write in results root", "Write", write(filepath.Join(results, "index.md")), false},
		{"relative write", "Write", write("summary.md"), false},
		{"rm in results", "Bash", shell("rm " + quote(filepath.Join(task, "old.md"))), false},
		{"edit original beside results", "Edit", write(sourceFile), true},
		{"new file in original folder", "Write", write(filepath.Join(source, "new.md")), true},
		{"relative escape to original", "Write", write("../../contract.pdf"), true},
		{"rm original folder", "Bash", shell("rm -rf " + quote(source)), true},
		{"mv result over original", "Bash", shell("mv summary.md " + quote(sourceFile)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckOriginalWrite([]string{source}, results, task, tc.tool, tc.input)
			if got.Blocked != tc.blocked {
				t.Fatalf("blocked = %v, want %v (path %q)", got.Blocked, tc.blocked, got.Path)
			}
		})
	}
}
