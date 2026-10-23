//go:build !retired

package cli

import "context"

func (*commandContext) handleRetiredMemcodeHook(string, string, string) bool { return false }

func retiredCursorTerminalFailureCorrelation([]byte) (string, string, bool) {
	return "", "", false
}

func (*commandContext) runRetiredCursorPermissionHook(context.Context, string, string, string, []byte) error {
	return nil
}
