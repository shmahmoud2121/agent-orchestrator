// Runs `go test` for just the backend packages with changed .go files: the
// uncommitted ones by default, or everything since a ref (`main`, `HEAD~3`).
// Go's test cache stays on, so an unchanged dependency costs nothing. The full
// suite still runs in CI.
//
//   npm run test:go:changed            # uncommitted changes
//   npm run test:go:changed -- main    # everything since main
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).split("\n").filter(Boolean);

const base = process.argv[2];
const files = base
	? git("diff", "--name-only", `${base}...HEAD`, "--", "backend")
		.concat(git("diff", "--name-only", "HEAD", "--", "backend"))
	: git("diff", "--name-only", "HEAD", "--", "backend");
files.push(...git("ls-files", "--others", "--exclude-standard", "--", "backend"));

const packages = [...new Set(files
	.filter((file) => file.endsWith(".go") && existsSync(join(root, file)))
	.map((file) => "./" + dirname(file).slice("backend/".length)))].sort();

if (packages.length === 0) {
	console.log("No changed Go packages.");
	process.exit(0);
}
console.log(`go test ${packages.join(" ")}`);
const result = spawnSync("go", ["test", ...packages], { cwd: join(root, "backend"), stdio: "inherit" });
process.exit(result.status ?? 1);
