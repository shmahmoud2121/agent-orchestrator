package watchdog

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// CheckOriginalWrite reports whether a tool call would create, change, move,
// or delete a file under one of the Work project's original paths. Reads and
// copies out of those paths are allowed. sources are absolute. Paths under
// results never count as originals, even when results sits inside an original
// folder. workDir is the session's working folder; relative paths resolve
// against it.
func CheckOriginalWrite(sources []string, results, workDir, toolName string, input json.RawMessage) Result {
	canon := originalSet{sources: canonicalSources(sources), results: canonicalResults(results)}
	if len(canon.sources) == 0 {
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

func canonicalSources(raw []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range raw {
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

// originalSet is the originals plus the results folder. A results folder may
// sit inside an original folder; everything under it belongs to the session.
type originalSet struct {
	sources []string
	results string
}

func canonicalResults(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	canon := filepath.Clean(raw)
	if resolved, err := filepath.EvalSymlinks(canon); err == nil {
		canon = filepath.Clean(resolved)
	}
	return canon
}

func (o originalSet) inResults(resolved string) bool {
	if o.results == "" {
		return false
	}
	if domain.PathCovers(o.results, resolved) {
		return true
	}
	return runtime.GOOS == "windows" && domain.PathCovers(strings.ToLower(o.results), strings.ToLower(resolved))
}

func originalHit(sources originalSet, resolved string) bool {
	if resolved == "" || sources.inResults(resolved) {
		return false
	}
	for _, source := range sources.sources {
		if domain.PathCovers(source, resolved) {
			return true
		}
		if runtime.GOOS == "windows" && domain.PathCovers(strings.ToLower(source), strings.ToLower(resolved)) {
			return true
		}
	}
	return false
}

// originalShellDepth caps how far wrapped shells (bash -c, powershell
// -Command, cmd /c) are unwrapped. Past it, a command that names an original
// is blocked.
const originalShellDepth = 3

func checkOriginalShell(sources originalSet, workDir, command string) Result {
	return checkOriginalShellAt(sources, workDir, command, 0)
}

// checkOriginalShellAt walks the segments in order. A cd moves the folder that
// later relative paths resolve against.
func checkOriginalShellAt(sources originalSet, workDir, command string, depth int) Result {
	if depth > originalShellDepth {
		return mentionsOriginal(sources, workDir, command)
	}
	var hit Result
	dir := workDir
	for _, segment := range shellSegments(shellWords(command)) {
		if isAOCommand(segment) {
			continue
		}
		if next, ok := changedDir(segment, dir, workDir); ok {
			dir = next
			continue
		}
		var next Result
		if inner, wrapped, ok := wrappedShell(segment); wrapped {
			if !ok {
				next = mentionsOriginal(sources, dir, command)
				if !next.Blocked {
					next = Result{Blocked: true, Path: "encoded command"}
				}
			} else {
				next = checkOriginalShellAt(sources, dir, inner, depth+1)
			}
		} else if xargsWriter(segment) {
			next = mentionsOriginal(sources, dir, command)
		} else {
			next = checkOriginalSegment(sources, dir, segment)
		}
		if next.Blocked && worse(next, hit) {
			hit = next
		}
	}
	return hit
}

// mentionsOriginal blocks when any word names an original or a folder that
// holds one. It is the fallback when the command cannot be read word by word.
func mentionsOriginal(sources originalSet, workDir, command string) Result {
	for _, word := range shellWords(command) {
		if isShellPunctuation(word) {
			continue
		}
		if resolved := resolveArg(workDir, word); originalHit(sources, resolved) || holdsOriginal(sources, resolved) {
			return Result{Blocked: true, Path: displayPath(word, resolved, workDir)}
		}
	}
	return Result{}
}

// changedDir reports the folder a cd-style segment moves to. A cd with no
// usable target returns to the session's working folder.
func changedDir(words []string, dir, home string) (string, bool) {
	cmd, at := commandBase(words)
	switch cmd {
	case "cd", "chdir", "pushd", "set-location", "sl", "popd":
	default:
		return "", false
	}
	if cmd == "popd" {
		return home, true
	}
	var args []string
	for _, word := range words[at+1:] {
		if isRedirectOp(word) {
			break
		}
		if isShellFlag(word) || strings.EqualFold(word, "/d") {
			continue
		}
		args = append(args, word)
	}
	if len(args) == 0 || args[0] == "-" || args[0] == "~" {
		return home, true
	}
	if resolved := resolveArg(dir, args[0]); resolved != "" {
		return resolved, true
	}
	return dir, true
}

// wrappedShell returns the command a shell launcher runs. wrapped is false for
// anything that is not a launcher; ok is false when the inner command could
// not be read (a bad -EncodedCommand payload).
func wrappedShell(words []string) (inner string, wrapped, ok bool) {
	cmd, at := commandBase(words)
	if at < 0 {
		return "", false, false
	}
	rest := words[at+1:]
	switch cmd {
	case "bash", "sh", "zsh", "dash", "ksh":
		for i, word := range rest {
			if strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") && strings.Contains(word[1:], "c") {
				if i+1 < len(rest) {
					return rest[i+1], true, true
				}
				return "", false, false
			}
		}
	case "cmd":
		for i, word := range rest {
			if low := strings.ToLower(word); low == "/c" || low == "/k" {
				return strings.Join(rest[i+1:], " "), true, true
			}
		}
	case "powershell", "pwsh":
		return powershellCommand(rest)
	}
	return "", false, false
}

// powershellValueFlags take a value, so the word after them is not the command.
var powershellValueFlags = map[string]bool{
	"executionpolicy": true, "ep": true, "ex": true, "workingdirectory": true, "wd": true,
	"inputformat": true, "inputformats": true, "outputformat": true, "of": true, "windowstyle": true, "w": true,
	"configurationname": true, "settingsfile": true, "version": true, "v": true, "psconsolefile": true,
	"custompipename": true,
}

func powershellCommand(rest []string) (string, bool, bool) {
	for i := 0; i < len(rest); i++ {
		word := rest[i]
		if !strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "/") {
			return strings.Join(rest[i:], " "), true, true
		}
		name := strings.ToLower(strings.TrimLeft(word, "-/"))
		name, value, hasValue := strings.Cut(name, ":")
		switch name {
		case "command", "c", "cmd":
			if hasValue {
				return word[len(word)-len(value):] + " " + strings.Join(rest[i+1:], " "), true, true
			}
			return strings.Join(rest[i+1:], " "), true, true
		case "encodedcommand", "enc", "ec", "e":
			if i+1 >= len(rest) {
				return "", true, false
			}
			decoded, err := decodePowershell(rest[i+1])
			if err != nil {
				return "", true, false
			}
			return decoded, true, true
		case "file", "f":
			return "", false, false
		}
		if powershellValueFlags[name] && !hasValue {
			i++
		}
	}
	return "", false, false
}

// decodePowershell reads an -EncodedCommand payload: base64 of UTF-16LE text.
func decodePowershell(payload string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return "", err
	}
	if len(raw)%2 != 0 {
		return "", errors.New("odd utf-16 length")
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return string(utf16.Decode(units)), nil
}

// xargsWriter reports `xargs rm` and friends: xargs feeding a known writer.
func xargsWriter(words []string) bool {
	cmd, at := commandBase(words)
	if cmd != "xargs" {
		return false
	}
	for _, word := range words[at+1:] {
		if isShellFlag(word) {
			continue
		}
		return originalWriters[strings.TrimSuffix(strings.ToLower(filepath.Base(word)), ".exe")]
	}
	return false
}

// originalWriters change the files they name.
var originalWriters = map[string]bool{
	"rm": true, "rmdir": true, "unlink": true, "mv": true, "move": true, "touch": true, "mkdir": true,
	"chmod": true, "chown": true, "truncate": true, "ln": true, "tee": true, "shred": true,
	"remove-item": true, "move-item": true, "rename-item": true, "set-content": true, "add-content": true,
	"out-file": true, "new-item": true, "clear-content": true, "del": true, "erase": true, "ren": true,
	"rd": true, "ri": true, "mi": true, "rni": true, "sc": true, "ac": true, "clc": true,
	"sed": true,
}

// originalRemovers take a whole folder with them, so naming a folder that
// holds an original is as bad as naming the original.
var originalRemovers = map[string]bool{
	"rm": true, "rmdir": true, "remove-item": true, "ri": true, "rd": true, "del": true, "erase": true,
	"mv": true, "move": true, "move-item": true, "mi": true, "rename-item": true, "ren": true, "rni": true,
	"shred": true,
}

// gitReadOnly are git subcommands that never change the working tree.
var gitReadOnly = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "blame": true, "grep": true,
	"ls-files": true, "ls-tree": true, "rev-parse": true, "describe": true, "shortlog": true,
	"cat-file": true, "rev-list": true,
}

func checkOriginalSegment(sources originalSet, workDir string, words []string) Result {
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
	case "sed":
		if !sedInPlace(words) {
			return Result{}
		}
		return firstOriginalArg(sources, workDir, pathArgs(words), false)
	case "perl", "ruby":
		if !scriptInPlace(words) {
			return Result{}
		}
		return firstOriginalArg(sources, workDir, pathArgs(words), false)
	case "find":
		return checkOriginalFind(sources, workDir, words)
	case "git":
		return checkOriginalGit(sources, workDir, words)
	}
	if !originalWriters[cmd] {
		return Result{}
	}
	args := pathArgs(words)
	if hit := firstOriginalArg(sources, workDir, args, false); hit.Blocked {
		return hit
	}
	if !originalRemovers[cmd] {
		return Result{}
	}
	// A move may name a parent folder as its destination; only the sources it
	// moves away can carry an original with them.
	if cmd == "mv" || cmd == "move" || cmd == "move-item" || cmd == "mi" {
		dest := destinationArg(words, cmd)
		var moved []string
		for _, arg := range args {
			if arg != dest {
				moved = append(moved, arg)
			}
		}
		args = moved
	}
	return firstOriginalArg(sources, workDir, args, true)
}

// firstOriginalArg blocks on the first argument under an original. With
// holders, a folder that holds an original also counts.
func firstOriginalArg(sources originalSet, workDir string, args []string, holders bool) Result {
	for _, raw := range args {
		resolved := resolveArg(workDir, raw)
		if originalHit(sources, resolved) || (holders && holdsOriginal(sources, resolved)) {
			return Result{Blocked: true, Path: displayPath(raw, resolved, workDir)}
		}
	}
	return Result{}
}

// scriptInPlace reports perl or ruby editing files in place: -i, -pi, -i.bak.
func scriptInPlace(words []string) bool {
	for _, word := range words {
		if !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
			continue
		}
		if strings.HasPrefix(word, "-M") || strings.HasPrefix(word, "-m") || strings.HasPrefix(word, "-I") {
			continue
		}
		flags, _, _ := strings.Cut(word[1:], ".")
		if strings.Contains(flags, "i") {
			return true
		}
	}
	return false
}

// checkOriginalFind blocks find -delete, or find -exec running a writer, when
// a search root is an original or holds one.
func checkOriginalFind(sources originalSet, workDir string, words []string) Result {
	_, at := commandBase(words)
	writes := false
	for i, word := range words[at+1:] {
		low := strings.ToLower(word)
		if low == "-delete" {
			writes = true
		}
		if low == "-exec" || low == "-execdir" || low == "-ok" || low == "-okdir" {
			next := at + 2 + i
			if next < len(words) && originalWriters[strings.TrimSuffix(strings.ToLower(filepath.Base(words[next])), ".exe")] {
				writes = true
			}
		}
	}
	if !writes {
		return Result{}
	}
	var roots []string
	for _, word := range words[at+1:] {
		if strings.HasPrefix(word, "-") || word == "(" || word == "!" {
			break
		}
		roots = append(roots, word)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	return firstOriginalArg(sources, workDir, roots, true)
}

// checkOriginalGit allows read-only git anywhere. Any other subcommand is
// blocked when git runs in an original or in a folder that holds one.
func checkOriginalGit(sources originalSet, workDir string, words []string) Result {
	_, at := commandBase(words)
	dir := workDir
	shown := ""
	sub := ""
	var subArgs []string
	rest := words[at+1:]
	for i := 0; i < len(rest); i++ {
		word := rest[i]
		low := strings.ToLower(word)
		switch {
		case sub != "":
			subArgs = append(subArgs, word)
		case word == "-C" && i+1 < len(rest):
			i++
			if resolved := resolveArg(dir, rest[i]); resolved != "" {
				dir, shown = resolved, rest[i]
			}
		case (low == "--git-dir" || low == "--work-tree") && i+1 < len(rest):
			i++
			if resolved := resolveArg(dir, rest[i]); resolved != "" {
				dir, shown = resolved, rest[i]
			}
		case strings.HasPrefix(low, "--git-dir=") || strings.HasPrefix(low, "--work-tree="):
			_, value, _ := strings.Cut(word, "=")
			if resolved := resolveArg(dir, value); resolved != "" {
				dir, shown = resolved, value
			}
		case word == "-c" && i+1 < len(rest):
			i++
		case strings.HasPrefix(word, "-"):
		default:
			sub = low
		}
	}
	if sub == "" || gitSubReadOnly(sub, subArgs) {
		return Result{}
	}
	if originalHit(sources, dir) || holdsOriginal(sources, dir) {
		if shown == "" {
			shown = dir
		}
		return Result{Blocked: true, Path: displayPath(shown, dir, workDir)}
	}
	return Result{}
}

func gitSubReadOnly(sub string, args []string) bool {
	if gitReadOnly[sub] {
		return true
	}
	onlyFlags := func(allowed ...string) bool {
		for _, arg := range args {
			ok := false
			for _, flag := range allowed {
				if arg == flag {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}
	switch sub {
	case "branch":
		return onlyFlags("--list", "-a", "-r", "-v", "-vv", "--all", "--remotes")
	case "remote":
		return onlyFlags("-v")
	case "config":
		return len(args) > 0 && (args[0] == "--get" || args[0] == "--list" || args[0] == "-l" || args[0] == "--get-all")
	}
	return false
}

// resolveArg is resolvePath that also understands "." as the folder itself.
func resolveArg(workDir, raw string) string {
	if strings.TrimSpace(raw) == "." {
		return resolvePath("", cleanOrEmpty(workDir))
	}
	return resolvePath(workDir, raw)
}

// holdsOriginal reports a folder that contains an original, so removing or
// moving the folder would take the original with it.
func holdsOriginal(sources originalSet, resolved string) bool {
	if resolved == "" {
		return false
	}
	for _, source := range sources.sources {
		if domain.PathCovers(resolved, source) {
			return true
		}
		if runtime.GOOS == "windows" && domain.PathCovers(strings.ToLower(resolved), strings.ToLower(source)) {
			return true
		}
	}
	return false
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

func redirectTargetsSource(sources originalSet, workDir string, words []string) (bool, string) {
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

func ddOfTargetsSource(sources originalSet, workDir string, words []string) (bool, string) {
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
