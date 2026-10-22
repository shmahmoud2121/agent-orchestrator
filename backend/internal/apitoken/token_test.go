package apitoken

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestLoadOrCreateCreatesAndReusesToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	first, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("LoadOrCreate replaced the existing token")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Path(dir))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("token mode = %04o, want 0600", got)
		}
		parent, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := parent.Mode().Perm(); got != 0o700 {
			t.Fatalf("data dir mode = %04o, want 0700", got)
		}
	}
}

func TestLoadOrCreateConcurrentCallersReuseOneCompleteToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	const callers = 8
	tokens := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens[i], errs[i] = LoadOrCreate(dir)
		}()
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if tokens[i] != tokens[0] {
			t.Fatalf("caller %d received a different token", i)
		}
	}
}

func TestLoadRejectsInsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows relies on the user-profile ACL")
	}
	dir := t.TempDir()
	path := Path(dir)
	if err := os.WriteFile(path, []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted group/world-readable token")
	}
}
