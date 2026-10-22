// Package apitoken owns the per-install credential for the loopback daemon.
package apitoken

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// FileName is deliberately stable so every local client can find the token
	// from the same AO_DATA_DIR without copying it into running.json.
	FileName   = "api-token"
	tokenBytes = 32
)

// Path returns the credential path for an already-resolved daemon data dir.
func Path(dataDir string) string { return filepath.Join(dataDir, FileName) }

// LoadOrCreate loads the existing install token or creates it with atomic,
// no-replace publication. A concurrent starter can never observe a partial
// token or overwrite the token another process published.
func LoadOrCreate(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", errors.New("API token data directory is empty")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("create API token directory: %w", err)
	}
	if err := secureDir(dataDir); err != nil {
		return "", err
	}

	path := Path(dataDir)
	if token, err := Load(path); err == nil {
		return token, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate API token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	f, err := os.CreateTemp(dataDir, ".api-token-*")
	if err != nil {
		return "", fmt.Errorf("create temporary API token: %w", err)
	}
	tempPath := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tempPath)
	}()
	if err := f.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure temporary API token: %w", err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		return "", fmt.Errorf("write API token: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync API token: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close API token: %w", err)
	}
	if err := publish(tempPath, path); errors.Is(err, os.ErrExist) {
		return Load(path)
	} else if err != nil {
		return "", fmt.Errorf("publish API token: %w", err)
	}
	return token, nil
}

// Load validates and reads a token without ever including its contents in an
// error. Unix ownership and permission checks live in platform files.
func Load(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("API token file must be a regular file")
	}
	if err := secureFile(info); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed API token location.
	if err != nil {
		return "", fmt.Errorf("read API token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != tokenBytes {
		return "", errors.New("API token file is invalid")
	}
	return token, nil
}
