package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// MaxWorkSources is the most original folders or files one Work project accepts.
const MaxWorkSources = 50

// WorkOriginalChangedReason is the watchdog reason when a tool call would
// change one of the person's original files.
const WorkOriginalChangedReason = "tried to change an original file"

// WorkGuardUnavailableMessage is returned when a Work session would otherwise
// start without every original-file guard.
const WorkGuardUnavailableMessage = "Altair couldn't turn on the original-file guard for this session"

// WorkHarnessUnwatchedMessage is returned when a Work session would use an AI
// the watchdog does not cover.
const WorkHarnessUnwatchedMessage = "Work projects need an AI that Altair can watch."

// WorkConfig is the durable layout of a Work project. Sources are the person's
// original folders or files. ResultsDir is where agents write.
type WorkConfig struct {
	Sources    []string `json:"sources"`
	ResultsDir string   `json:"resultsDir"`
}

// WorkConfigError is a validation failure with a stable API code.
type WorkConfigError struct {
	Code    string
	Message string
}

func (e *WorkConfigError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func workErr(code, message string) error {
	return &WorkConfigError{Code: code, Message: message}
}

// WorkHarnessWatched reports whether the watchdog covers this harness. Work
// sessions may use only these harnesses.
func WorkHarnessWatched(h AgentHarness) bool {
	switch h {
	case HarnessClaudeCode, HarnessCodex:
		return true
	default:
		return false
	}
}

// ValidateWorkLayout checks a Work project's originals and results folder.
// It creates ResultsDir when the layout is valid and the folder is missing.
// aoHome is the Altair home (usually ~/.ao). Paths are compared after Clean
// and EvalSymlinks of the longest existing ancestor. On Windows the comparison
// ignores case.
func ValidateWorkLayout(cfg *WorkConfig, aoHome string) error {
	if cfg == nil {
		return workErr("WORK_CONFIG_REQUIRED", "A Work project needs its original files and a results folder.")
	}
	if len(cfg.Sources) == 0 {
		return workErr("WORK_SOURCE_REQUIRED", "Add at least one original folder or file.")
	}
	if len(cfg.Sources) > MaxWorkSources {
		return workErr("WORK_SOURCE_TOO_MANY", fmt.Sprintf("A Work project can have at most %d originals.", MaxWorkSources))
	}
	home, err := canonicalAOHome(aoHome)
	if err != nil {
		return err
	}
	sources := make([]string, 0, len(cfg.Sources))
	seen := map[string]struct{}{}
	for _, raw := range cfg.Sources {
		raw = strings.TrimSpace(raw)
		if raw == "" || !filepath.IsAbs(raw) {
			return workErr("WORK_SOURCE_NOT_ABSOLUTE", "Each original must be an absolute path.")
		}
		if _, err := os.Lstat(raw); err != nil {
			return workErr("WORK_SOURCE_MISSING", fmt.Sprintf("Original %s was not found.", raw))
		}
		canon, err := CanonicalPath(raw)
		if err != nil {
			return workErr("WORK_SOURCE_MISSING", fmt.Sprintf("Original %s could not be resolved.", raw))
		}
		if pathCoversFold(home, canon) {
			return workErr("WORK_PATH_UNDER_AO", "Originals cannot live inside the Altair home folder.")
		}
		key := foldPathKey(canon)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, canon)
	}
	resultsRaw := strings.TrimSpace(cfg.ResultsDir)
	if resultsRaw == "" || !filepath.IsAbs(resultsRaw) {
		return workErr("WORK_RESULTS_NOT_ABSOLUTE", "The results folder must be an absolute path.")
	}
	results, err := CanonicalPath(resultsRaw)
	if err != nil {
		return workErr("WORK_RESULTS_NOT_ABSOLUTE", "The results folder could not be resolved.")
	}
	if pathCoversFold(home, results) {
		return workErr("WORK_PATH_UNDER_AO", "The results folder cannot live inside the Altair home folder.")
	}
	// The results folder may sit inside an original folder (an "Output"
	// subfolder); it may not be the original itself or hold one.
	for _, source := range sources {
		if pathCoversFold(results, source) {
			return workErr("WORK_PATH_OVERLAP", "The results folder cannot be an original, and an original cannot be inside the results folder.")
		}
	}
	if err := os.MkdirAll(results, 0o755); err != nil {
		return workErr("WORK_RESULTS_CREATE", "The results folder could not be created.")
	}
	cfg.Sources = sources
	cfg.ResultsDir = results
	return nil
}

// CanonicalPath returns a cleaned absolute path. When the path exists, it is
// the EvalSymlinks result. Otherwise the longest existing ancestor is resolved
// and the missing tail is joined back on.
func CanonicalPath(p string) (string, error) {
	p = filepath.Clean(strings.TrimSpace(p))
	if p == "" || !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute")
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved), nil
	}
	parent := p
	var tail []string
	for {
		next := filepath.Dir(parent)
		if next == parent {
			return p, nil
		}
		if _, err := os.Lstat(parent); err == nil {
			if resolved, err := filepath.EvalSymlinks(parent); err == nil {
				parent = filepath.Clean(resolved)
			}
			return filepath.Join(append([]string{parent}, tail...)...), nil
		}
		tail = append([]string{filepath.Base(parent)}, tail...)
		parent = next
	}
}

func canonicalAOHome(aoHome string) (string, error) {
	aoHome = strings.TrimSpace(aoHome)
	if aoHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", workErr("WORK_PATH_UNDER_AO", "Altair couldn't find the home folder, so it can't check that originals stay outside it.")
		}
		aoHome = filepath.Join(home, ".ao")
	}
	if !filepath.IsAbs(aoHome) {
		return "", workErr("WORK_PATH_UNDER_AO", "Altair couldn't resolve its home folder.")
	}
	canon, err := CanonicalPath(aoHome)
	if err != nil {
		return filepath.Clean(aoHome), nil
	}
	return canon, nil
}

func pathCoversFold(parent, child string) bool {
	if runtime.GOOS == "windows" {
		parent = strings.ToLower(parent)
		child = strings.ToLower(child)
	}
	return PathCovers(parent, child)
}

func foldPathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// PathsOverlap reports whether the two absolute paths are the same or one is
// inside the other, after CanonicalPath. On Windows the comparison ignores case.
func PathsOverlap(a, b string) (bool, error) {
	left, err := CanonicalPath(a)
	if err != nil {
		return false, err
	}
	right, err := CanonicalPath(b)
	if err != nil {
		return false, err
	}
	return pathCoversFold(left, right) || pathCoversFold(right, left), nil
}

// AOHome returns Altair's home folder. When dataDir ends in "data" (the
// default ~/.ao/data layout, including ~/.ao/dev/data), the home is its parent.
// Any other data directory, such as a lab AO_DATA_DIR, is its own home so the
// scratch folder stays inside that directory.
func AOHome(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ""
	}
	clean := filepath.Clean(dataDir)
	if strings.EqualFold(filepath.Base(clean), "data") {
		return filepath.Dir(clean)
	}
	return clean
}

// WorkScratchDir is the private scratch folder for one Work session. It is
// removed when the session ends. The directory is not created here.
func WorkScratchDir(dataDir, projectID, sessionID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	home := AOHome(dataDir)
	if home == "" || projectID == "" || sessionID == "" {
		return "", fmt.Errorf("work scratch folder needs a data directory, project, and session")
	}
	if strings.ContainsAny(projectID, `/\`) || strings.ContainsAny(sessionID, `/\`) {
		return "", fmt.Errorf("work scratch folder ids must not contain path separators")
	}
	return filepath.Join(home, "work", projectID, sessionID), nil
}

// SanitizeTaskTitle strips characters Windows rejects in a file name and
// trailing dots or spaces. An empty title becomes "task".
func SanitizeTaskTitle(title string) string {
	title = strings.TrimSpace(title)
	if i := strings.IndexAny(title, "\r\n"); i >= 0 {
		title = strings.TrimSpace(title[:i])
	}
	var b strings.Builder
	for _, r := range title {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimRight(strings.TrimSpace(b.String()), ". ")
	const maxRunes = 80
	runes := []rune(out)
	if len(runes) > maxRunes {
		out = strings.TrimRight(string(runes[:maxRunes]), ". ")
	}
	if out == "" || windowsReservedName(out) {
		if out == "" {
			out = "task"
		} else {
			out += "_"
		}
	}
	return out
}

func windowsReservedName(name string) bool {
	base := name
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		base = name[:dot]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

// UniqueTaskFolder picks ResultsDir/<YYYY-MM-DD> <title>, adding " (2)", " (3)"
// and so on while exists reports the path is already there. It never returns
// a path exists already accepted.
func UniqueTaskFolder(resultsDir, title string, when time.Time, exists func(string) bool) (string, error) {
	resultsDir = strings.TrimSpace(resultsDir)
	if resultsDir == "" || !filepath.IsAbs(resultsDir) {
		return "", workErr("WORK_RESULTS_NOT_ABSOLUTE", "The results folder must be an absolute path.")
	}
	if when.IsZero() {
		when = time.Now()
	}
	base := when.Format("2006-01-02") + " " + SanitizeTaskTitle(title)
	name := base
	for n := 2; n < 10000; n++ {
		path := filepath.Join(resultsDir, name)
		if exists == nil || !exists(path) {
			return path, nil
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	return "", fmt.Errorf("could not find a free task folder name")
}

// WorkWriteDeny lists the paths a Work worker's sandbox must not write. An
// original folder that holds the results folder is replaced by its entries,
// minus the branch that leads to results, because the sandbox lets a denied
// folder win over an allowed folder inside it.
func WorkWriteDeny(cfg *WorkConfig) []string {
	if cfg == nil {
		return nil
	}
	results := strings.TrimSpace(cfg.ResultsDir)
	var out []string
	for _, source := range cfg.Sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if !WorkResultsInsideOriginal(&WorkConfig{Sources: []string{source}, ResultsDir: results}) {
			out = append(out, source)
			continue
		}
		out = append(out, denyAround(source, results)...)
	}
	return out
}

// WorkResultsInsideOriginal reports a results folder kept inside one of the
// original folders.
func WorkResultsInsideOriginal(cfg *WorkConfig) bool {
	if cfg == nil || strings.TrimSpace(cfg.ResultsDir) == "" {
		return false
	}
	results := strings.TrimSpace(cfg.ResultsDir)
	for _, source := range cfg.Sources {
		source = strings.TrimSpace(source)
		if source != "" && foldPathKey(source) != foldPathKey(results) && pathCoversFold(source, results) {
			return true
		}
	}
	return false
}

// denyAround returns every entry under dir except the chain of folders that
// leads down to keep.
func denyAround(dir, keep string) []string {
	rel, err := filepath.Rel(dir, keep)
	if err != nil {
		return []string{dir}
	}
	var out []string
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return append(out, dir)
		}
		for _, entry := range entries {
			if !samePathPart(entry.Name(), part) {
				out = append(out, filepath.Join(dir, entry.Name()))
			}
		}
		dir = filepath.Join(dir, part)
	}
	return out
}

func samePathPart(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// WorkReadableDirs is the set of directories Claude Code may be given with
// --add-dir. A file original contributes its parent directory. Codex must not
// receive these: its extra-directory flag grants writes.
func WorkReadableDirs(project ProjectRecord, harness AgentHarness) []string {
	if project.Kind.WithDefault() != ProjectKindWork || harness != HarnessClaudeCode || project.Config.Work == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, source := range project.Config.Work.Sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		dir := source
		if info, err := os.Stat(source); err == nil && !info.IsDir() {
			dir = filepath.Dir(source)
		}
		key := foldPathKey(dir)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, dir)
	}
	return out
}
