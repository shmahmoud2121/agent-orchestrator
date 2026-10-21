// A project's verifyCommand is an argument list the daemon runs directly, never
// through a shell. Settings edits it as one line, so these helpers convert
// between that line and the list using POSIX-style quoting.

/** Mirrors the daemon's domain.MaxVerifyTimeoutSeconds. */
export const MAX_VERIFY_TIMEOUT_SECONDS = 7200;
/** Mirrors the daemon's domain.DefaultVerifyTimeoutSeconds. */
export const DEFAULT_VERIFY_TIMEOUT_SECONDS = 600;

/** Splits a command line into arguments; undefined when a quote is left open. */
export function parseVerifyCommand(line: string): string[] | undefined {
	const args: string[] = [];
	let current = "";
	let inArg = false;
	let quote: "'" | '"' | undefined;
	for (let i = 0; i < line.length; i++) {
		const char = line[i];
		if (quote === "'") {
			if (char === "'") quote = undefined;
			else current += char;
		} else if (quote === '"') {
			if (char === '"') quote = undefined;
			else if (char === "\\" && (line[i + 1] === '"' || line[i + 1] === "\\")) current += line[++i];
			else current += char;
		} else if (char === "'" || char === '"') {
			quote = char;
			inArg = true;
		} else if (char === "\\" && i + 1 < line.length) {
			current += line[++i];
			inArg = true;
		} else if (/\s/.test(char)) {
			if (inArg) args.push(current);
			current = "";
			inArg = false;
		} else {
			current += char;
			inArg = true;
		}
	}
	if (quote) return undefined;
	if (inArg) args.push(current);
	return args;
}

/** Joins arguments into a line that parseVerifyCommand reads back unchanged. */
export function formatVerifyCommand(args: readonly string[] | undefined): string {
	return (args ?? []).map((arg) => /^[\w@%+=:,./-]+$/.test(arg) ? arg : `'${arg.replaceAll("'", `'\\''`)}'`).join(" ");
}

/** Reads the timeout field: undefined means the daemon default, NaN is invalid. */
export function parseVerifyTimeout(value: string, max = MAX_VERIFY_TIMEOUT_SECONDS): number | undefined {
	const trimmed = value.trim();
	if (!trimmed) return undefined;
	const seconds = /^\d+$/.test(trimmed) ? Number(trimmed) : Number.NaN;
	return seconds >= 1 && seconds <= max ? seconds : Number.NaN;
}
