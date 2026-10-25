//go:build !retired

package sessionmanager

import "context"

func retiredMemcodeUsesCustomEndpoint(context.Context, string, map[string]string) (bool, error) {
	return false, nil
}
