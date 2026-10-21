package daemon

import (
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/mobilebridge"
)

// This build of AO is local-only. The daemon serves on loopback and never
// reaches the network on its own initiative: no Connect Mobile LAN listener,
// cloudflared tunnel, Tailscale serve, push dispatch, remote-host address
// publishing, remote telemetry (PostHog), Sentry, harness update checks,
// pricing-catalog downloads, link-preview unfurling, or cloud control plane.
// The subsystems' code is kept so these can be switched back on later; the
// switch points are here and in Run.

// localOnlyConfig forces every network-facing setting off, whatever the
// environment asked for. Run applies it before anything reads cfg, so the
// telemetry, Sentry, harness-update, and cloud wiring all see the disabled
// values.
func localOnlyConfig(cfg config.Config) config.Config {
	cfg.Telemetry.Remote = config.TelemetryRemoteOff
	cfg.Telemetry.PostHogKey = ""
	cfg.Telemetry.PostHogHost = ""
	cfg.Telemetry.SentryDSN = ""
	cfg.HarnessUpdateChecks = false
	cfg.CloudOffering = false
	cfg.CloudControlPlaneURL = ""
	return cfg
}

// logRemoteAccessDisabled records, once per boot, that remote access is not
// available in this build. A Connect Mobile config persisted by an earlier
// build is left on disk untouched but never acted on.
func logRemoteAccessDisabled(dataDir string, log *slog.Logger) {
	state, err := mobilebridge.Load(mobilebridge.Path(dataDir))
	persistedEnabled := err == nil && state.Enabled
	log.Info("remote access is disabled in this build; AO serves on loopback only",
		"persistedMobileEnabled", persistedEnabled)
}
