//go:build !retired

package registry

import "log/slog"

const retiredBuildEnabled = false

func addRetiredDrivers(*Registry, *slog.Logger) {}
