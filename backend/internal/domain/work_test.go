package domain

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProjectKindIsGitless(t *testing.T) {
	cases := []struct {
		kind ProjectKind
		want bool
	}{
		{ProjectKindScratch, true},
		{ProjectKindWork, true},
		{ProjectKindSingleRepo, false},
		{ProjectKindWorkspace, false},
		{"", false},
	}
	for _, tc := range cases {
		if got := tc.kind.IsGitless(); got != tc.want {
			t.Errorf("IsGitless(%q) = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestValidateWorkLayout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "originals")
	nested := filepath.Join(source, "notes")
	results := filepath.Join(root, "results")
	ao := filepath.Join(root, "ao-home")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ao, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok := &WorkConfig{Sources: []string{source, file}, ResultsDir: results}
	if err := ValidateWorkLayout(ok, ao); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(results)
	if err != nil || !info.IsDir() {
		t.Fatalf("results dir was not created: %v", err)
	}

	inside := &WorkConfig{Sources: []string{source}, ResultsDir: nested}
	if err := ValidateWorkLayout(inside, ao); err != nil {
		t.Fatalf("results inside a source folder: %v", err)
	}
	overlapUp := &WorkConfig{Sources: []string{filepath.Join(results, "later")}, ResultsDir: results}
	if err := os.MkdirAll(filepath.Join(results, "later"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := workCode(t, ValidateWorkLayout(overlapUp, ao)); code != "WORK_PATH_OVERLAP" {
		t.Fatalf("source inside results code = %s", code)
	}
	equal := &WorkConfig{Sources: []string{source}, ResultsDir: source}
	if code := workCode(t, ValidateWorkLayout(equal, ao)); code != "WORK_PATH_OVERLAP" {
		t.Fatalf("equal paths code = %s", code)
	}
	under := &WorkConfig{Sources: []string{filepath.Join(ao, "secret")}, ResultsDir: results}
	if err := os.MkdirAll(filepath.Join(ao, "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := workCode(t, ValidateWorkLayout(under, ao)); code != "WORK_PATH_UNDER_AO" {
		t.Fatalf("source under ao code = %s", code)
	}
	resultsUnder := &WorkConfig{Sources: []string{source}, ResultsDir: filepath.Join(ao, "out")}
	if code := workCode(t, ValidateWorkLayout(resultsUnder, ao)); code != "WORK_PATH_UNDER_AO" {
		t.Fatalf("results under ao code = %s", code)
	}
	missing := &WorkConfig{Sources: []string{filepath.Join(root, "missing")}, ResultsDir: results}
	if code := workCode(t, ValidateWorkLayout(missing, ao)); code != "WORK_SOURCE_MISSING" {
		t.Fatalf("missing source code = %s", code)
	}
	none := &WorkConfig{ResultsDir: results}
	if code := workCode(t, ValidateWorkLayout(none, ao)); code != "WORK_SOURCE_REQUIRED" {
		t.Fatalf("no source code = %s", code)
	}
	if code := workCode(t, ValidateWorkLayout(nil, ao)); code != "WORK_CONFIG_REQUIRED" {
		t.Fatalf("nil config code = %s", code)
	}
	relative := &WorkConfig{Sources: []string{source}, ResultsDir: "results"}
	if code := workCode(t, ValidateWorkLayout(relative, ao)); code != "WORK_RESULTS_NOT_ABSOLUTE" {
		t.Fatalf("relative results code = %s", code)
	}
	many := make([]string, MaxWorkSources+1)
	for i := range many {
		many[i] = source
	}
	if code := workCode(t, ValidateWorkLayout(&WorkConfig{Sources: many, ResultsDir: filepath.Join(root, "other")}, ao)); code != "WORK_SOURCE_TOO_MANY" {
		t.Fatalf("too many sources code = %s", code)
	}
}

func TestValidateWorkLayoutWindowsCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows path case")
	}
	root := t.TempDir()
	source := filepath.Join(root, "Originals")
	results := filepath.Join(root, "Results")
	ao := filepath.Join(root, "ao-home")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ao, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &WorkConfig{Sources: []string{strings.ToUpper(source)}, ResultsDir: strings.ToLower(results)}
	if err := ValidateWorkLayout(cfg, strings.ToUpper(ao)); err != nil {
		t.Fatal(err)
	}
	overlap, err := PathsOverlap(strings.ToLower(source), strings.ToUpper(source))
	if err != nil || !overlap {
		t.Fatalf("case-only overlap = %v err=%v", overlap, err)
	}
	nested := filepath.Join(source, "Inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	inside, err := PathsOverlap(strings.ToUpper(source), strings.ToLower(nested))
	if err != nil || !inside {
		t.Fatalf("case-insensitive containment = %v err=%v", inside, err)
	}
}

func TestUniqueTaskFolderSuffix(t *testing.T) {
	results := filepath.Join(t.TempDir(), "results")
	when := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	taken := map[string]bool{}
	first, err := UniqueTaskFolder(results, "Fix: the <typo>", when, func(p string) bool { return taken[p] })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "2026-10-27 Fix the typo" {
		t.Fatalf("folder = %q", filepath.Base(first))
	}
	taken[first] = true
	second, err := UniqueTaskFolder(results, "Fix: the <typo>", when, func(p string) bool { return taken[p] })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) != "2026-10-27 Fix the typo (2)" {
		t.Fatalf("clash folder = %q", filepath.Base(second))
	}
	if second == first {
		t.Fatal("reused the existing folder")
	}
}

func TestWorkHarnessWatched(t *testing.T) {
	for _, h := range []AgentHarness{HarnessClaudeCode, HarnessCodex, HarnessGrok, HarnessGemini} {
		if !WorkHarnessWatched(h) {
			t.Fatalf("%s is watched", h)
		}
	}
	// Cursor's hooks are wired but its read deny is not yet proven.
	for _, h := range []AgentHarness{HarnessCursor, HarnessOpenCode, ""} {
		if WorkHarnessWatched(h) {
			t.Fatalf("%q is not watched", h)
		}
	}
}

func workCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var workErr *WorkConfigError
	if !errors.As(err, &workErr) {
		t.Fatalf("error type %T: %v", err, err)
	}
	return workErr.Code
}

func TestWorkWriteDenyKeepsResultsInsideOriginal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "06_Archive")
	results := filepath.Join(source, "Output")
	for _, dir := range []string{filepath.Join(source, "letters"), results} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "contract.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	got := WorkWriteDeny(&WorkConfig{Sources: []string{source, other}, ResultsDir: results})
	want := []string{filepath.Join(source, "contract.pdf"), filepath.Join(source, "letters"), other}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("deny = %v, want %v", got, want)
	}
}
