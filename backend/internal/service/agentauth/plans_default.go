//go:build !retired

package agentauth

import "context"

const retiredAuthPlansEnabled = false

func prepareRetiredKimiWorkspace(context.Context, string) error { return nil }
func prepareRetiredCopilotWorkspace(context.Context, string) error { return nil }
