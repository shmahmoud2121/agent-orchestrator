//go:build !windows

package apitoken

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func secureDir(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure API token directory: %w", err)
	}
	return nil
}

func secureFile(info os.FileInfo) error {
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("API token file has insecure permissions %04o; want 0600", info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot verify API token file owner")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return errors.New("API token file is not owned by the current user")
	}
	return nil
}
