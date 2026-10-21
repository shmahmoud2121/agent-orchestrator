package daemon

import (
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
)

func TestLocalOnlyConfigOverridesEnabledNetworkFeatures(t *testing.T) {
	cfg := config.Config{Host: "0.0.0.0", TrackerIntake: true, CloudOffering: true, CloudControlPlaneURL: "https://cloud.example.test", HarnessUpdateChecks: true}
	cfg.Telemetry.Remote = config.TelemetryRemotePostHog
	cfg.Telemetry.PostHogKey = "key"
	cfg.Telemetry.PostHogHost = "https://telemetry.example.test"
	cfg.Telemetry.SentryDSN = "https://key@errors.example.test/1"
	cfg = localOnlyConfig(cfg)
	if cfg.Host != config.LoopbackHost || cfg.TrackerIntake {
		t.Fatal("local-only build allowed a non-loopback host or background issue intake")
	}
	if cfg.CloudOffering || cfg.CloudControlPlaneURL != "" || cfg.HarnessUpdateChecks {
		t.Fatal("local-only build enabled cloud or harness update checks")
	}
	if cfg.Telemetry.Remote != config.TelemetryRemoteOff || cfg.Telemetry.PostHogKey != "" || cfg.Telemetry.PostHogHost != "" || cfg.Telemetry.SentryDSN != "" {
		t.Fatal("local-only build enabled a telemetry destination")
	}
}
