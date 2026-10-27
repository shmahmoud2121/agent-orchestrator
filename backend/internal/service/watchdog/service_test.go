package watchdog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type fakeStore struct {
	session domain.SessionRecord
	ok      bool
	project domain.ProjectRecord
	hasProj bool
}

func (f fakeStore) GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error) {
	return f.session, f.ok, nil
}

func (f fakeStore) GetProject(context.Context, string) (domain.ProjectRecord, bool, error) {
	return f.project, f.hasProj, nil
}

func TestDecideSkipsStandaloneSession(t *testing.T) {
	svc := &Service{Store: fakeStore{ok: true, session: domain.SessionRecord{ID: "solo"}}}
	got, err := svc.Decide(context.Background(), "solo", "Read", json.RawMessage(`{"file_path":".env"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "skip" {
		t.Fatalf("action = %q, want skip", got.Action)
	}
}

func TestDecideBlocksProtectedRead(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "src", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		DataDir: filepath.Join(root, "data"),
		HomeDir: filepath.Join(root, "home"),
		Store: fakeStore{
			ok: true,
			session: domain.SessionRecord{
				ID: "proj-1", ProjectID: "proj", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
				Metadata: domain.SessionMetadata{WorkspacePath: work},
			},
			hasProj: true,
			project: domain.ProjectRecord{
				ID: "proj", Path: project,
				Config: domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{ProtectedPaths: []string{"notes.txt"}}},
			},
		},
	}
	blocked, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":".env"}`))
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Action != "deny" || blocked.Path != ".env" || blocked.Reason == "" {
		t.Fatalf("env read = %+v", blocked)
	}
	allowed, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":"src/main.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Action != "allow" {
		t.Fatalf("source read = %+v, want allow", allowed)
	}
	added, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":`+quote(filepath.Join(project, "notes.txt"))+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if added.Action != "deny" {
		t.Fatalf("added file = %+v, want deny", added)
	}
}

func TestDecideBlocksOriginalWrite(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "originals")
	results := filepath.Join(root, "results")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(results, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(source, "notes.txt")
	if err := os.WriteFile(original, []byte("typo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := &fakeNotifier{}
	svc := &Service{
		DataDir: filepath.Join(root, "data"),
		HomeDir: filepath.Join(root, "home"),
		Notify:  notes,
		Store: fakeStore{
			ok: true,
			session: domain.SessionRecord{
				ID: "work-1", ProjectID: "docs", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
				Metadata: domain.SessionMetadata{WorkspacePath: results},
			},
			hasProj: true,
			project: domain.ProjectRecord{
				ID: "docs", Kind: domain.ProjectKindWork, Path: results,
				Config: domain.ProjectConfig{Work: &domain.WorkConfig{
					Sources: []string{source}, ResultsDir: results,
				}},
			},
		},
	}
	blocked, err := svc.Decide(context.Background(), "work-1", "Edit", json.RawMessage(`{"file_path":`+quote(original)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Action != "deny" || blocked.Reason != domain.WorkOriginalChangedReason {
		t.Fatalf("edit original = %+v", blocked)
	}
	if len(notes.intents) != 1 || notes.intents[0].Reason != domain.WorkOriginalChangedReason || notes.intents[0].Subject != blocked.Path {
		t.Fatalf("alert = %+v", notes.intents)
	}
	allowed, err := svc.Decide(context.Background(), "work-1", "Read", json.RawMessage(`{"file_path":`+quote(original)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Action != "allow" {
		t.Fatalf("read original = %+v, want allow", allowed)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type fakeNotifier struct {
	intents []ports.NotificationIntent
}

func (f *fakeNotifier) Notify(_ context.Context, intent ports.NotificationIntent) error {
	f.intents = append(f.intents, intent)
	return nil
}

type fakeActivity struct {
	summaries []string
}

func (f *fakeActivity) ConversationForSession(context.Context, domain.SessionID) (domain.ConversationRecord, error) {
	return domain.ConversationRecord{ID: "conv-1"}, nil
}

func (f *fakeActivity) UpsertActivity(_ context.Context, _, _ string, activity domain.ConversationActivity, _ time.Time) error {
	f.summaries = append(f.summaries, activity.Summary)
	return nil
}

type fakeStopper struct {
	calls int
}

func (f *fakeStopper) KillPreservingWorkspace(context.Context, domain.SessionID) error {
	f.calls++
	return nil
}

func TestThirdBlockedCallStopsAWorker(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	notes := &fakeNotifier{}
	activity := &fakeActivity{}
	stop := &fakeStopper{}
	svc := &Service{
		DataDir:  filepath.Join(root, "data"),
		HomeDir:  filepath.Join(root, "home"),
		Notify:   notes,
		Activity: activity,
		Stop:     stop,
		Store: fakeStore{
			ok: true,
			session: domain.SessionRecord{
				ID: "worker-1", ProjectID: "proj", DisplayName: "Docs worker",
				Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
				Metadata: domain.SessionMetadata{WorkspacePath: work},
			},
			hasProj: true,
			project: domain.ProjectRecord{ID: "proj", Path: filepath.Join(root, "project")},
		},
	}
	if err := os.MkdirAll(filepath.Join(root, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"file_path":".env"}`)
	for i := 1; i <= 2; i++ {
		got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Stopped {
			t.Fatalf("call %d stopped early", i)
		}
	}
	if stop.calls != 0 {
		t.Fatalf("stop calls before the third block = %d", stop.calls)
	}
	got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped || stop.calls != 1 {
		t.Fatalf("third call stopped=%v calls=%d", got.Stopped, stop.calls)
	}
	if len(activity.summaries) != 3 || activity.summaries[0] != "Watchdog blocked reading .env" {
		t.Fatalf("timeline = %v", activity.summaries)
	}
	var blocked, stopped int
	for _, intent := range notes.intents {
		switch intent.Type {
		case domain.NotificationWatchdogBlocked:
			blocked++
			if intent.SourceKey != "watchdog:worker-1:.env" || intent.Agent != "Claude Code" || intent.Subject != ".env" {
				t.Fatalf("block alert = %+v", intent)
			}
		case domain.NotificationWatchdogStopped:
			stopped++
			if intent.SessionDisplayName != "Docs worker" {
				t.Fatalf("stop alert = %+v", intent)
			}
		default:
			t.Fatalf("unexpected alert %s", intent.Type)
		}
	}
	if blocked != 3 || stopped != 1 {
		t.Fatalf("alerts blocked=%d stopped=%d", blocked, stopped)
	}
	log, err := os.ReadFile(filepath.Join(root, "data", "watchdog.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(log)
	if !strings.Contains(text, "action=blocked") || !strings.Contains(text, "action=stopped") {
		t.Fatalf("log = %s", text)
	}
	if !strings.Contains(text, "project=proj") || !strings.Contains(text, "ai=claude-code") || !strings.Contains(text, "tool=Read") {
		t.Fatalf("log = %s", text)
	}
}

func TestStrikeCountClearsAfterAStop(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := &fakeStopper{}
	svc := workerService(t, root, work, domain.KindWorker, stop)
	input := json.RawMessage(`{"file_path":".env"}`)
	for i := 1; i <= 3; i++ {
		got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && got.Stopped {
			t.Fatalf("call %d stopped early", i)
		}
		if i == 3 && !got.Stopped {
			t.Fatal("third call did not stop")
		}
	}
	if stop.calls != 1 {
		t.Fatalf("stops = %d, want 1", stop.calls)
	}
	for i := 1; i <= 2; i++ {
		got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Stopped {
			t.Fatalf("block %d after the stop stopped again", i)
		}
	}
	if stop.calls != 1 {
		t.Fatalf("stops after two more blocks = %d, want 1", stop.calls)
	}
	got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stopped || stop.calls != 2 {
		t.Fatalf("third block after the stop: stopped=%v calls=%d", got.Stopped, stop.calls)
	}
}

type errStopper struct {
	calls int
}

func (f *errStopper) KillPreservingWorkspace(context.Context, domain.SessionID) error {
	f.calls++
	return errors.New("busy")
}

func TestFailedStopKeepsTheStrikeCount(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := &errStopper{}
	svc := workerService(t, root, work, domain.KindWorker, stop)
	input := json.RawMessage(`{"file_path":".env"}`)
	for i := 1; i <= 4; i++ {
		got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Stopped {
			t.Fatalf("call %d reported stopped after a failed kill", i)
		}
	}
	if stop.calls != 2 {
		t.Fatalf("stop attempts = %d, want 2", stop.calls)
	}
}

func TestTamperStopsAWorkerAtOnce(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	stop := &fakeStopper{}
	svc := workerService(t, root, work, domain.KindWorker, stop)
	got, err := svc.Decide(context.Background(), "worker-1", "Edit", json.RawMessage(`{"file_path":".claude/settings.local.json"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tamper || !got.Stopped || stop.calls != 1 {
		t.Fatalf("tamper = %+v calls=%d", got, stop.calls)
	}
}

func TestManagerIsBlockedButNotStopped(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := &fakeStopper{}
	svc := workerService(t, root, work, domain.KindOrchestrator, stop)
	input := json.RawMessage(`{"file_path":".env"}`)
	for i := 0; i < 3; i++ {
		got, err := svc.Decide(context.Background(), "worker-1", "Read", input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Action != "deny" || got.Stopped {
			t.Fatalf("manager read = %+v", got)
		}
	}
	tamper, err := svc.Decide(context.Background(), "worker-1", "Edit", json.RawMessage(`{"file_path":".claude/settings.local.json"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !tamper.Tamper || tamper.Stopped || stop.calls != 0 {
		t.Fatalf("manager tamper = %+v calls=%d", tamper, stop.calls)
	}
}

type flakyNotifier struct {
	fail    int
	intents []ports.NotificationIntent
}

func (f *flakyNotifier) Notify(_ context.Context, intent ports.NotificationIntent) error {
	if f.fail > 0 {
		f.fail--
		return errors.New("database is locked")
	}
	f.intents = append(f.intents, intent)
	return nil
}

func TestNotifyRetriesUntilTheAlertIsSaved(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	notes := &flakyNotifier{fail: 2}
	svc := workerService(t, root, work, domain.KindWorker, &fakeStopper{})
	svc.Notify = notes
	got, err := svc.Decide(context.Background(), "worker-1", "Read", json.RawMessage(`{"file_path":".env"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "deny" || len(notes.intents) != 1 || notes.intents[0].Type != domain.NotificationWatchdogBlocked {
		t.Fatalf("alert = %+v decision=%+v", notes.intents, got)
	}
}

func workerService(t *testing.T, root, work string, kind domain.SessionKind, stop Stopper) *Service {
	t.Helper()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Service{
		DataDir: filepath.Join(root, "data"),
		HomeDir: filepath.Join(root, "home"),
		Stop:    stop,
		Store: fakeStore{
			ok: true,
			session: domain.SessionRecord{
				ID: "worker-1", ProjectID: "proj", Kind: kind, Harness: domain.HarnessClaudeCode,
				Metadata: domain.SessionMetadata{WorkspacePath: work},
			},
			hasProj: true,
			project: domain.ProjectRecord{ID: "proj", Path: project},
		},
	}
}
