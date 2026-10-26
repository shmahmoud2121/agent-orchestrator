// Runs the real Claude Code worker-secrets canary
// (backend/internal/adapters/agent/claudecode/canary_test.go) and fails when it
// could not run, so a skipped canary is never mistaken for a pass.
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const backend = fileURLToPath(new URL("../backend/", import.meta.url));
const result = spawnSync("go", ["test", "-count=1", "-v", "-run", "^TestWorkerSecretsCanary$", "./internal/adapters/agent/claudecode/"], {
	cwd: backend,
	env: { ...process.env, AO_CANARY: "1" },
	encoding: "utf8",
	shell: process.platform === "win32",
});
process.stdout.write(result.stdout ?? "");
process.stderr.write(result.stderr ?? "");
if (result.status === 0 && /--- SKIP: TestWorkerSecretsCanary/.test(result.stdout ?? "")) {
	console.error("canary did not run: see the skip reason above");
	process.exit(2);
}
process.exit(result.status ?? 1);
