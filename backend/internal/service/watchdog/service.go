// Package watchdog decides whether a project session's tool call may run.
// The daemon owns the protected list. The hook only sends the tool name and
// input, then prints the decision.
package watchdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/apitoken"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/watchdog"
)

// ErrSessionNotFound means the hook named a session this daemon does not have.
var ErrSessionNotFound = errors.New("watchdog: session not found")

// Decision is what the hook should do with one tool call.
type Decision struct {
	// Action is allow, deny, or skip. Skip means this session is not watched
	// (no project); the hook prints nothing.
	Action   string
	Path     string
	Reason   string
	Tamper   bool
	Stopped  bool
	Original bool
}

// Store loads the session and project the decision is based on.
type Store interface {
	GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error)
	GetProject(ctx context.Context, id string) (domain.ProjectRecord, bool, error)
}

// Notifier is the existing notification write path.
type Notifier interface {
	Notify(ctx context.Context, intent ports.NotificationIntent) error
}

// ActivityWriter records one line on the session's chat timeline.
type ActivityWriter interface {
	ConversationForSession(ctx context.Context, session domain.SessionID) (domain.ConversationRecord, error)
	UpsertActivity(ctx context.Context, conversationID, providerTurnID string, activity domain.ConversationActivity, now time.Time) error
}

// Stopper stops a worker and leaves its worktree on disk.
type Stopper interface {
	KillPreservingWorkspace(ctx context.Context, id domain.SessionID) error
}

// Service checks tool calls for project sessions.
type Service struct {
	Store    Store
	DataDir  string
	HomeDir  string
	Notify   Notifier
	Activity ActivityWriter
	Stop     Stopper

	mu     sync.Mutex
	blocks map[domain.SessionID]int
}

const watchdogStopAfter = 3

// Decide reports whether the tool call may run. Standalone sessions, which
// have no project, are skipped. A project session whose protected list cannot
// be loaded is denied.
func (s *Service) Decide(ctx context.Context, id domain.SessionID, toolName string, toolInput json.RawMessage) (Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, errors.New("watchdog: store is required")
	}
	session, ok, err := s.Store.GetSession(ctx, id)
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{}, ErrSessionNotFound
	}
	if session.ProjectID == "" {
		return Decision{Action: "skip"}, nil
	}
	project, ok, err := s.Store.GetProject(ctx, string(session.ProjectID))
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return denyDecision("protected files"), nil
	}
	workDir := strings.TrimSpace(session.Metadata.WorkspacePath)
	if workDir == "" {
		workDir = project.Path
	}
	home := s.HomeDir
	if home == "" {
		if dir, err := os.UserHomeDir(); err == nil {
			home = dir
		}
	}
	list := watchdog.SessionReadDenyPaths(domain.WorkerSandboxConfig{
		ProtectedPaths: project.Config.WorkerSandbox.ProtectedPaths,
		Launch: domain.WorkerSandboxLaunch{
			ProjectPath:   project.Path,
			WorkspacePath: workDir,
			HomeDir:       home,
			APITokenPath:  apitoken.Path(s.DataDir),
		},
	})
	hit := watchdog.Check(list, workDir, toolName, toolInput)
	if hit.Blocked {
		decision := denyDecision(hit.Path)
		decision.Tamper = hit.Tamper
		s.recordBlock(ctx, session, toolName, &decision)
		return decision, nil
	}
	if project.Kind.WithDefault() == domain.ProjectKindWork && project.Config.Work != nil && len(project.Config.Work.Sources) > 0 {
		original := watchdog.CheckOriginalWrite(project.Config.Work.Sources, workDir, toolName, toolInput)
		if original.Blocked {
			decision := Decision{
				Action:   "deny",
				Path:     original.Path,
				Reason:   domain.WorkOriginalChangedReason,
				Original: true,
			}
			s.recordBlock(ctx, session, toolName, &decision)
			return decision, nil
		}
	}
	return Decision{Action: "allow"}, nil
}

func denyDecision(path string) Decision {
	shown := strings.TrimSpace(path)
	if shown == "" {
		shown = "that file"
	}
	return Decision{
		Action: "deny",
		Path:   shown,
		Reason: fmt.Sprintf("AO blocked this: %s is a protected file. Do the task without reading it, or ask the person.", shown),
	}
}
