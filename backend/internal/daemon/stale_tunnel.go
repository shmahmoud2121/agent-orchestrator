package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	aoprocess "github.com/aoagents/agent-orchestrator/backend/internal/process"
)

const staleTunnelProbeTimeout = 3 * time.Second

func staleTunnelPIDPath(dataDir string) string {
	return filepath.Join(dataDir, "mobile", "tunnel.pid")
}

func readStaleTunnelPID(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid, err == nil && pid > 0
}

// reapStaleTunnel closes a public cloudflared tunnel left by a retired build.
// It verifies the process name first because PIDs can be reused.
func reapStaleTunnel(path string) error {
	pid, ok := readStaleTunnelPID(path)
	if !ok {
		return nil
	}
	defer func() { _ = os.Remove(path) }()
	if !isLiveCloudflared(pid) {
		return nil
	}
	if err := killCloudflared(pid); err != nil {
		return fmt.Errorf("kill stale tunnel %d: %w", pid, err)
	}
	return nil
}

func isLiveCloudflared(pid int) bool {
	if pid <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), staleTunnelProbeTimeout)
	defer cancel()
	if runtime.GOOS == "windows" {
		out, err := aoprocess.CommandContext(ctx, "tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH", "/FO", "CSV").Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "cloudflared")
	}
	out, err := aoprocess.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	return err == nil && strings.Contains(strings.ToLower(strings.TrimSpace(string(out))), "cloudflared")
}

func killCloudflared(pid int) error {
	if runtime.GOOS == "windows" {
		ctx, cancel := context.WithTimeout(context.Background(), staleTunnelProbeTimeout)
		defer cancel()
		return aoprocess.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
