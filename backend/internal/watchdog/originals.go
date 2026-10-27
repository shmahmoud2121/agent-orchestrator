package watchdog

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// CheckOriginalWrite reports whether a tool call would create, change, move,
// or delete a file under one of the Work project's original paths. Reads and
// copies out of those paths are allowed. sources are absolute. workDir is the
// session's working folder; relative paths resolve against it.
func CheckOriginalWrite(sources []string, workDir, toolName string, input json.RawMessage) Result {
	canon := canonicalSources(sources)
	if len(canon) == 0 {
		return Result{}
	}
	workDir = cleanOrEmpty(workDir)
	name := canonicalTool(toolName)
	fields := toolFields(input)

	if isShellTool(name) {
		command := firstString(fields, "command", "cmd", "script")
		return checkOriginalShell(canon, workDir, command)
	}
	if !isWriteTool(name) && name != "applypatch" {
		return Result{}
	}
	var paths []string
	collectJSONPaths(input, &paths)
	if name == "applypatch" {
		for _, key := range []string{"command", "patch", "input"} {
			for _, raw := range fieldStrings(fields, key) {
				paths = append(paths, patchFiles(raw)...)
			}
		}
	}
	for _, raw := range paths {
		if resolved := resolvePath(workDir, raw); originalHit(canon, resolved) {
			return Result{Blocked: true, Path: displayPath(raw, resolved, workDir)}
		}
	}
	return Result{}
}

func canonicalSources(sources []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range sources {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		canon := filepath.Clean(raw)
		if resolved, err := filepath.EvalSymlinks(canon); err == nil {
			canon = filepath.Clean(resolved)
		}
		key := canon
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, canon)
	}
	return out
}

func originalHit(sources []string, resolved string) bool {
	if resolved == "" {
		return false
	}
	for _, source := range sources {
		if domain.PathCovers(source, resolved) {
			return true
		}
		if runtime.GOOS == "windows" && domain.PathCovers(strings.ToLower(source), strings.ToLower(resolved)) {
			return true
		}
	}
	return false
}

func checkOriginalShell(sources []string, workDir, command string) Result {
	var hit Result
	for _, segment := range shellSegments(shellWords(command)) {
		if isAOCommand(segment) {
			continue
		}
		if next := checkOriginalSegment(sources, workDir, segment); next.Blocked && worse(next, hit) {
			hit = next
		}
	}
	return hit
}

func checkOriginalSegment(sources []string, workDir string, words []string) Result {
	if blocked, shown := redirectTargetsSource(sources, workDir, words); blocked {
		return Result{Blocked: true, Path: shown}
	}
	if blocked, shown := ddOfTargetsSource(sources, workDir, words); blocked {
		return Result{Blocked: true, Path: shown}
	}
	cmd, _ := commandBase(words)
	switch cmd {
	case "":
		return Result{}
	case "cp", "rsync", "robocopy", "copy-item", "copy", "cpi":
		dest := destinationArg(words, cmd)
		if resolved := resolvePath(workDir, dest); originalHit(sources, resolved) {
			return Result{Blocked: true, Path: displayPath(dest, resolved, workDir)}
		}
		return Result{}
	case "rm", "rmdir", "unlink", "mv", "move", "touch", "mkdir", "chmod", "chown", "truncate", "ln", "tee",
		"remove-item", "move-item", "rename-item", "set-content", "add-content", "out-file", "new-item",
		"del", "erase", "ren", "rd", "ri", "mi", "rni", "sc", "ac":
		for _, raw := range pathArgs(words) {
			if resolved := resolvePath(workDir, raw); originalHit(sources, resolved) {
				return Result{Blocked: true, Path: displayPath(raw, resolved, workDir)}
			}
		}
		return Result{}
	case "sed":
		if !sedInPlace(words) {
			return Result{}
		}
		for _, raw := range pathArgs(words) {
			if resolved := resolvePath(workDir, raw); originalHit(sources, resolved) {
				return Result{Blocked: true, Path: displayPath(raw, resolved, workDir)}
			}
		}
	}
	return Result{}
}

func commandBase(words []string) (string, int) {
	for i, word := range words {
		if isRedirectOp(word) {
			break
		}
		if isShellFlag(word) || isShellPunctuation(word) {
			continue
		}
		if isEnvAssignment(word) {
			continue
		}
		base := strings.TrimSuffix(strings.ToLower(filepath.Base(word)), ".exe")
		return base, i
	}
	return "", -1
}

func isEnvAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 || strings.HasPrefix(word, "-") {
		return false
	}
	key := word[:eq]
	if key == "" || strings.ContainsAny(key, `/\`) {
		return false
	}
	for _, r := range key {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func isRedirectOp(word string) bool {
	switch word {
	case ">", ">>", "&>", "1>", "2>", "1>>", "2>>":
		return true
	default:
		return false
	}
}

func pathArgs(words []string) []string {
	var out []string
	for _, word := range words {
		if isRedirectOp(word) || isShellFlag(word) || isShellPunctuation(word) || isEnvAssignment(word) {
			continue
		}
		if isCommandName(word, words) {
			continue
		}
		out = append(out, trimRedirect(word))
	}
	return out
}

func isCommandName(word string, words []string) bool {
	base, _ := commandBase(words)
	got := strings.TrimSuffix(strings.ToLower(filepath.Base(word)), ".exe")
	return base != "" && got == base && word == words[indexOfBase(words)]
}

func indexOfBase(words []string) int {
	_, i := commandBase(words)
	return i
}

func destinationArg(words []string, cmd string) string {
	for i, word := range words {
		low := strings.ToLower(word)
		switch {
		case low == "-destination" || low == "-dest" || low == "-t" || low == "--target-directory":
			if i+1 < len(words) {
				return words[i+1]
			}
		case strings.HasPrefix(low, "-destination:") || strings.HasPrefix(low, "-dest:"):
			_, rest, _ := strings.Cut(word, ":")
			if rest != "" {
				return rest
			}
		}
	}
	paths := pathArgs(words)
	if cmd == "robocopy" && len(paths) >= 2 {
		return paths[1]
	}
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

func sedInPlace(words []string) bool {
	for _, word := range words {
		low := strings.ToLower(word)
		if low == "-i" || strings.HasPrefix(low, "-i") {
			return true
		}
	}
	return false
}

func redirectTargetsSource(sources []string, workDir string, words []string) (bool, string) {
	for i, word := range words {
		target := ""
		switch {
		case isRedirectOp(word):
			if i+1 < len(words) {
				target = words[i+1]
			}
		case strings.HasPrefix(word, ">>"):
			target = strings.TrimPrefix(word, ">>")
		case strings.HasPrefix(word, ">"):
			target = strings.TrimPrefix(word, ">")
		}
		if target == "" {
			continue
		}
		if resolved := resolvePath(workDir, target); originalHit(sources, resolved) {
			return true, displayPath(target, resolved, workDir)
		}
	}
	return false, ""
}

func ddOfTargetsSource(sources []string, workDir string, words []string) (bool, string) {
	for _, word := range words {
		low := strings.ToLower(word)
		if !strings.HasPrefix(low, "of=") {
			continue
		}
		target := word[len("of="):]
		if resolved := resolvePath(workDir, target); originalHit(sources, resolved) {
			return true, displayPath(target, resolved, workDir)
		}
	}
	return false, ""
}

func collectJSONPaths(raw json.RawMessage, into *[]string) {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	walkJSONPaths(value, into)
}

func walkJSONPaths(value any, into *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isOriginalPathKey(key) {
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
					*into = append(*into, text)
				}
			}
			walkJSONPaths(child, into)
		}
	case []any:
		for _, child := range typed {
			walkJSONPaths(child, into)
		}
	}
}

func isOriginalPathKey(key string) bool {
	switch strings.ToLower(key) {
	case "file_path", "filepath", "path", "target_file", "targetfile", "notebook_path", "notebookpath":
		return true
	default:
		return false
	}
}
