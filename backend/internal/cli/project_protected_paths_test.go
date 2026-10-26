package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestProjectSetConfigAddProtectedPathKeepsOtherSettings(t *testing.T) {
	cfg := setConfigEnv(t)
	var putBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/demo":
			_, _ = io.WriteString(w, `{"status":"ok","project":{"id":"demo","config":{"teamMode":false,"env":{"A":"1"},"workerSandbox":{"enabled":false,"allowedDomains":["x.dev"],"protectedPaths":["config/prod.yml"]}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/demo/config":
			putBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"project":{"id":"demo"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"project", "set-config", "demo", "--add-protected-path", "exports/", "--add-protected-path", "config/prod.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "updated config for project demo") {
		t.Fatalf("stdout = %q", out)
	}
	var got struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(putBody, &got); err != nil {
		t.Fatalf("decode PUT: %v\n%s", err, putBody)
	}
	sandbox := got.Config["workerSandbox"].(map[string]any)
	if !reflect.DeepEqual(sandbox["protectedPaths"], []any{"config/prod.yml", "exports/"}) {
		t.Fatalf("protectedPaths = %#v", sandbox["protectedPaths"])
	}
	if sandbox["enabled"] != false || got.Config["teamMode"] != false || got.Config["env"] == nil ||
		!reflect.DeepEqual(sandbox["allowedDomains"], []any{"x.dev"}) {
		t.Fatalf("other settings must survive: %s", putBody)
	}
}

func TestProjectSetConfigAddProtectedPathRejectsOtherFlags(t *testing.T) {
	setConfigEnv(t)
	_, _, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }},
		"project", "set-config", "demo", "--add-protected-path", "exports/", "--session-prefix", "x")
	var usage usageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want usage error", err)
	}
}
