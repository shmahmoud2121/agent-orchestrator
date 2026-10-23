package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStaleTunnelPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mobile", "tunnel.pid")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := readStaleTunnelPID(path); !ok || got != 4242 {
		t.Fatalf("pid = (%d, %v), want (4242, true)", got, ok)
	}
}

func TestReapStaleTunnelClearsUnconfirmedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mobile", "tunnel.pid")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reapStaleTunnel(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale pid record survived: %v", err)
	}
}
