//go:build !retired

package cli

func (*commandContext) handleRetiredMemcodeHook(string, string, string) bool { return false }
