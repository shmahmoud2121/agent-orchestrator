package domain

import (
	"fmt"
	"slices"
	"strings"
)

// WorkerEnvKeepVar is the launch env entry that turns on FilterWorkerEnv. The
// session manager sets it only for worker launches; its value lists the
// inherited variables the worker keeps although they look secret (a trailing
// "*" keeps every name with that prefix). The manager and the person's own
// terminals never carry it, so their environment is unchanged.
const WorkerEnvKeepVar = "AO_WORKER_ENV_KEEP"

// IsSecretEnvName reports whether an inherited variable looks like a password,
// key, token or database address. Matching ignores case.
func IsSecretEnvName(name string) bool {
	upper := strings.ToUpper(name)
	if upper == "PWD" {
		return false
	}
	for _, suffix := range []string{"_KEY", "_SECRET", "_TOKEN", "_PASSWORD", "_PASSWD", "_PWD", "_DATABASE_URL", "_DSN"} {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	if strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") || upper == "DATABASE_URL" {
		return true
	}
	for _, prefix := range []string{"AWS_", "AZURE_", "GOOGLE_APPLICATION_CREDENTIALS", "STRIPE_"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// harnessLoginEnv lists the login variables each harness reads, taken from
// its adapter's auth probe, so a filtered worker still signs in. A trailing
// "*" keeps the prefix. Harnesses that sign in only through their own config
// files have no entry.
var harnessLoginEnv = map[AgentHarness][]string{
	HarnessClaudeCode: {"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"},
	HarnessCodex:      {"OPENAI_API_KEY", "CODEX_*"},
	HarnessAider: {"AIDER_API_KEY", "AIDER_OPENAI_API_KEY", "AIDER_ANTHROPIC_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "XAI_API_KEY",
		"MISTRAL_API_KEY", "COHERE_API_KEY"},
	HarnessOpenCode: {"OPENCODE_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		"OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "XAI_API_KEY", "MISTRAL_API_KEY", "COHERE_API_KEY"},
	HarnessOpenCodeV2: {"OPENCODE_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		"OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "XAI_API_KEY", "MISTRAL_API_KEY", "COHERE_API_KEY"},
	HarnessGrok:  {"XAI_API_KEY"},
	HarnessDroid: {"FACTORY_API_KEY"},
	HarnessAmp:   {"AMP_API_KEY"},
	HarnessCrush: {"HYPER_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "VERCEL_API_KEY", "GEMINI_API_KEY", "ZAI_API_KEY",
		"MINIMAX_API_KEY", "SYNTHETIC_API_KEY", "HF_TOKEN", "CEREBRAS_API_KEY", "OPENROUTER_API_KEY", "IONET_API_KEY",
		"ALIBABA_SINGAPORE_API_KEY", "ALIBABA_US_API_KEY", "GROQ_API_KEY", "AVIAN_API_KEY", "OPENCODE_API_KEY",
		"AZURE_OPENAI_API_KEY", "MOONSHOT_API_KEY"},
	HarnessCursor: {"CURSOR_API_KEY"},
	HarnessQwen: {"QWEN_API_KEY", "BAILIAN_CODING_PLAN_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "OPENROUTER_API_KEY", "REQUESTY_API_KEY", "DASHSCOPE_API_KEY", "ZAI_API_KEY"},
	HarnessGemini:  {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	HarnessCopilot: {"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"},
	HarnessGoose: {"GOOSE_API_KEY", "GOOSE_PROVIDER__API_KEY", "GOOSE_EDITOR_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "XAI_API_KEY",
		"MISTRAL_API_KEY", "COHERE_API_KEY"},
	HarnessDevin: {"DEVIN_API_KEY"},
	HarnessCline: {"CLINE_API_KEY"},
	HarnessKimi:  {"KIMI_API_KEY", "KIMI_CODE_API_KEY", "MOONSHOT_API_KEY", "OPENAI_API_KEY"},
	HarnessMuse:  {"META_API_KEY"},
	HarnessKiro:  {"KIRO_API_KEY"},
	HarnessKilocode: {"KILO_API_KEY", "KILOCODE_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "XAI_API_KEY", "MISTRAL_API_KEY",
		"COHERE_API_KEY"},
	HarnessVibe: {"MISTRAL_API_KEY", "VIBE_CODE_API_KEY"},
	HarnessPi: {"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "MISTRAL_API_KEY", "GROQ_API_KEY",
		"XAI_API_KEY", "OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "ZAI_API_KEY", "CEREBRAS_API_KEY", "KIMI_API_KEY"},
	HarnessKimchi: {"KIMCHI_API_KEY"},
	HarnessPrimeAgent: {"ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN", "AZURE_OPENAI_API_KEY", "OPENAI_API_KEY", "PRIME_API_KEY",
		"DEEPSEEK_API_KEY", "GEMINI_API_KEY", "GOOGLE_CLOUD_API_KEY", "MISTRAL_API_KEY", "GROQ_API_KEY", "CEREBRAS_API_KEY",
		"CLOUDFLARE_API_KEY", "XAI_API_KEY", "OPENROUTER_API_KEY", "AI_GATEWAY_API_KEY", "ZAI_API_KEY", "OPENCODE_API_KEY",
		"HF_TOKEN", "FIREWORKS_API_KEY", "KIMI_API_KEY", "MINIMAX_API_KEY", "MINIMAX_CN_API_KEY", "XIAOMI_*"},
	HarnessAutohand: {"AUTOHAND_API_KEY", "AUTOHAND_AUTH_TOKEN", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "OPENROUTER_API_KEY", "MISTRAL_API_KEY", "GROQ_API_KEY"},
	HarnessUnreal:   {"UNREAL_HARNESS_LLM_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "FIREWORKS_API_KEY", "OPENAI_CODEX_ACCESS_TOKEN"},
	HarnessDeepSeek: {"DEEPSEEK_API_KEY"},
	HarnessMemcode:  {"MEMCODE_API_TOKEN", "MEMCODE_CREDENTIALS", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"},
	HarnessAuggie:   {"AUGMENT_SESSION_AUTH"},
	HarnessMiMoCode: {"ANTHROPIC_API_KEY", "OPENAI_API_KEY"},
	HarnessOMP: {"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "MISTRAL_API_KEY", "GROQ_API_KEY",
		"XAI_API_KEY", "OPENROUTER_API_KEY", "DEEPSEEK_API_KEY", "ZAI_API_KEY", "CEREBRAS_API_KEY", "KIMI_API_KEY"},
}

// WorkerEnvKeepList is the WorkerEnvKeepVar value for one worker launch: the
// harness's login variables, the project's own env (the person chose those
// with --env), and the project's workerEnvKeep. Claude Code on Bedrock or
// Vertex signs in with cloud credentials, so those stay when the launch env
// selects that provider.
func WorkerEnvKeepList(harness AgentHarness, cfg ProjectConfig, lookup func(string) string) string {
	keep := append([]string(nil), harnessLoginEnv[harness]...)
	if harness == HarnessClaudeCode && lookup != nil {
		if envFlagOn(lookup("CLAUDE_CODE_USE_BEDROCK")) {
			keep = append(keep, "AWS_*")
		}
		if envFlagOn(lookup("CLAUDE_CODE_USE_VERTEX")) {
			keep = append(keep, "GOOGLE_APPLICATION_CREDENTIALS")
		}
	}
	for name := range cfg.Env {
		keep = append(keep, name)
	}
	keep = append(keep, cfg.WorkerEnvKeep...)
	slices.Sort(keep)
	return strings.Join(slices.Compact(keep), ",")
}

func envFlagOn(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// FilterWorkerEnv is the one filter every worker launch path applies to the
// environment it inherits from the daemon. When overlay carries
// WorkerEnvKeepVar it removes inherited variables that look secret
// (IsSecretEnvName) unless they are AO's own (AO_*), set by overlay, or kept.
// It returns the kept entries and the removed names, never values. Without the
// marker it returns environ unchanged. caseInsensitive matches names the way
// Windows does.
func FilterWorkerEnv(environ []string, overlay map[string]string, caseInsensitive bool) (kept, removed []string) {
	list, ok := overlay[WorkerEnvKeepVar]
	if !ok {
		return environ, nil
	}
	fold := func(name string) string {
		if caseInsensitive {
			return strings.ToUpper(name)
		}
		return name
	}
	var names, prefixes []string
	for _, entry := range strings.Split(list, ",") {
		if entry = strings.TrimSpace(entry); entry == "" {
			continue
		}
		if prefix, isPrefix := strings.CutSuffix(entry, "*"); isPrefix {
			prefixes = append(prefixes, fold(prefix))
		} else {
			names = append(names, fold(entry))
		}
	}
	for key := range overlay {
		names = append(names, fold(key))
	}
	keep := func(name string) bool {
		folded := fold(name)
		if strings.HasPrefix(strings.ToUpper(name), "AO_") || slices.Contains(names, folded) {
			return true
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(folded, prefix) {
				return true
			}
		}
		return false
	}
	kept = make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if name != "" && IsSecretEnvName(name) && !keep(name) {
			removed = append(removed, name)
			continue
		}
		kept = append(kept, entry)
	}
	return kept, removed
}

// ValidateWorkerEnvKeep accepts variable names and NAME_* prefixes.
func ValidateWorkerEnvKeep(entries []string) error {
	if len(entries) > MaxWorkerProtectedPaths {
		return fmt.Errorf("workerEnvKeep: at most %d entries", MaxWorkerProtectedPaths)
	}
	for i, entry := range entries {
		name := strings.TrimSuffix(entry, "*")
		if !validEnvName(name) || strings.Count(entry, "*") > 1 {
			return fmt.Errorf("workerEnvKeep[%d]: %q must be a variable name, optionally ending in *", i, entry)
		}
	}
	return nil
}
