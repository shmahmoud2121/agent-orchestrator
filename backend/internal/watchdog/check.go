package watchdog

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// Result is the outcome of checking one tool call against a protected list.
type Result struct {
	// Blocked is true when the call must not run.
	Blocked bool
	// Path is the short name shown to the person and the agent, such as ".env".
	Path string
	// Tamper is true when the call writes AO's own hook settings. One write
	// attempt is enough to stop a worker.
	Tamper bool
}

// hookConfigRelPaths are AO hook settings files relative to the workspace.
// Editing them would remove the watchdog. Codex also receives its hooks as
// launch flags; the workspace file is the legacy config those flags replaced.
// Grok reads its AO hooks from .claude/settings.local.json; its project config
// can turn that Claude-compat loading off.
var hookConfigRelPaths = []string{
	filepath.Join(".claude", "settings.local.json"),
	filepath.Join(".claude", "settings.json"),
	filepath.Join(".codex", "hooks.json"),
	filepath.Join(".gemini", "settings.json"),
	filepath.Join(".cursor", "hooks.json"),
	filepath.Join(".grok", "config.toml"),
}

// Check reports whether toolName with raw tool-input JSON touches a protected
// path. workDir is the session's working folder; relative paths resolve against
// it. The check is best-effort for shell commands: it splits on words and does
// not parse every shell trick.
func Check(protected []ReadDeny, workDir, toolName string, input json.RawMessage) Result {
	workDir = cleanOrEmpty(workDir)
	fields := toolFields(input)
	name := canonicalTool(toolName)

	if isShellTool(name) {
		command := firstString(fields, "command", "cmd", "script")
		return checkShell(protected, workDir, command)
	}

	var hit Result
	for _, key := range pathKeys(name) {
		for _, raw := range fieldStrings(fields, key) {
			if next := checkPath(protected, workDir, raw); worse(next, hit) {
				hit = next
			}
			if !isWriteTool(name) {
				continue
			}
			if resolved := resolvePath(workDir, raw); isHookConfig(workDir, resolved) {
				if next := hookHit(displayPath(raw, resolved, workDir)); worse(next, hit) {
					hit = next
				}
			}
		}
	}
	if name == "readmanyfiles" {
		// Gemini's read_many_files takes a list of paths and globs.
		for _, key := range []string{"include", "paths"} {
			for _, raw := range fieldStrings(fields, key) {
				if next := checkPath(protected, workDir, raw); worse(next, hit) {
					hit = next
				}
				if next := checkGlob(protected, raw); worse(next, hit) {
					hit = next
				}
			}
		}
	}
	if isGrepTool(name) || isGlobTool(name) {
		globKey := "glob"
		if isGlobTool(name) {
			// Claude's Glob tool names the file pattern "pattern", not "glob".
			if _, ok := fields["pattern"]; ok {
				globKey = "pattern"
			}
		}
		for _, raw := range fieldStrings(fields, globKey) {
			if isGrepTool(name) && globKey == "pattern" {
				continue
			}
			if next := checkGlob(protected, raw); worse(next, hit) {
				hit = next
			}
		}
		if isGrepTool(name) {
			for _, raw := range append(fieldStrings(fields, "glob"), fieldStrings(fields, "include_pattern")...) {
				if next := checkGlob(protected, raw); worse(next, hit) {
					hit = next
				}
			}
		}
	}
	if name == "applypatch" {
		var patchText []string
		for _, key := range []string{"command", "patch", "input"} {
			patchText = append(patchText, fieldStrings(fields, key)...)
		}
		for _, raw := range patchText {
			for _, file := range patchFiles(raw) {
				if next := checkPath(protected, workDir, file); worse(next, hit) {
					hit = next
				}
				if resolved := resolvePath(workDir, file); isHookConfig(workDir, resolved) {
					if next := hookHit(displayPath(file, resolved, workDir)); worse(next, hit) {
						hit = next
					}
				}
			}
		}
	}
	return hit
}

func checkShell(protected []ReadDeny, workDir, command string) Result {
	var hit Result
	for _, segment := range shellSegments(shellWords(command)) {
		if isAOCommand(segment) {
			continue
		}
		if next := checkShellSegment(protected, workDir, segment); worse(next, hit) {
			hit = next
		}
	}
	return hit
}

func checkShellSegment(protected []ReadDeny, workDir string, words []string) Result {
	var hit Result
	for i, word := range words {
		if isShellFlag(word) || isShellPunctuation(word) {
			continue
		}
		if next := checkPath(protected, workDir, word); worse(next, hit) {
			hit = next
		}
		if next := checkGlob(protected, word); worse(next, hit) {
			hit = next
		}
		if shellWriteTargetsHook(words, i, workDir) {
			if next := hookHit(word); next.Blocked && worse(next, hit) {
				hit = next
			}
		}
	}
	return hit
}

func checkPath(protected []ReadDeny, workDir, raw string) Result {
	raw = strings.TrimSpace(raw)
	if raw == "" || isShellFlag(raw) {
		return Result{}
	}
	if strings.ContainsAny(raw, "*?[") && !filepath.IsAbs(raw) {
		if hit := checkGlob(protected, raw); hit.Blocked {
			return hit
		}
	}
	resolved := resolvePath(workDir, raw)
	if resolved == "" || samePath(resolved, workDir) || isGitInternal(resolved) {
		return Result{}
	}
	for _, entry := range protected {
		if pathMatchesEntry(resolved, entry.Path) {
			return Result{Blocked: true, Path: displayPath(raw, resolved, workDir)}
		}
	}
	return Result{}
}

func checkGlob(protected []ReadDeny, pattern string) Result {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || !strings.ContainsAny(pattern, "*?[") {
		return Result{}
	}
	for _, seg := range splitSlash(pattern) {
		if seg == "" || seg == "*" || seg == "**" || seg == "*.*" {
			continue
		}
		for _, entry := range protected {
			name := lastSegment(entry.Path)
			if name == "" || name == "*" || name == "**" {
				continue
			}
			if globOverlap(seg, name) {
				shown := seg
				if !strings.ContainsAny(shown, "*?[") {
					shown = name
				}
				return Result{Blocked: true, Path: shown}
			}
		}
	}
	return Result{}
}

func hookHit(shown string) Result {
	if shown == "" {
		shown = "AO hook settings"
	}
	return Result{Blocked: true, Path: shown, Tamper: true}
}

func shellWriteTargetsHook(words []string, index int, workDir string) bool {
	word := words[index]
	resolved := resolvePath(workDir, trimRedirect(word))
	if !isHookConfig(workDir, resolved) && !isHookConfig(workDir, resolvePath(workDir, word)) {
		return false
	}
	if isRedirect(word) || (index > 0 && isRedirect(words[index-1])) {
		return true
	}
	return isWriteCommand(words)
}

func isHookConfig(workDir, resolved string) bool {
	if workDir == "" || resolved == "" {
		return false
	}
	for _, rel := range hookConfigRelPaths {
		if samePath(resolved, filepath.Join(workDir, rel)) {
			return true
		}
	}
	return false
}

func isWriteCommand(words []string) bool {
	for _, word := range words {
		if isShellFlag(word) || isShellPunctuation(word) || strings.Contains(word, "=") {
			continue
		}
		switch strings.ToLower(filepath.Base(trimRedirect(word))) {
		case "rm", "del", "erase", "rmdir", "mv", "move", "cp", "copy", "tee",
			"set-content", "add-content", "out-file", "new-item", "remove-item",
			"clear-content", "ni", "sc", "ac", "set-item", "rename-item", "ri":
			return true
		default:
			return strings.Contains(word, ">")
		}
	}
	return false
}

func isRedirect(word string) bool {
	word = strings.TrimSpace(word)
	switch word {
	case ">", ">>", "1>", "2>", "&>", "1>>", "2>>":
		return true
	}
	return false
}

func trimRedirect(word string) string {
	for _, prefix := range []string{"&>>", "1>>", "2>>", "&>", "1>", "2>", ">>", ">"} {
		if strings.HasPrefix(word, prefix) && len(word) > len(prefix) {
			return word[len(prefix):]
		}
	}
	return word
}

func pathMatchesEntry(candidate, entry string) bool {
	if candidate == "" || entry == "" {
		return false
	}
	if strings.ContainsAny(entry, "*?[") {
		return globPathMatch(candidate, entry)
	}
	return domain.PathCovers(entry, candidate)
}

func globPathMatch(candidate, pattern string) bool {
	return matchSegments(splitSlash(foldPath(candidate)), splitSlash(foldPath(pattern)))
}

func matchSegments(got, pattern []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if matchSegments(got, pattern[1:]) {
				return true
			}
			if len(got) == 0 {
				return false
			}
			got = got[1:]
			continue
		}
		if len(got) == 0 {
			return false
		}
		ok, err := path.Match(pattern[0], got[0])
		if err != nil || !ok {
			return false
		}
		pattern = pattern[1:]
		got = got[1:]
	}
	return len(got) == 0
}

func globOverlap(a, b string) bool {
	a = foldPath(a)
	b = foldPath(b)
	if a == b {
		return true
	}
	if strings.ContainsAny(a, "*?[") {
		if ok, err := path.Match(a, b); err == nil && ok {
			return true
		}
	}
	if strings.ContainsAny(b, "*?[") {
		if ok, err := path.Match(b, a); err == nil && ok {
			return true
		}
	}
	return false
}

func resolvePath(workDir, raw string) string {
	raw = trimRedirect(strings.TrimSpace(raw))
	if raw == "" || raw == "." {
		return ""
	}
	if strings.ContainsAny(raw, "*?[") {
		return ""
	}
	p := raw
	if !filepath.IsAbs(p) {
		if workDir == "" {
			return ""
		}
		p = filepath.Join(workDir, p)
	}
	p = filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := p
	var tail []string
	for {
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		if info, err := os.Lstat(parent); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				if resolved, err := filepath.EvalSymlinks(parent); err == nil {
					return filepath.Join(append([]string{resolved}, tail...)...)
				}
			}
			return filepath.Join(append([]string{parent}, tail...)...)
		}
		tail = append([]string{filepath.Base(parent)}, tail...)
		parent = next
	}
	return p
}

func displayPath(raw, resolved, workDir string) string {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned != "" && cleaned != "." && !filepath.IsAbs(cleaned) && !strings.ContainsAny(cleaned, "*?[") {
		return filepath.ToSlash(cleaned)
	}
	if workDir != "" && resolved != "" {
		if rel, err := filepath.Rel(workDir, resolved); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return filepath.ToSlash(rel)
		}
	}
	if resolved != "" {
		return filepath.Base(resolved)
	}
	return filepath.ToSlash(cleaned)
}

func isGitInternal(p string) bool {
	for _, seg := range splitSlash(p) {
		if strings.EqualFold(seg, ".git") {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// isAOCommand reports whether this one command is ao, altair, or their .exe
// names. Only that command is skipped. The caller checks every other segment.
func isAOCommand(words []string) bool {
	for _, word := range words {
		if isSegmentBreak(word) {
			return false
		}
		base := strings.TrimSuffix(strings.ToLower(filepath.Base(word)), ".exe")
		if base == "ao" || base == "altair" {
			return true
		}
		if isShellPunctuation(word) || isShellFlag(word) || strings.Contains(word, "=") {
			continue
		}
		return false
	}
	return false
}

func isShellFlag(word string) bool {
	return strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "./") && !strings.HasPrefix(word, "../") && word != "-"
}

func canonicalTool(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return name
}

func isWriteTool(name string) bool {
	switch name {
	case "edit", "write", "multiedit", "notebookedit", "applypatch",
		// Gemini, Grok, and Cursor names.
		"writefile", "replace", "searchreplace", "strreplace", "editfile", "createfile",
		"deletefile", "delete", "hashlineedit":
		return true
	default:
		return false
	}
}

func isShellTool(name string) bool {
	switch name {
	case "bash", "shell", "shellcommand", "exec", "execcommand", "localshell", "powershell", "command",
		// Gemini's run_shell_command and Grok's run_terminal_cmd.
		"runshellcommand", "runterminalcmd", "runterminalcommand":
		return true
	default:
		return false
	}
}

func isGrepTool(name string) bool {
	return name == "grep" || name == "grepfiles" || name == "search" || name == "grepsearch" || name == "searchfilecontent"
}

func isGlobTool(name string) bool {
	return name == "glob" || name == "globfilesearch" || name == "globfiles"
}

func pathKeys(name string) []string {
	switch name {
	case "notebookedit":
		return []string{"notebook_path", "notebookpath", "file_path", "filepath", "path"}
	default:
		return []string{"file_path", "filepath", "path", "target_file", "targetfile", "notebook_path", "notebookpath",
			// Gemini's absolute_path and dir_path; Grok's and Cursor's target_directory.
			"absolute_path", "absolutepath", "dir_path", "dirpath", "target_directory", "targetdirectory"}
	}
}

func patchFiles(command string) []string {
	var out []string
	for _, line := range strings.Split(command, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"*** Update File:", "*** Add File:", "*** Delete File:", "*** Move to:"} {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				if file := strings.TrimSpace(rest); file != "" {
					out = append(out, file)
				}
			}
		}
	}
	return out
}

func toolFields(input json.RawMessage) map[string]json.RawMessage {
	input = bytesTrimSpace(input)
	if len(input) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return nil
	}
	folded := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		folded[strings.ToLower(key)] = value
	}
	return folded
}

func fieldStrings(fields map[string]json.RawMessage, key string) []string {
	raw, ok := fields[strings.ToLower(key)]
	if !ok {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []string{text}
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	return nil
}

func firstString(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if values := fieldStrings(fields, key); len(values) > 0 && strings.TrimSpace(values[0]) != "" {
			return values[0]
		}
	}
	return ""
}

func shellWords(command string) []string {
	var words []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() == 0 {
			return
		}
		words = append(words, b.String())
		b.Reset()
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			// Inside double quotes, \" is a literal quote (POSIX). Other
			// backslashes stay, so Windows paths are unchanged.
			if quote == '"' && r == '\\' && i+1 < len(runes) && runes[i+1] == '"' {
				b.WriteRune('"')
				i++
				continue
			}
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			op, n, peel := shellOperator(runes, i, b.String())
			if n == 0 {
				b.WriteRune(r)
				continue
			}
			if peel > 0 {
				pending := b.String()
				b.Reset()
				if keep := pending[:len(pending)-peel]; keep != "" {
					words = append(words, keep)
				}
			} else {
				flush()
			}
			words = append(words, op)
			i += n - 1
		}
	}
	flush()
	return words
}

// shellOperator returns the operator at runes[i], how many runes it uses, and
// how many trailing bytes of pending belong to a redirect such as 2>.
func shellOperator(runes []rune, i int, pending string) (string, int, int) {
	has := func(s string) bool {
		if i+len(s) > len(runes) {
			return false
		}
		return string(runes[i:i+len(s)]) == s
	}
	switch runes[i] {
	case ';':
		return ";", 1, 0
	case '|':
		if has("||") {
			return "||", 2, 0
		}
		return "|", 1, 0
	case '&':
		if has("&&") {
			return "&&", 2, 0
		}
		if has("&>") {
			return "&>", 2, 0
		}
		return "&", 1, 0
	case '<':
		return "<", 1, 0
	case '>':
		op, n := ">", 1
		if has(">>") {
			op, n = ">>", 2
		}
		if pending == "1" || pending == "2" {
			return pending + op, n, len(pending)
		}
		return op, n, 0
	default:
		return "", 0, 0
	}
}

func shellSegments(words []string) [][]string {
	var segments [][]string
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		segments = append(segments, current)
		current = nil
	}
	for _, word := range words {
		if isSegmentBreak(word) {
			flush()
			continue
		}
		current = append(current, word)
	}
	flush()
	return segments
}

func isSegmentBreak(word string) bool {
	switch word {
	case "&&", "||", "|", ";", "&":
		return true
	default:
		return false
	}
}

func isShellPunctuation(word string) bool {
	switch word {
	case "&&", "||", "|", ";", "&", "<", ">", ">>", "1>", "2>", "&>", "1>>", "2>>":
		return true
	default:
		return false
	}
}

func splitSlash(p string) []string {
	p = filepath.ToSlash(p)
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func lastSegment(p string) string {
	parts := splitSlash(p)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func foldPath(p string) string {
	p = filepath.ToSlash(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

func bytesTrimSpace(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

// worse reports whether next should replace hit. A tamper attempt outranks a
// plain block, and any block outranks a clean result.
func worse(next, hit Result) bool {
	if !next.Blocked {
		return false
	}
	if !hit.Blocked {
		return true
	}
	return next.Tamper && !hit.Tamper
}
