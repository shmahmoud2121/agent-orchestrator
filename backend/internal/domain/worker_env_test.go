package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestIsSecretEnvName(t *testing.T) {
	for _, secret := range []string{
		"GITHUB_TOKEN", "my_api_key", "DB_PASSWORD", "SMTP_PASSWD", "MYSQL_PWD", "CLIENT_SECRET",
		"PASSWORD_FILE", "SECRETS_DIR", "DATABASE_URL", "REPLICA_DATABASE_URL", "SENTRY_DSN",
		"AWS_PROFILE", "AZURE_TENANT_ID", "GOOGLE_APPLICATION_CREDENTIALS", "STRIPE_ACCOUNT",
	} {
		if !IsSecretEnvName(secret) {
			t.Errorf("%s should be stripped", secret)
		}
	}
	for _, plain := range []string{"PWD", "OLDPWD", "PATH", "HOME", "SSH_AUTH_SOCK", "LANG", "KEYCHAIN", "TOKENIZERS_PARALLELISM"} {
		if IsSecretEnvName(plain) {
			t.Errorf("%s should be kept", plain)
		}
	}
}

func TestFilterWorkerEnv(t *testing.T) {
	environ := []string{
		"PATH=/bin", "PWD=/w", "GITHUB_TOKEN=gh", "ANTHROPIC_API_KEY=sk", "AWS_REGION=us", "AO_DATA_DIR=/ao",
		"MY_SERVICE_TOKEN=x", "PROJECT_SECRET=old", "XIAOMI_TOKEN_PLAN_CN_API_KEY=y",
	}
	overlay := map[string]string{WorkerEnvKeepVar: "ANTHROPIC_API_KEY,MY_SERVICE_*", "PROJECT_SECRET": "new"}
	kept, removed := FilterWorkerEnv(environ, overlay, false)
	for _, want := range []string{"PATH=/bin", "PWD=/w", "ANTHROPIC_API_KEY=sk", "AO_DATA_DIR=/ao", "MY_SERVICE_TOKEN=x", "PROJECT_SECRET=old"} {
		if !slices.Contains(kept, want) {
			t.Errorf("kept %v, missing %s", kept, want)
		}
	}
	slices.Sort(removed)
	if want := []string{"AWS_REGION", "GITHUB_TOKEN", "XIAOMI_TOKEN_PLAN_CN_API_KEY"}; !slices.Equal(removed, want) {
		t.Fatalf("removed = %v, want %v", removed, want)
	}
	for _, name := range removed {
		if strings.Contains(name, "=") {
			t.Fatalf("removed names must not carry values: %q", name)
		}
	}

	if kept, removed := FilterWorkerEnv(environ, map[string]string{"X": "1"}, false); !slices.Equal(kept, environ) || removed != nil {
		t.Fatal("without the marker the environment must be unchanged")
	}
}

func TestFilterWorkerEnvWindowsCase(t *testing.T) {
	overlay := map[string]string{WorkerEnvKeepVar: "ANTHROPIC_API_KEY"}
	kept, _ := FilterWorkerEnv([]string{"Anthropic_Api_Key=sk", "github_token=gh"}, overlay, true)
	if !slices.Equal(kept, []string{"Anthropic_Api_Key=sk"}) {
		t.Fatalf("windows kept = %v", kept)
	}
	kept, _ = FilterWorkerEnv([]string{"Anthropic_Api_Key=sk"}, overlay, false)
	if len(kept) != 0 {
		t.Fatalf("unix names are case-sensitive: kept = %v", kept)
	}
}

func TestWorkerEnvKeepList(t *testing.T) {
	cfg := ProjectConfig{Env: map[string]string{"STRIPE_KEY": "x"}, WorkerEnvKeep: []string{"NPM_TOKEN"}}
	got := strings.Split(WorkerEnvKeepList(HarnessClaudeCode, cfg, func(string) string { return "" }), ",")
	for _, want := range []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "STRIPE_KEY", "NPM_TOKEN"} {
		if !slices.Contains(got, want) {
			t.Errorf("keep list %v missing %s", got, want)
		}
	}
	if slices.Contains(got, "AWS_*") {
		t.Error("AWS credentials stay stripped unless Claude Code uses Bedrock")
	}
	bedrock := WorkerEnvKeepList(HarnessClaudeCode, ProjectConfig{}, func(name string) string {
		if name == "CLAUDE_CODE_USE_BEDROCK" {
			return "1"
		}
		return ""
	})
	if !strings.Contains(bedrock, "AWS_*") {
		t.Fatalf("Claude Code on Bedrock keeps AWS credentials: %s", bedrock)
	}
	codex := WorkerEnvKeepList(HarnessCodex, ProjectConfig{}, nil)
	if !strings.Contains(codex, "OPENAI_API_KEY") || !strings.Contains(codex, "CODEX_*") || strings.Contains(codex, "ANTHROPIC") {
		t.Fatalf("codex keep list = %s", codex)
	}
}

func TestValidateWorkerEnvKeep(t *testing.T) {
	if err := ValidateWorkerEnvKeep([]string{"NPM_TOKEN", "MY_APP_*"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "BAD-NAME", "A**", "*"} {
		if err := ValidateWorkerEnvKeep([]string{bad}); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}
