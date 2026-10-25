package daemon

import "github.com/aoagents/agent-orchestrator/backend/internal/config"

// This build of AO is local-only. The daemon serves on loopback and never
// reaches the network on its own initiative: no Connect Mobile LAN listener,
// cloudflared tunnel, Tailscale serve, push dispatch, remote-host address
// publishing, remote telemetry (PostHog), Sentry, harness update checks,
// pricing-catalog downloads, link-preview unfurling, or cloud control plane.
// Archived cloud and mobile implementations can be restored from
// archive/pre-retire-2026-10.

const localOnlyBuild = true

// localOnlyConfig forces every network-facing setting off, whatever the
// environment asked for. Run applies it before anything reads cfg, so the
// telemetry, Sentry, harness-update, and cloud wiring all see the disabled
// values.
func localOnlyConfig(cfg config.Config) config.Config {
	cfg.Host = config.LoopbackHost
	cfg.TrackerIntake = false
	cfg.Telemetry.Remote = config.TelemetryRemoteOff
	cfg.Telemetry.PostHogKey = ""
	cfg.Telemetry.PostHogHost = ""
	cfg.Telemetry.SentryDSN = ""
	cfg.HarnessUpdateChecks = false
	cfg.CloudOffering = false
	cfg.CloudControlPlaneURL = ""
	return cfg
}
