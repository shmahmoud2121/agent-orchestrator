package sessionmanager

import (
	"os"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
)

// TestMain forces Claude worker sandbox readiness for this package. Most
// session_manager tests build Claude worker launch argv without caring about
// bubblewrap/socat or a real `claude --version`; the real probes still run in
// claudecode's own tests and at daemon start.
func TestMain(m *testing.M) {
	restore := claudecode.ForceSandboxReadyForTest()
	code := m.Run()
	restore()
	os.Exit(code)
}
