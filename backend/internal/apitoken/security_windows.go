//go:build windows

package apitoken

import "os"

// The token stays under the user's AO profile directory. Windows relies on
// that profile ACL; POSIX mode and uid checks have no equivalent meaning here.
func secureDir(string) error       { return nil }
func secureFile(os.FileInfo) error { return nil }
