//go:build !windows

package apitoken

import "os"

// publish hard-links the fully written inode under the stable name. Link is
// atomic and fails rather than replacing an existing concurrent winner.
func publish(tempPath, path string) error { return os.Link(tempPath, path) }
