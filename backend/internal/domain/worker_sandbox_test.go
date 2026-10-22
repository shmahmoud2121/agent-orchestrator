package domain

import (
	"encoding/json"
	"testing"
)

func TestWorkerSandboxConfigIsEnabledDefaultsTrue(t *testing.T) {
	if !(WorkerSandboxConfig{}).IsEnabled() {
		t.Fatal("missing config must default to enabled")
	}
	enabled := true
	if !(WorkerSandboxConfig{Enabled: &enabled}).IsEnabled() {
		t.Fatal("explicit true must be enabled")
	}
	disabled := false
	if (WorkerSandboxConfig{Enabled: &disabled}).IsEnabled() {
		t.Fatal("explicit false must be disabled")
	}
}

func TestWorkerSandboxConfigJSONOmitsEnabledMeansOn(t *testing.T) {
	var cfg ProjectConfig
	if err := json.Unmarshal([]byte(`{"workerSandbox":{"allowedDomains":["registry.npmjs.org"]}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.WorkerSandbox.IsEnabled() {
		t.Fatal("omitted enabled must default on")
	}
	if got := cfg.WorkerSandbox.AllowedDomains; len(got) != 1 || got[0] != "registry.npmjs.org" {
		t.Fatalf("domains = %#v", got)
	}
}

func TestWorkerSandboxConfigValidateDomains(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
		wantErr bool
	}{
		{"empty ok", nil, false},
		{"hostname", []string{"github.com"}, false},
		{"wildcard", []string{"*.npmjs.org"}, false},
		{"scheme rejected", []string{"https://github.com"}, true},
		{"path rejected", []string{"github.com/foo"}, true},
		{"port rejected", []string{"github.com:443"}, true},
		{"space rejected", []string{" github.com"}, true},
		{"ip rejected", []string{"127.0.0.1"}, true},
		{"double wildcard", []string{"*.*.example.com"}, true},
		{"bare star", []string{"*"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (WorkerSandboxConfig{AllowedDomains: tt.domains}).Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjectConfigValidateWorkerSandbox(t *testing.T) {
	if err := (ProjectConfig{WorkerSandbox: WorkerSandboxConfig{AllowedDomains: []string{"ok.example"}}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ProjectConfig{WorkerSandbox: WorkerSandboxConfig{AllowedDomains: []string{"http://bad"}}}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
